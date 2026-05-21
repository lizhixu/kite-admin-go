package models

type Permission struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"not null" json:"name"`
	Code        string       `gorm:"unique;not null" json:"code"`
	Type        string       `gorm:"not null" json:"type"` // MENU, BUTTON
	ParentID    *uint        `json:"parentId"`
	Path        *string      `json:"path"`
	Redirect    *string      `json:"redirect"`
	Icon        *string      `json:"icon"`
	Component   *string      `json:"component"`
	Layout      *string      `json:"layout"`
	KeepAlive   *bool        `json:"keepAlive"`
	Method      *string      `json:"method"`
	Description *string      `json:"description"`
	Show        *bool        `gorm:"default:true" json:"show"`
	Enable      *bool        `gorm:"default:true" json:"enable"`
	Order       int          `gorm:"default:0" json:"order"`
	Children    []Permission `gorm:"-" json:"children,omitempty"`
}
