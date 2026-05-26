package models

import "time"

type LoginLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"userId"`
	Username   string    `gorm:"type:varchar(50);index" json:"username"`
	IP         string    `gorm:"type:varchar(50)" json:"ip"`
	UserAgent  string    `gorm:"type:varchar(255)" json:"userAgent"`
	Success    bool      `gorm:"index" json:"success"`
	Message    string    `gorm:"type:varchar(255)" json:"message"`
	CreateTime time.Time `gorm:"autoCreateTime;index" json:"createTime"`
}
