package scheduler

import (
	"backend/config"
	"backend/models"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

// Scheduler 管理所有定时任务的注册与触发
type Scheduler struct {
	cron    *cron.Cron
	mu      sync.Mutex
	entries map[uint]cron.EntryID
}

var defaultScheduler *Scheduler

// Default 返回全局调度器实例（main 初始化后可用）
func Default() *Scheduler { return defaultScheduler }

// Init 创建调度器并加载所有启用的任务
func Init() error {
	defaultScheduler = &Scheduler{
		cron:    cron.New(),
		entries: make(map[uint]cron.EntryID),
	}

	var tasks []models.Task
	if err := config.DB.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return err
	}

	for i := range tasks {
		if err := defaultScheduler.Add(&tasks[i]); err != nil {
			log.Printf("scheduler: skip task %d (%s): %v", tasks[i].ID, tasks[i].Name, err)
		}
	}

	defaultScheduler.cron.Start()
	log.Printf("Scheduler started with %d task(s)", len(defaultScheduler.entries))
	return nil
}

// Stop 优雅停止调度器
func Stop() {
	if defaultScheduler == nil {
		return
	}
	<-defaultScheduler.cron.Stop().Done()
}

// Add 注册或重新注册一个任务到调度器（重复 ID 会先移除再添加）
func (s *Scheduler) Add(task *models.Task) error {
	if task == nil {
		return errors.New("nil task")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addLocked(task.ID, task.Spec)
}

func (s *Scheduler) addLocked(taskID uint, spec string) error {
	if eid, ok := s.entries[taskID]; ok {
		s.cron.Remove(eid)
		delete(s.entries, taskID)
	}
	id, err := s.cron.AddFunc(spec, func() {
		// 每次触发时重新读 DB，保证编辑后立即生效，删除后自动跳过
		var t models.Task
		if err := config.DB.First(&t, taskID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if s := Default(); s != nil {
					s.Remove(taskID)
				}
			}
			return
		}
		if !t.Enabled {
			return
		}
		run(t, "CRON")
	})
	if err != nil {
		return err
	}
	s.entries[taskID] = id
	next := s.cron.Entry(id).Next
	config.DB.Model(&models.Task{}).Where("id = ?", taskID).Update("next_run_at", next)
	return nil
}

// Remove 从调度器移除任务（不影响数据库记录）
func (s *Scheduler) Remove(taskID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if eid, ok := s.entries[taskID]; ok {
		s.cron.Remove(eid)
		delete(s.entries, taskID)
	}
	config.DB.Model(&models.Task{}).Where("id = ?", taskID).Update("next_run_at", nil)
}

// RunOnce 立即异步执行一次任务（手动触发）
func (s *Scheduler) RunOnce(taskID uint) error {
	var task models.Task
	if err := config.DB.First(&task, taskID).Error; err != nil {
		return err
	}
	go run(task, "MANUAL")
	return nil
}

// nextRunFor 返回下一次预计执行时间（若已注册）
func (s *Scheduler) nextRunFor(taskID uint) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	eid, ok := s.entries[taskID]
	if !ok {
		return time.Time{}, false
	}
	return s.cron.Entry(eid).Next, true
}

// ValidateSpec 校验 cron 表达式是否合法
func ValidateSpec(spec string) error {
	_, err := cron.ParseStandard(spec)
	return err
}

// PreviewNext 返回 cron 表达式从 from 时间起接下来 n 次的执行时间
func PreviewNext(spec string, n int) ([]time.Time, error) {
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		n = 5
	}
	out := make([]time.Time, 0, n)
	t := time.Now()
	for i := 0; i < n; i++ {
		t = sched.Next(t)
		out = append(out, t)
	}
	return out, nil
}
