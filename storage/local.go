package storage

import (
	"backend/models"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type localStorage struct {
	dir          string
	publicPrefix string
	customDomain string
}

// NewLocal 构建本地存储实例
func NewLocal(cfg *models.StorageConfig) (Storage, error) {
	dir := strings.TrimSpace(cfg.LocalDir)
	if dir == "" {
		dir = "./uploads"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("ensure local dir: %w", err)
	}
	prefix := strings.TrimRight(strings.TrimSpace(cfg.PublicPrefix), "/")
	if prefix == "" {
		prefix = "/uploads"
	}
	domain := strings.TrimRight(strings.TrimSpace(cfg.CustomDomain), "/")
	return &localStorage{dir: dir, publicPrefix: prefix, customDomain: domain}, nil
}

func (l *localStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) (string, error) {
	cleanKey := filepath.ToSlash(filepath.Clean(strings.TrimLeft(key, "/")))
	if cleanKey == "." || strings.Contains(cleanKey, "..") {
		return "", fmt.Errorf("invalid key: %s", key)
	}
	dst := filepath.Join(l.dir, cleanKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	if l.customDomain != "" {
		return l.customDomain + "/" + cleanKey, nil
	}
	return l.publicPrefix + "/" + cleanKey, nil
}

func (l *localStorage) Delete(_ context.Context, key string) error {
	cleanKey := filepath.ToSlash(filepath.Clean(strings.TrimLeft(key, "/")))
	if cleanKey == "." || strings.Contains(cleanKey, "..") {
		return nil
	}
	dst := filepath.Join(l.dir, cleanKey)
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *localStorage) DeletePrefix(_ context.Context, prefix string) error {
	cleanPrefix := filepath.ToSlash(filepath.Clean(strings.TrimLeft(prefix, "/")))
	if cleanPrefix == "" || cleanPrefix == "." || strings.Contains(cleanPrefix, "..") {
		return fmt.Errorf("invalid prefix: %s", prefix)
	}
	target := filepath.Join(l.dir, cleanPrefix)
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
