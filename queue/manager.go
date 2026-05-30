package queue

import (
	"backend/config"
	"backend/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	StatusRunning = "RUNNING"
	StatusPaused  = "PAUSED"
	JobPending    = "PENDING"
	JobRunning    = "RUNNING"
	JobSuccess    = "SUCCESS"
	JobFailed     = "FAILED"
)

// JobHandler 处理器签名
type JobHandler func(ctx context.Context, payload string) (string, error)

// HandleOpts Handle 注册时的运维参数默认值（仅在 tube 不存在时使用）
type HandleOpts struct {
	Description string
	Concurrency int // 默认 3
	Timeout     int // 秒，默认 60
	MaxRetries  int
}

// PushOpts 投递任务时的可选参数
type PushOpts struct {
	MaxRetries int // 0 时沿用 queue 默认
	Priority   int
	DelayUntil *time.Time
	UniqueKey  string
}

var (
	handlers   = make(map[string]JobHandler)
	handlersMu sync.RWMutex
)

func setHandler(name string, fn JobHandler) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers[name] = fn
}

func getHandler(name string) (JobHandler, bool) {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	fn, ok := handlers[name]
	return fn, ok
}

// HandlerList 返回已注册 handler 的 tube 名称清单（前端「已注册」徽标用）
func HandlerList() []string {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	out := make([]string, 0, len(handlers))
	for k := range handlers {
		out = append(out, k)
	}
	return out
}

// Handle 注册一个 tube 的处理函数；若对应 Queue 行不存在则自动创建（FirstOrCreate）
func Handle(name string, fn JobHandler) error {
	return HandleWithOpts(name, fn, HandleOpts{})
}

// HandleWithOpts 注册带默认运维参数的 tube
func HandleWithOpts(name string, fn JobHandler, opts HandleOpts) error {
	if name == "" || fn == nil {
		return errors.New("queue.Handle: name and fn required")
	}
	setHandler(name, fn)
	_, err := ensureQueue(name, opts)
	return err
}

// Push 投递任务到 tube；tube 不存在则自动创建（beanstalk 行为）
func Push(ctx context.Context, name string, payload any) (uint, error) {
	return PushWithOpts(ctx, name, payload, PushOpts{})
}

