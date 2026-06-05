package storage

import (
	"backend/models"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type s3Storage struct {
	client       *minio.Client
	bucket       string
	endpoint     string // host[:port] 已去除 scheme
	useSSL       bool
	customDomain string
}

// NewS3 构建 S3 协议存储实例（兼容 MinIO / 阿里 OSS / 腾讯 COS / 七牛 等）
func NewS3(cfg *models.StorageConfig) (Storage, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket required")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("accessKey/secretKey required")
	}
	// minio-go 要求 endpoint 不带 scheme
	rawEndpoint := endpoint
	endpoint = stripScheme(endpoint)

	useSSL := cfg.UseSSL
	if hasScheme(rawEndpoint) {
		useSSL = strings.HasPrefix(rawEndpoint, "https://")
	}

	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: useSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}
	return &s3Storage{
		client:       cli,
		bucket:       cfg.Bucket,
		endpoint:     endpoint,
		useSSL:       useSSL,
		customDomain: strings.TrimRight(strings.TrimSpace(cfg.CustomDomain), "/"),
	}, nil
}

func (s *s3Storage) Put(ctx context.Context, key string, r io.Reader, size int64, mime string) error {
	if size <= 0 {
		size = -1 // 让 minio-go 走分片，未知大小
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: mime,
	})
	return err
}

func (s *s3Storage) PublicURL(key string) string {
	return s.buildURL(key)
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *s3Storage) DeletePrefix(ctx context.Context, prefix string) error {
	p := strings.TrimLeft(prefix, "/")
	if p == "" {
		return fmt.Errorf("empty prefix")
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	objCh := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    p,
		Recursive: true,
	})
	for errCh := range s.client.RemoveObjects(ctx, s.bucket, objCh, minio.RemoveObjectsOptions{}) {
		if errCh.Err != nil {
			return fmt.Errorf("remove %s: %w", errCh.ObjectName, errCh.Err)
		}
	}
	return nil
}

func (s *s3Storage) buildURL(key string) string {
	escapedKey := strings.Join(splitAndEscape(key), "/")
	if s.customDomain != "" {
		return s.customDomain + "/" + escapedKey
	}
	scheme := "http"
	if s.useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, s.endpoint, s.bucket, escapedKey)
}

func splitAndEscape(key string) []string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return parts
}

func hasScheme(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func stripScheme(s string) string {
	if strings.HasPrefix(s, "http://") {
		return strings.TrimPrefix(s, "http://")
	}
	if strings.HasPrefix(s, "https://") {
		return strings.TrimPrefix(s, "https://")
	}
	return s
}
