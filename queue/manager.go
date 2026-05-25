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

	job := models.QueueJob{
		QueueID:    q.ID,
		Payload:    body,
		Status:     JobPending,
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
	running map[uint]int       // queueID -> 当前执行中任务数
	warned  map[uint]time.Time // queueID -> 最近一次「未注册」警告时间（日志限流）
}

var defaultManager *Manager

// Default 返回全局队列管理器（必须在 Init 之后使用）
func Default() *Manager { return defaultManager }

// Init 启动队列管理器。首次启动会清空 queues / queue_jobs 表（本次重构期一次性迁移）
func Init() {
	// 一次性清表：重构稳定后请删除以下两行
	if err := config.DB.Exec("TRUNCATE TABLE queue_jobs").Error; err != nil {
		log.Printf("queue: truncate queue_jobs: %v", err)
	}
	if err := config.DB.Exec("TRUNCATE TABLE queues").Error; err != nil {
		log.Printf("queue: truncate queues: %v", err)
	}

	defaultManager = &Manager{
		stopCh:  make(chan struct{}),
		running: make(map[uint]int),
		warned:  make(map[uint]time.Time),
	}

	// 重新把已注册的 handlers 同步出对应 Queue 行（清表之后需要重建）
	rehydrateHandlers()

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
		m.dispatch(&queues[i])
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
	busy := m.running[q.ID]
	free := concurrency - busy
	m.mu.Unlock()
	if free <= 0 {
		return
	}

	var jobs []models.QueueJob
	if err := config.DB.Where("queue_id = ? AND status = ?", q.ID, JobPending).
		Order("id ASC").Limit(free).Find(&jobs).Error; err != nil {
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

		m.mu.Lock()
		m.running[q.ID]++
		m.mu.Unlock()

		queueCopy := *q
		go m.process(queueCopy, job.ID)
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

func (m *Manager) process(queue models.Queue, jobID uint) {
	defer func() {
		m.mu.Lock()
		m.running[queue.ID]--
		if m.running[queue.ID] < 0 {
			m.running[queue.ID] = 0
		}
		m.mu.Unlock()
	}()

	var job models.QueueJob
	if err := config.DB.First(&job, jobID).Error; err != nil {
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
		// 执行间隙 handler 被注销：退回 PENDING
		config.DB.Model(&models.QueueJob{}).Where("id = ?", jobID).
			Updates(map[string]any{"status": JobPending, "started_at": nil})
		return
	}

	start := time.Now()
	result, err := fn(ctx, job.Payload)
	end := time.Now()

	completedAt := end
	updates := map[string]any{
		"completed_at": &completedAt,
		"duration":     end.Sub(start).Milliseconds(),
		"result":       result,
	}

	if err != nil {
		updates["error"] = err.Error()
		if job.RetryCount < job.MaxRetries {
			updates["status"] = JobPending
			updates["retry_count"] = job.RetryCount + 1
			updates["started_at"] = nil
			updates["completed_at"] = nil
			config.DB.Model(&models.QueueJob{}).Where("id = ?", jobID).Updates(updates)
			return
		}
		updates["status"] = JobFailed
		config.DB.Model(&models.QueueJob{}).Where("id = ?", jobID).Updates(updates)
		config.DB.Model(&models.Queue{}).Where("id = ?", queue.ID).
			UpdateColumn("failed_jobs", gorm.Expr("failed_jobs + 1"))
		return
	}

	updates["status"] = JobSuccess
	updates["error"] = ""
	config.DB.Model(&models.QueueJob{}).Where("id = ?", jobID).Updates(updates)
	config.DB.Model(&models.Queue{}).Where("id = ?", queue.ID).
		UpdateColumn("completed_jobs", gorm.Expr("completed_jobs + 1"))
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
	return m.running[queueID]
}