// createUniqueJob 在事务中创建带 uniqueKey 的 job，防止并发重复插入。
// 先锁住 Queue 行，序列化同一队列的 unique 投递，再检查 PENDING/RUNNING 任务。
func createUniqueJob(ctx context.Context, queueID uint, payload string, opts PushOpts, retries int) (uint, error) {
	var jobID uint
	err := config.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var q models.Queue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&q, queueID).Error; err != nil {
			return err
		}

		var existing models.QueueJob
		err := tx.Where("queue_id = ? AND unique_key = ? AND status IN ?", queueID, opts.UniqueKey, []string{JobPending, JobRunning}).
			First(&existing).Error
		if err == nil {
			jobID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		job := models.QueueJob{
			QueueID:    queueID,
			Payload:    payload,
			Status:     JobPending,
			Priority:   opts.Priority,
			DelayUntil: opts.DelayUntil,
			UniqueKey:  opts.UniqueKey,
			MaxRetries: retries,
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		jobID = job.ID

		if err := tx.Model(&models.Queue{}).Where("id = ?", queueID).
			UpdateColumn("total_jobs", gorm.Expr("total_jobs + 1")).Error; err != nil {
			return fmt.Errorf("bump total_jobs queue=%d: %w", queueID, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return jobID, nil
}

// PushWithOpts 带选项投递
func PushWithOpts(ctx context.Context, name string, payload any, opts PushOpts) (uint, error) {
	if name == "" {
		return 0, errors.New("queue.Push: name required")
	}
	body, err := encodePayload(payload)
	if err != nil {
		return 0, err
	}

	q, err := ensureQueue(name, HandleOpts{})
	if err != nil {
		return 0, err
	}

	retries := opts.MaxRetries
	if retries <= 0 {
		retries = q.MaxRetries
	}

	if opts.UniqueKey != "" {
		jobID, err := createUniqueJob(ctx, q.ID, body, opts, retries)
		if err != nil {
			return 0, err
		}
		return jobID, nil
	}

	job := models.QueueJob{
		QueueID:    q.ID,
		Payload:    body,
		Status:     JobPending,
		Priority:   opts.Priority,
		DelayUntil: opts.DelayUntil,
		UniqueKey:  opts.UniqueKey,
		MaxRetries: retries,
	}
	if err := config.DB.WithContext(ctx).Create(&job).Error; err != nil {
		return 0, err
	}
	if err := config.DB.Model(&models.Queue{}).Where("id = ?", q.ID).
		UpdateColumn("total_jobs", gorm.Expr("total_jobs + 1")).Error; err != nil {
		log.Printf("queue: bump total_jobs queue=%d: %v", q.ID, err)
	}
	return job.ID, nil
}

// BulkPushItem 批量投递的单个条目
type BulkPushItem struct {
	Payload any
	Opts    PushOpts
}

// BulkPush 批量投递任务到同一 tube。重复 unique_key 会被去重；
// 已存在 PENDING/RUNNING 的 unique_key 会被跳过。返回新创建的任务数。
func BulkPush(ctx context.Context, name string, items []BulkPushItem) (int, error) {
	if name == "" {
		return 0, errors.New("queue.BulkPush: name required")
	}
	if len(items) == 0 {
		return 0, nil
	}
	q, err := ensureQueue(name, HandleOpts{})
	if err != nil {
		return 0, err
	}

	created := 0
	err = config.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lockedQueue models.Queue
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedQueue, q.ID).Error; err != nil {
			return err
		}

		uniqueKeys := make([]string, 0, len(items))
		seen := make(map[string]struct{}, len(items))
		for _, it := range items {
			if it.Opts.UniqueKey == "" {
				continue
			}
			if _, ok := seen[it.Opts.UniqueKey]; ok {
				continue
			}
			seen[it.Opts.UniqueKey] = struct{}{}
			uniqueKeys = append(uniqueKeys, it.Opts.UniqueKey)
		}

		existingKeys := make(map[string]struct{}, len(uniqueKeys))
		if len(uniqueKeys) > 0 {
			var existing []models.QueueJob
			if err := tx.Where("queue_id = ? AND unique_key IN ? AND status IN ?", q.ID, uniqueKeys, []string{JobPending, JobRunning}).
				Find(&existing).Error; err != nil {
				return err
			}
			for _, j := range existing {
				existingKeys[j.UniqueKey] = struct{}{}
			}
		}

		jobs := make([]models.QueueJob, 0, len(items))
		batchKeys := make(map[string]struct{}, len(items))
		for _, it := range items {
			if it.Opts.UniqueKey != "" {
				if _, ok := existingKeys[it.Opts.UniqueKey]; ok {
					continue
				}
				if _, ok := batchKeys[it.Opts.UniqueKey]; ok {
					continue
				}
				batchKeys[it.Opts.UniqueKey] = struct{}{}
			}
			body, err := encodePayload(it.Payload)
			if err != nil {
				return err
			}
			retries := it.Opts.MaxRetries
			if retries <= 0 {
				retries = q.MaxRetries
			}
			jobs = append(jobs, models.QueueJob{
				QueueID:    q.ID,
				Payload:    body,
				Status:     JobPending,
				Priority:   it.Opts.Priority,
				DelayUntil: it.Opts.DelayUntil,
				UniqueKey:  it.Opts.UniqueKey,
				MaxRetries: retries,
			})
		}
		if len(jobs) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(jobs, 200).Error; err != nil {
			return err
		}
		created = len(jobs)
		if err := tx.Model(&models.Queue{}).Where("id = ?", q.ID).
			UpdateColumn("total_jobs", gorm.Expr("total_jobs + ?", len(jobs))).Error; err != nil {
			return fmt.Errorf("bump total_jobs queue=%d: %w", q.ID, err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

func encodePayload(payload any) (string, error) {
	if payload == nil {
		return "", nil
	}
	switch v := payload.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		buf, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	}
}

// ensureQueue 按 name 查找 Queue，不存在则按 opts 创建（FirstOrCreate）
func ensureQueue(name string, opts HandleOpts) (*models.Queue, error) {
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 3
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	q := models.Queue{
		Name:        name,
		Description: opts.Description,
		Status:      StatusRunning,
		Concurrency: concurrency,
		Timeout:     timeout,
		MaxRetries:  opts.MaxRetries,
	}
	if err := config.DB.Where(models.Queue{Name: name}).Attrs(q).FirstOrCreate(&q).Error; err != nil {
		return nil, fmt.Errorf("ensure queue %s: %w", name, err)
	}
	return &q, nil
}

// Manager 队列调度器
type Manager struct {
	stopCh chan struct{}
	wg     sync.WaitGroup

	mu      sync.Mutex
	running map[uint]map[uint]int // queueID -> jobID -> retryCount
	warned  map[uint]time.Time    // queueID -> 最近一次「未注册」警告时间（日志限流）
}

var defaultManager *Manager

// Default 返回全局队列管理器（必须在 Init 之后使用）
func Default() *Manager { return defaultManager }

// Init 启动队列管理器。
func Init() {
	defaultManager = &Manager{
		stopCh:  make(chan struct{}),
		running: make(map[uint]map[uint]int),
		warned:  make(map[uint]time.Time),
	}

	rehydrateHandlers()
	recoverOrphanedJobs()

	defaultManager.wg.Add(1)
	go defaultManager.loop()
	log.Println("Queue manager started")
}

func rehydrateHandlers() {
	handlersMu.RLock()
	names := make([]string, 0, len(handlers))
	for k := range handlers {
		names = append(names, k)
	}
	handlersMu.RUnlock()
	for _, name := range names {
		if _, err := ensureQueue(name, HandleOpts{}); err != nil {
			log.Printf("queue: rehydrate %s: %v", name, err)
		}
	}
}

func recoverOrphanedJobs() {
	var jobs []models.QueueJob
	if err := config.DB.Where("status = ?", JobRunning).Find(&jobs).Error; err != nil {
		log.Printf("queue: load orphaned jobs: %v", err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	requeued := 0
	failed := 0
	now := time.Now()
	for _, job := range jobs {
		if job.RetryCount < job.MaxRetries {
			res := config.DB.Model(&models.QueueJob{}).
				Where("id = ? AND status = ?", job.ID, JobRunning).
				Updates(map[string]any{
					"status":       JobPending,
					"retry_count":  job.RetryCount + 1,
					"started_at":   nil,
					"completed_at": nil,
					"duration":     int64(0),
					"result":       "",
					"error":        "",
				})
			if res.Error != nil || res.RowsAffected == 0 {
				log.Printf("queue: requeue orphaned job=%d: %v", job.ID, res.Error)
				continue
			}
			requeued++
			continue
		}

		completedAt := now
		duration := int64(0)
		if job.StartedAt != nil {
			duration = now.Sub(*job.StartedAt).Milliseconds()
			if duration < 0 {
				duration = 0
			}
		}
		res := config.DB.Model(&models.QueueJob{}).
			Where("id = ? AND status = ?", job.ID, JobRunning).
			Updates(map[string]any{
				"status":       JobFailed,
				"completed_at": &completedAt,
				"duration":     duration,
				"result":       "",
				"error":        "worker restarted before job completed",
			})
		if res.Error != nil || res.RowsAffected == 0 {
			log.Printf("queue: fail orphaned job=%d: %v", job.ID, res.Error)
			continue
		}
		if err := config.DB.Model(&models.Queue{}).Where("id = ?", job.QueueID).
			UpdateColumn("failed_jobs", gorm.Expr("failed_jobs + 1")).Error; err != nil {
			log.Printf("queue: bump failed_jobs queue=%d: %v", job.QueueID, err)
		}
		failed++
	}

	log.Printf("queue: recovered orphaned RUNNING jobs, requeued=%d failed=%d", requeued, failed)
}

// Stop 优雅停止
func Stop() {
	if defaultManager == nil {
		return
	}
	close(defaultManager.stopCh)
	defaultManager.wg.Wait()
}

func (m *Manager) loop() {
	defer m.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *Manager) tick() {
	var queues []models.Queue
	if err := config.DB.Where("status = ?", StatusRunning).Find(&queues).Error; err != nil {
		log.Printf("queue: load queues: %v", err)
		return
	}
	for i := range queues {
		m.recoverTimedOutJobs(&queues[i])
		m.dispatch(&queues[i])
	}
}

func (m *Manager) recoverTimedOutJobs(q *models.Queue) {
	timeout := q.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	deadline := time.Now().Add(-time.Duration(timeout) * time.Second)

	var jobs []models.QueueJob
	if err := config.DB.Where("queue_id = ? AND status = ? AND started_at IS NOT NULL AND started_at < ?", q.ID, JobRunning, deadline).
		Order("id ASC").Limit(100).Find(&jobs).Error; err != nil {
		log.Printf("queue: load timed out jobs queue=%d: %v", q.ID, err)
		return
	}

	now := time.Now()
	for _, job := range jobs {
		duration := int64(0)
		if job.StartedAt != nil {
			duration = now.Sub(*job.StartedAt).Milliseconds()
			if duration < 0 {
				duration = 0
			}
		}

		updates := map[string]any{
			"duration": duration,
			"result":   "",
			"error":    fmt.Sprintf("job timed out after %d seconds", timeout),
		}
		if job.RetryCount < job.MaxRetries {
			updates["status"] = JobPending
			updates["retry_count"] = job.RetryCount + 1
			updates["started_at"] = nil
			updates["completed_at"] = nil
		} else {
			completedAt := now
			updates["status"] = JobFailed
			updates["completed_at"] = &completedAt
		}

		res := config.DB.Model(&models.QueueJob{}).
			Where("id = ? AND status = ? AND retry_count = ?", job.ID, JobRunning, job.RetryCount).
			Updates(updates)
		if res.Error != nil {
			log.Printf("queue: recover timed out job=%d: %v", job.ID, res.Error)
			continue
		}
		if res.RowsAffected == 0 {
			continue
		}
		m.releaseRunning(q.ID, job.ID, job.RetryCount)
		if updates["status"] == JobFailed {
			if err := config.DB.Model(&models.Queue{}).Where("id = ?", q.ID).
				UpdateColumn("failed_jobs", gorm.Expr("failed_jobs + 1")).Error; err != nil {
				log.Printf("queue: bump failed_jobs queue=%d: %v", q.ID, err)
			}
		}
	}
}

func (m *Manager) dispatch(q *models.Queue) {
	if _, ok := getHandler(q.Name); !ok {
		m.warnNoHandler(q)
		return
	}

	concurrency := q.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	m.mu.Lock()
	busy := len(m.running[q.ID])
	free := concurrency - busy
	m.mu.Unlock()
	if free <= 0 {
		return
	}

	var jobs []models.QueueJob
	if err := config.DB.Where("queue_id = ? AND status = ? AND (delay_until IS NULL OR delay_until <= ?)", q.ID, JobPending, time.Now()).
		Order("priority DESC, id ASC").Limit(free).Find(&jobs).Error; err != nil {
		log.Printf("queue: load jobs queue=%d: %v", q.ID, err)
		return
	}
	if len(jobs) == 0 {
		return
	}

	for i := range jobs {
		job := jobs[i]
		// 抢占式 UPDATE，避免重复执行
		res := config.DB.Model(&models.QueueJob{}).
			Where("id = ? AND status = ?", job.ID, JobPending).
			Updates(map[string]any{"status": JobRunning, "started_at": time.Now()})
		if res.Error != nil || res.RowsAffected == 0 {
			continue
		}

		m.trackRunning(q.ID, job.ID, job.RetryCount)

		queueCopy := *q
		go m.process(queueCopy, job.ID, job.RetryCount)
	}
}

func (m *Manager) warnNoHandler(q *models.Queue) {
	m.mu.Lock()
	last := m.warned[q.ID]
	now := time.Now()
	if now.Sub(last) < time.Minute {
		m.mu.Unlock()
		return
	}
	m.warned[q.ID] = now
	m.mu.Unlock()
	log.Printf("queue: tube %q has no registered handler — jobs will stay PENDING", q.Name)
}

func (m *Manager) trackRunning(queueID, jobID uint, retryCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running[queueID] == nil {
		m.running[queueID] = make(map[uint]int)
	}
	m.running[queueID][jobID] = retryCount
}

func (m *Manager) releaseRunning(queueID, jobID uint, retryCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if jobs := m.running[queueID]; jobs != nil {
		if currentRetryCount, ok := jobs[jobID]; ok && currentRetryCount == retryCount {
			delete(jobs, jobID)
		}
		if len(jobs) == 0 {
			delete(m.running, queueID)
		}
	}
}

func (m *Manager) process(queue models.Queue, jobID uint, retryCount int) {
	defer m.releaseRunning(queue.ID, jobID, retryCount)

	var job models.QueueJob
	if err := config.DB.First(&job, jobID).Error; err != nil {
		return
	}
	if job.Status != JobRunning || job.RetryCount != retryCount {
		return
	}

	timeout := time.Duration(queue.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fn, ok := getHandler(queue.Name)
	if !ok {
		config.DB.Model(&models.QueueJob{}).Where("id = ?", jobID).
			Updates(map[string]any{"status": JobPending, "started_at": nil})
		return
	}

	start := time.Now()
	result := ""
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
		m.finishJob(queue, job, start, result, err)
	}()

	result, err = fn(ctx, job.Payload)
}

func (m *Manager) finishJob(queue models.Queue, job models.QueueJob, start time.Time, result string, err error) {
	end := time.Now()
	duration := end.Sub(start).Milliseconds()
	if duration < 0 {
		duration = 0
	}

	updates := map[string]any{
		"duration": duration,
		"result":   result,
	}

	if err != nil {
		updates["error"] = err.Error()
		if job.RetryCount < job.MaxRetries {
			updates["status"] = JobPending
			updates["retry_count"] = job.RetryCount + 1
			updates["started_at"] = nil
			updates["completed_at"] = nil
			res := config.DB.Model(&models.QueueJob{}).Where("id = ? AND status = ? AND retry_count = ?", job.ID, JobRunning, job.RetryCount).Updates(updates)
			if res.Error != nil {
				log.Printf("queue: requeue failed job=%d: %v", job.ID, res.Error)
			}
			return
		}

		completedAt := end
		updates["status"] = JobFailed
		updates["completed_at"] = &completedAt
		res := config.DB.Model(&models.QueueJob{}).Where("id = ? AND status = ? AND retry_count = ?", job.ID, JobRunning, job.RetryCount).Updates(updates)
		if res.Error != nil {
			log.Printf("queue: mark failed job=%d: %v", job.ID, res.Error)
			return
		}
		if res.RowsAffected > 0 {
			if err := config.DB.Model(&models.Queue{}).Where("id = ?", queue.ID).
				UpdateColumn("failed_jobs", gorm.Expr("failed_jobs + 1")).Error; err != nil {
				log.Printf("queue: bump failed_jobs queue=%d: %v", queue.ID, err)
			}
		}
		return
	}

	completedAt := end
	updates["status"] = JobSuccess
	updates["error"] = ""
	updates["completed_at"] = &completedAt
	res := config.DB.Model(&models.QueueJob{}).Where("id = ? AND status = ? AND retry_count = ?", job.ID, JobRunning, job.RetryCount).Updates(updates)
	if res.Error != nil {
		log.Printf("queue: mark success job=%d: %v", job.ID, res.Error)
		return
	}
	if res.RowsAffected > 0 {
		if err := config.DB.Model(&models.Queue{}).Where("id = ?", queue.ID).
			UpdateColumn("completed_jobs", gorm.Expr("completed_jobs + 1")).Error; err != nil {
			log.Printf("queue: bump completed_jobs queue=%d: %v", queue.ID, err)
		}
	}
}

// Kick 将单个 FAILED 任务复活回 PENDING（清 error / result / 时间戳，retry_count 归零）
func (m *Manager) Kick(jobID uint) error {
	res := config.DB.Model(&models.QueueJob{}).
		Where("id = ? AND status = ?", jobID, JobFailed).
		Updates(map[string]any{
			"status":       JobPending,
			"error":        "",
			"result":       "",
			"started_at":   nil,
			"completed_at": nil,
			"retry_count":  0,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("job not in FAILED state")
	}
	return nil
}

// KickAll 把队列内所有 FAILED 任务复活为 PENDING，返回影响行数
func (m *Manager) KickAll(queueID uint) (int64, error) {
	res := config.DB.Model(&models.QueueJob{}).
		Where("queue_id = ? AND status = ?", queueID, JobFailed).
		Updates(map[string]any{
			"status":       JobPending,
			"error":        "",
			"result":       "",
			"started_at":   nil,
			"completed_at": nil,
			"retry_count":  0,
		})
	return res.RowsAffected, res.Error
}

// CountRunning 返回某队列当前正在运行的任务数（前端实时状态用）
func (m *Manager) CountRunning(queueID uint) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.running[queueID])
}
