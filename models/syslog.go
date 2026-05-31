package models

import "time"

type SysLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `json:"userId"`
	Username   string    `gorm:"type:varchar(50);index" json:"username"`
	Method     string    `gorm:"type:varchar(10)" json:"method"`
	Path       string    `gorm:"type:varchar(255)" json:"path"`
	Params     string    `gorm:"type:longtext" json:"params"`
	Response   string    `gorm:"type:longtext" json:"response"`
	IP         string    `gorm:"type:varchar(50)" json:"ip"`
	StatusCode int       `gorm:"index" json:"statusCode"`
	Latency    int64     `json:"latency"` // in milliseconds
	UserAgent  string    `gorm:"type:varchar(255)" json:"userAgent"`
	CreateTime time.Time `gorm:"autoCreateTime;index" json:"createTime"`
}
