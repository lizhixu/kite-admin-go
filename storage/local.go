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

func (l *localStorage) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	cleanKey, err := cleanLocalKey(key)
	if err != nil {
		return err
	}
	dst := filepath.Join(l.dir, cleanKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func (l *localStorage) PublicURL(key string) string {
	cleanKey, err := cleanLocalKey(key)
	if err != nil {
		return ""
	}
	if l.customDomain != "" {
		return l.customDomain + "/" + cleanKey
	}
	return l.publicPrefix + "/" + cleanKey
}

func (l *localStorage) Delete(_ context.Context, key string) error {
	cleanKey, err := cleanLocalKey(key)
	if err != nil {
		return nil
	}
	dst := filepath.Join(l.dir, cleanKey)
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (l *localStorage) DeletePrefix(_ context.Context, prefix string) error {
	cleanPrefix, err := cleanLocalKey(prefix)
	if err != nil || cleanPrefix == "" {
		return fmt.Errorf("invalid prefix: %s", prefix)
	}
	target := filepath.Join(l.dir, cleanPrefix)
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cleanLocalKey(key string) (string, error) {
	cleanKey := filepath.ToSlash(filepath.Clean(strings.TrimLeft(key, "/")))
	if cleanKey == "." || strings.Contains(cleanKey, "..") {
		return "", fmt.Errorf("invalid key: %s", key)
	}
	return cleanKey, nil
}
