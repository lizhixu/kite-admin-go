package models

import "time"

type Message struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"type:varchar(255);not null" json:"title"`
	Content     string    `gorm:"type:longtext" json:"content"`
	Type        string    `gorm:"type:varchar(20);default:SYSTEM;index" json:"type"`
	SenderID    uint      `json:"senderId"`
	SenderName  string    `gorm:"type:varchar(50)" json:"senderName"`
	TargetType  string    `gorm:"type:varchar(20);not null" json:"targetType"`
	TargetRoles string    `gorm:"type:varchar(500)" json:"targetRoles"` // JSON array of role IDs, e.g. "[1,2]"
	Status      string    `gorm:"type:varchar(20);default:DRAFT;index" json:"status"`
	CreateTime  time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime  time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}

type MessageRecipient struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	MessageID uint       `gorm:"index;constraint:OnDelete:CASCADE" json:"messageId"`
	UserID    uint       `gorm:"index" json:"userId"`
	IsRead    bool       `gorm:"default:false" json:"isRead"`
	ReadAt    *time.Time `json:"readAt"`
	Emailed   bool       `gorm:"default:false" json:"emailed"`
}

type EmailConfig struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	Host      string `gorm:"type:varchar(255)" json:"host"`
	Port      int    `gorm:"default:587" json:"port"`
	Username  string `gorm:"type:varchar(255)" json:"username"`
	Password  string `gorm:"type:varchar(255)" json:"-"`
	FromName  string `gorm:"type:varchar(100)" json:"fromName"`
	FromEmail string `gorm:"type:varchar(255)" json:"fromEmail"`
	Enabled   bool   `gorm:"default:false" json:"enabled"`
}
