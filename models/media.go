package models

import "time"

// Media 媒体文件元数据
type Media struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Filename     string    `gorm:"type:varchar(255);not null" json:"filename"`
	StorageKey   string    `gorm:"type:varchar(512);not null" json:"storageKey"`
	StorageType  string    `gorm:"type:varchar(16);not null" json:"storageType"` // LOCAL / S3
	ConfigID     uint      `gorm:"index" json:"configId"`
	FolderID     uint      `gorm:"index;default:0" json:"folderId"`       // 0 = 该存储的根目录
	FolderPath   string    `gorm:"type:varchar(1024)" json:"folderPath"`  // 冗余，便于前端展示与过滤
	BizType      string    `gorm:"type:varchar(32);index" json:"bizType"` // 空 = 媒体库素材；其它如 avatar/rich-text/message-attach 为业务上传
	LegacyURL    string    `gorm:"column:url;type:varchar(1024);not null" json:"-"`
	AccessURL    string    `gorm:"-" json:"accessUrl"`
	URL          string    `gorm:"-" json:"url,omitempty"`
	MimeType     string    `gorm:"type:varchar(128)" json:"mimeType"`
	Ext          string    `gorm:"type:varchar(16)" json:"ext"`
	Size         int64     `json:"size"`
	UploaderID   uint      `gorm:"index" json:"uploaderId"`
	UploaderName string    `gorm:"type:varchar(64)" json:"uploaderName"`
	CreateTime   time.Time `gorm:"autoCreateTime" json:"createTime"`
}

// MediaFolder 媒体文件夹（每个存储独立树）
type MediaFolder struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Name       string    `gorm:"type:varchar(128);not null" json:"name"`
	ParentID   *uint     `gorm:"index" json:"parentId"` // nil = 根
	ConfigID   uint      `gorm:"index;not null" json:"configId"`
	Path       string    `gorm:"type:varchar(1024);not null;index" json:"path"` // 如 "photos/2024"
	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}

func (MediaFolder) TableName() string { return "media_folders" }

// StorageConfig 存储后端配置
type StorageConfig struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Name string `gorm:"type:varchar(64);not null" json:"name"`
	Type string `gorm:"type:varchar(16);not null" json:"type"` // LOCAL / S3

	// S3
	Endpoint     string `gorm:"type:varchar(255)" json:"endpoint"`
	Region       string `gorm:"type:varchar(64)"  json:"region"`
	Bucket       string `gorm:"type:varchar(128)" json:"bucket"`
	AccessKey    string `gorm:"type:varchar(255)" json:"accessKey"`
	SecretKey    string `gorm:"type:varchar(255)" json:"-"`
	UseSSL       bool   `gorm:"default:true" json:"useSSL"`
	CustomDomain string `gorm:"type:varchar(255)" json:"customDomain"`

	// LOCAL
	LocalDir     string `gorm:"type:varchar(255)" json:"localDir"`
	PublicPrefix string `gorm:"type:varchar(255)" json:"publicPrefix"`

	// 通用
	AllowExtensions string `gorm:"type:varchar(512)" json:"allowExtensions"` // 逗号分隔，空表示不限
	MaxSizeMB       int    `gorm:"default:50" json:"maxSizeMB"`
	Enabled         bool   `gorm:"default:true" json:"enabled"`
	IsDefault       bool   `gorm:"default:false" json:"isDefault"`

	CreateTime time.Time `gorm:"autoCreateTime" json:"createTime"`
	UpdateTime time.Time `gorm:"autoUpdateTime" json:"updateTime"`
}

func (StorageConfig) TableName() string { return "storage_configs" }
