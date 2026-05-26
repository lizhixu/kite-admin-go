package models

import "time"

type SystemConfig struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SiteName   string    `gorm:"type:varchar(100);default:'Kite Admin'" json:"siteName"`
	Logo       string    `gorm:"type:varchar(500)" json:"logo"`
	Copyright  string    `gorm:"type:varchar(500)" json:"copyright"`
	Favicon    string    `gorm:"type:varchar(500)" json:"favicon"`
	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}