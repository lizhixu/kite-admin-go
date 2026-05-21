package models

type Role struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Code        string       `gorm:"unique;not null" json:"code"`
	Name        string       `gorm:"not null" json:"name"`
	Enable      bool         `gorm:"default:true" json:"enable"`
	Permissions []Permission `gorm:"many2many:role_permissions;" json:"-"`
	Users       []User       `gorm:"many2many:user_roles;" json:"-"`
}
