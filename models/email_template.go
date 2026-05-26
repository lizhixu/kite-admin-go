package models

import "time"

type EmailTemplate struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Scene      string    `gorm:"type:varchar(50);uniqueIndex;not null" json:"scene"`
	Name       string    `gorm:"type:varchar(100);not null" json:"name"`
	Subject    string    `gorm:"type:varchar(255)" json:"subject"`
	Content    string    `gorm:"type:longtext" json:"content"`
	IsBuiltin  bool      `gorm:"default:false" json:"isBuiltin"`
	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}
