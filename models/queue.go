package models

import "time"

// Queue 队列定义（tube）
// 由代码 queue.Handle() / queue.Push() 创建；前端只负责运维（暂停 / kick / 清理）。
type Queue struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"name"`
	Description   string    `gorm:"type:varchar(255)" json:"description"`
	Status        string    `gorm:"type:varchar(16);default:'RUNNING';index" json:"status"` // RUNNING / PAUSED
	Concurrency   int       `gorm:"default:3" json:"concurrency"`
	Timeout       int       `gorm:"default:60" json:"timeout"`
	MaxRetries    int       `json:"maxRetries"`
	TotalJobs     int64     `gorm:"default:0" json:"totalJobs"`
	CompletedJobs int64     `gorm:"default:0" json:"completedJobs"`
	FailedJobs    int64     `gorm:"default:0" json:"failedJobs"`
	CreateTime    time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime    time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}

// QueueJob 队列中的任务单元
type QueueJob struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	QueueID     uint       `gorm:"not null;index:idx_queue_unique_key,priority:1" json:"queueId"`
	Payload     string     `gorm:"type:longtext" json:"payload"`
	Status      string     `gorm:"type:varchar(16);default:'PENDING';index" json:"status"`
	Priority    int        `gorm:"default:0;index" json:"priority"`
	DelayUntil  *time.Time `gorm:"index" json:"delayUntil"`
	UniqueKey   string     `gorm:"type:varchar(191);index:idx_queue_unique_key,priority:2" json:"uniqueKey"`
	Result      string     `gorm:"type:longtext" json:"result"`
	Error       string     `gorm:"type:text" json:"error"`
	RetryCount  int        `gorm:"default:0" json:"retryCount"`
	MaxRetries  int        `gorm:"default:0" json:"maxRetries"`
	Duration    int64      `json:"duration"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"completedAt"`
}

func (QueueJob) TableName() string { return "queue_jobs" }
