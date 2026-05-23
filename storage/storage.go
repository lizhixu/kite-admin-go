package storage

import (
	"backend/models"
	"context"
	"fmt"
	"io"
	"strings"
)

// Storage 存储后端抽象
type Storage interface {
	// Put 写入对象，返回完整可访问 URL
	Put(ctx context.Context, key string, r io.Reader, size int64, mime string) (publicURL string, err error)
	// Delete 删除对象（不存在不报错）
	Delete(ctx context.Context, key string) error
	// DeletePrefix 删除指定前缀（文件夹）下的所有对象
	DeletePrefix(ctx context.Context, prefix string) error
}

// New 根据配置构建对应后端实例
func New(cfg *models.StorageConfig) (Storage, error) {
	if cfg == nil {
		return nil, fmt.Errorf("storage config is nil")
	}
	switch strings.ToUpper(cfg.Type) {
	case "LOCAL":
		return NewLocal(cfg)
	case "S3":
		return NewS3(cfg)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
}
