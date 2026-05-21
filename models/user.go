package models

import "time"

type User struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Username   string    `gorm:"unique;not null" json:"username"`
	Password   string    `gorm:"not null" json:"-"`
	Enable     bool      `gorm:"default:true" json:"enable"`
	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime"`
	Profile    *Profile  `gorm:"foreignKey:UserID" json:"profile,omitempty"`
	Roles      []Role    `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

type Profile struct {
	ID       uint    `gorm:"primaryKey" json:"id"`
	NickName string  `json:"nickName"`
	Gender   *string `json:"gender"`
	Avatar   string  `json:"avatar"`
	Address  *string `json:"address"`
	Email    *string `json:"email"`
	UserID   uint    `json:"userId"`
}
