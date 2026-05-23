package models

import "time"

// Task 定时任务定义
type Task struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"type:varchar(100);not null" json:"name"`
	Spec        string     `gorm:"type:varchar(64);not null" json:"spec"` // 标准 5 段 cron 表达式
	Type        string     `gorm:"type:varchar(16);not null" json:"type"` // HTTP / SHELL / FUNC
	Command     string     `gorm:"type:text;not null" json:"command"`     // URL / shell 命令 / 内置函数名
	HttpMethod  string     `gorm:"type:varchar(8)" json:"httpMethod,omitempty"`
	HttpHeaders string     `gorm:"type:text" json:"httpHeaders,omitempty"` // JSON 字符串：{"K":"V"}
	HttpBody    string     `gorm:"type:text" json:"httpBody,omitempty"`
	Timeout     int        `gorm:"default:60" json:"timeout"` // 秒
	Enabled     bool       `gorm:"default:true" json:"enabled"`
	Description string     `gorm:"type:varchar(255)" json:"description"`
	LastRunAt   *time.Time `json:"lastRunAt"`
	NextRunAt   *time.Time `json:"nextRunAt"`
	CreateTime  time.Time  `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime  time.Time  `gorm:"autoUpdateTime" json:"updateTime"`
}

// TaskLog 任务执行记录
type TaskLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    uint      `gorm:"index" json:"taskId"`
	TaskName  string    `gorm:"type:varchar(100)" json:"taskName"`
	Status    string    `gorm:"type:varchar(16)" json:"status"`  // SUCCESS / FAILED / TIMEOUT
	Trigger   string    `gorm:"type:varchar(16)" json:"trigger"` // CRON / MANUAL
	Output    string    `gorm:"type:longtext" json:"output"`
	Error     string    `gorm:"type:text" json:"error"`
	Steps     string    `gorm:"type:longtext" json:"steps"` // JSON: [{time, level, message}]
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Duration  int64     `json:"duration"` // 毫秒
}
