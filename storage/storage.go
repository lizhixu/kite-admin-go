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
	// Put 写入对象
	Put(ctx context.Context, key string, r io.Reader, size int64, mime string) error
	// PublicURL 根据对象 key 生成当前配置下的公开访问 URL
	PublicURL(key string) string
	// Delete 删除对象（不存在不报错）
	Delete(ctx context.Context, key string) error
	// DeletePrefix 删除指定前缀（文件夹）下的所有对象
	DeletePrefix(ctx context.Context, prefix string) error
}

// BuildPublicURL 根据存储配置和对象 key 生成当前公开访问 URL。
func BuildPublicURL(cfg *models.StorageConfig, key string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("storage config is nil")
	}
	switch strings.ToUpper(cfg.Type) {
	case "LOCAL":
		cleanKey, err := cleanLocalKey(key)
		if err != nil {
			return "", err
		}
		domain := strings.TrimRight(strings.TrimSpace(cfg.CustomDomain), "/")
		if domain != "" {
			return domain + "/" + cleanKey, nil
		}
		prefix := strings.TrimRight(strings.TrimSpace(cfg.PublicPrefix), "/")
		if prefix == "" {
			prefix = "/uploads"
		}
		return prefix + "/" + cleanKey, nil
	case "S3":
		rawEndpoint := strings.TrimSpace(cfg.Endpoint)
		endpoint := stripScheme(rawEndpoint)
		if endpoint == "" {
			return "", fmt.Errorf("endpoint required")
		}
		if cfg.Bucket == "" {
			return "", fmt.Errorf("bucket required")
		}
		escapedKey := strings.Join(splitAndEscape(strings.TrimLeft(key, "/")), "/")
		domain := strings.TrimRight(strings.TrimSpace(cfg.CustomDomain), "/")
		if domain != "" {
			return domain + "/" + escapedKey, nil
		}
		useSSL := cfg.UseSSL
		if hasScheme(rawEndpoint) {
			useSSL = strings.HasPrefix(rawEndpoint, "https://")
		}
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s/%s/%s", scheme, endpoint, cfg.Bucket, escapedKey), nil
	default:
		return "", fmt.Errorf("unsupported storage type: %s", cfg.Type)
	}
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
