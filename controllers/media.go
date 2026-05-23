package controllers

import (
	"backend/config"
	"backend/models"
	"backend/storage"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type MediaController struct{}

// --- 媒体 ---

// Upload 处理 multipart/form-data 上传
func (mc *MediaController) Upload(c *gin.Context) {
	configIDStr := c.PostForm("configId")
	folderIDStr := c.PostForm("folderId")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		respondErr(c, 400, "file required: "+err.Error())
		return
	}

	cfg, err := pickStorageConfig(configIDStr)
	if err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	if !cfg.Enabled {
		respondErr(c, 400, "storage config disabled")
		return
	}

	folder, err := resolveFolder(folderIDStr, cfg.ID)
	if err != nil {
		respondErr(c, 400, err.Error())
		return
	}

	originalName := fileHeader.Filename
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(originalName)), ".")
	if cfg.AllowExtensions != "" {
		allow := splitCSV(cfg.AllowExtensions)
		if !containsCI(allow, ext) {
			respondErr(c, 400, "extension not allowed: "+ext)
			return
		}
	}
	if cfg.MaxSizeMB > 0 && fileHeader.Size > int64(cfg.MaxSizeMB)*1024*1024 {
		respondErr(c, 400, fmt.Sprintf("file too large (>%dMB)", cfg.MaxSizeMB))
		return
	}

	store, err := storage.New(cfg)
	if err != nil {
		respondErr(c, 500, "init storage: "+err.Error())
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		respondErr(c, 500, "open upload: "+err.Error())
		return
	}
	defer src.Close()

	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	filename := id
	if ext != "" {
		filename += "." + ext
	}
	folderPath := ""
	if folder != nil {
		folderPath = folder.Path
	}
	key := filename
	if folderPath != "" {
		key = folderPath + "/" + filename
	}

	mime := detectMime(fileHeader)
	url, err := store.Put(c, key, src, fileHeader.Size, mime)
	if err != nil {
		respondErr(c, 500, "store: "+err.Error())
		return
	}

	uploaderID, _ := c.Get("userID")
	uploaderName, _ := c.Get("username")
	uid, _ := uploaderID.(uint)
	uname, _ := uploaderName.(string)

	var folderID uint
	if folder != nil {
		folderID = folder.ID
	}
	rec := models.Media{
		Filename:     originalName,
		StorageKey:   key,
		StorageType:  cfg.Type,
		ConfigID:     cfg.ID,
		FolderID:     folderID,
		FolderPath:   folderPath,
		Url:          url,
		MimeType:     mime,
		Ext:          ext,
		Size:         fileHeader.Size,
		UploaderID:   uid,
		UploaderName: uname,
	}
	if err := config.DB.Create(&rec).Error; err != nil {
		_ = store.Delete(c, key)
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, rec)
}

// GetPage 分页查询媒体列表
func (mc *MediaController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "24"))
	filename := c.Query("filename")
	mimePrefix := c.Query("mimePrefix") // image/, video/, application/
	storageType := c.Query("storageType")
	configIDStr := c.Query("configId")
	folderIDStr := c.Query("folderId")
	scope := c.Query("scope") // mine(默认) / all

	q := config.DB.Model(&models.Media{})
	if filename != "" {
		q = q.Where("filename LIKE ?", "%"+filename+"%")
	}
	if mimePrefix != "" {
		q = q.Where("mime_type LIKE ?", mimePrefix+"%")
	}
	if storageType != "" {
		q = q.Where("storage_type = ?", storageType)
	}
	if configIDStr != "" {
		if id, err := strconv.Atoi(configIDStr); err == nil {
			q = q.Where("config_id = ?", id)
		}
	}
	if folderIDStr != "" {
		if id, err := strconv.Atoi(folderIDStr); err == nil {
			q = q.Where("folder_id = ?", id)
		}
	}
	// 默认仅展示自己上传的文件;有 ViewAllMedia 权限的用户显式传 scope=all 才能跨用户
	if scope != "all" || !canViewAllMedia(c) {
		q = q.Where("uploader_id = ?", currentUserID(c))
	}

	var total int64
	q.Count(&total)

	var rows []models.Media
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// Delete 删除单个媒体
func (mc *MediaController) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var m models.Media
	if err := config.DB.First(&m, id).Error; err != nil {
		respondErr(c, 404, "media not found")
		return
	}
	if m.UploaderID != currentUserID(c) && !canViewAllMedia(c) {
		respondErr(c, 403, "not the owner of this file")
		return
	}
	if err := deleteMedia(c, &m); err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// BulkDelete 批量删除
func (mc *MediaController) BulkDelete(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}
	var rows []models.Media
	if err := config.DB.Where("id IN ?", req.IDs).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	if !canViewAllMedia(c) {
		uid := currentUserID(c)
		for _, m := range rows {
			if m.UploaderID != uid {
				respondErr(c, 403, "contains files you don't own")
				return
			}
		}
	}
	for i := range rows {
		_ = deleteMedia(c, &rows[i])
	}
	respondOK(c, len(rows))
}

// MoveMedia 批量移动到目标文件夹（仅改 DB，不动物理文件）
func (mc *MediaController) MoveMedia(c *gin.Context) {
	var req struct {
		IDs      []uint `json:"ids" binding:"required"`
		FolderID uint   `json:"folderId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}
	// 所有权校验
	var rows []models.Media
	if err := config.DB.Where("id IN ?", req.IDs).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	if !canViewAllMedia(c) {
		uid := currentUserID(c)
		for _, m := range rows {
			if m.UploaderID != uid {
				respondErr(c, 403, "contains files you don't own")
				return
			}
		}
	}
	folderPath := ""
	if req.FolderID > 0 {
		var f models.MediaFolder
		if err := config.DB.First(&f, req.FolderID).Error; err != nil {
			respondErr(c, 404, "folder not found")
			return
		}
		folderPath = f.Path
		// 同一存储校验
		if len(rows) > 0 && rows[0].ConfigID != f.ConfigID {
			respondErr(c, 400, "folder belongs to a different storage")
			return
		}
	}
	err := config.DB.Model(&models.Media{}).Where("id IN ?", req.IDs).Updates(map[string]any{
		"folder_id":   req.FolderID,
		"folder_path": folderPath,
	}).Error
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, len(req.IDs))
}

// --- 文件夹 ---

// ListFolders 返回指定存储的文件夹列表（扁平，前端自行构建树）
func (mc *MediaController) ListFolders(c *gin.Context) {
	configIDStr := c.Query("configId")
	q := config.DB.Model(&models.MediaFolder{})
	if configIDStr != "" {
		if id, err := strconv.Atoi(configIDStr); err == nil {
			q = q.Where("config_id = ?", id)
		}
	}
	var rows []models.MediaFolder
	if err := q.Order("path ASC").Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, rows)
}

// CreateFolder 新建文件夹
func (mc *MediaController) CreateFolder(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		ParentID *uint  `json:"parentId"`
		ConfigID uint   `json:"configId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.ContainsAny(name, "/\\") {
		respondErr(c, 400, "invalid folder name")
		return
	}
	// 校验存储存在
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, req.ConfigID).Error; err != nil {
		respondErr(c, 404, "storage config not found")
		return
	}
	parentPath := ""
	if req.ParentID != nil && *req.ParentID > 0 {
		var parent models.MediaFolder
		if err := config.DB.First(&parent, *req.ParentID).Error; err != nil {
			respondErr(c, 404, "parent folder not found")
			return
		}
		if parent.ConfigID != req.ConfigID {
			respondErr(c, 400, "parent belongs to a different storage")
			return
		}
		parentPath = parent.Path
	}
	folderPath := name
	if parentPath != "" {
		folderPath = parentPath + "/" + name
	}
	// 同级唯一
	var dup int64
	q := config.DB.Model(&models.MediaFolder{}).Where("config_id = ? AND name = ?", req.ConfigID, name)
	if req.ParentID != nil && *req.ParentID > 0 {
		q = q.Where("parent_id = ?", *req.ParentID)
	} else {
		q = q.Where("parent_id IS NULL")
	}
	q.Count(&dup)
	if dup > 0 {
		respondErr(c, 400, "folder already exists")
		return
	}
	folder := models.MediaFolder{
		Name:     name,
		ParentID: req.ParentID,
		ConfigID: req.ConfigID,
		Path:     folderPath,
	}
	if err := config.DB.Create(&folder).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, folder)
}

// RenameFolder 重命名文件夹（联动更新本节点与所有后代的 Path，以及关联 Media.folder_path）
// 物理存储不做移动以避免半成品状态——下一次上传按新 Path，已上传文件的 storageKey 保持原值。
func (mc *MediaController) RenameFolder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || strings.ContainsAny(name, "/\\") {
		respondErr(c, 400, "invalid folder name")
		return
	}
	var folder models.MediaFolder
	if err := config.DB.First(&folder, id).Error; err != nil {
		respondErr(c, 404, "folder not found")
		return
	}
	if name == folder.Name {
		respondOK(c, folder)
		return
	}
	parentPath := ""
	if idx := strings.LastIndex(folder.Path, "/"); idx >= 0 {
		parentPath = folder.Path[:idx]
	}
	newPath := name
	if parentPath != "" {
		newPath = parentPath + "/" + name
	}
	// 同级唯一
	var dup int64
	q := config.DB.Model(&models.MediaFolder{}).Where("config_id = ? AND name = ? AND id <> ?", folder.ConfigID, name, folder.ID)
	if folder.ParentID != nil {
		q = q.Where("parent_id = ?", *folder.ParentID)
	} else {
		q = q.Where("parent_id IS NULL")
	}
	q.Count(&dup)
	if dup > 0 {
		respondErr(c, 400, "folder already exists")
		return
	}

	oldPath := folder.Path
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// 本节点
		if err := tx.Model(&folder).Updates(map[string]any{"name": name, "path": newPath}).Error; err != nil {
			return err
		}
		// 后代节点 Path 前缀替换
		oldPrefix := oldPath + "/"
		newPrefix := newPath + "/"
		// 取出所有后代再单独更新，跨方言安全
		var descendants []models.MediaFolder
		if err := tx.Where("config_id = ? AND path LIKE ?", folder.ConfigID, oldPrefix+"%").Find(&descendants).Error; err != nil {
			return err
		}
		for i := range descendants {
			d := &descendants[i]
			d.Path = newPrefix + strings.TrimPrefix(d.Path, oldPrefix)
			if err := tx.Model(d).Update("path", d.Path).Error; err != nil {
				return err
			}
		}
		// 媒体 folder_path 更新（本文件夹直属）
		if err := tx.Model(&models.Media{}).Where("folder_id = ?", folder.ID).Update("folder_path", newPath).Error; err != nil {
			return err
		}
		// 后代文件夹下的媒体
		for _, d := range descendants {
			if err := tx.Model(&models.Media{}).Where("folder_id = ?", d.ID).Update("folder_path", d.Path).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	folder.Name = name
	folder.Path = newPath
	respondOK(c, folder)
}

// DeleteFolder 删除文件夹；默认仅允许空文件夹，传 ?cascade=1 级联删
func (mc *MediaController) DeleteFolder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	cascade := c.Query("cascade") == "1"
	var folder models.MediaFolder
	if err := config.DB.First(&folder, id).Error; err != nil {
		respondErr(c, 404, "folder not found")
		return
	}
	// 找出本节点与所有后代
	var allFolders []models.MediaFolder
	allFolders = append(allFolders, folder)
	var descendants []models.MediaFolder
	if err := config.DB.Where("config_id = ? AND path LIKE ?", folder.ConfigID, folder.Path+"/%").Find(&descendants).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	allFolders = append(allFolders, descendants...)
	folderIDs := make([]uint, 0, len(allFolders))
	for _, f := range allFolders {
		folderIDs = append(folderIDs, f.ID)
	}
	var mediaCount int64
	config.DB.Model(&models.Media{}).Where("folder_id IN ?", folderIDs).Count(&mediaCount)

	if !cascade && (len(descendants) > 0 || mediaCount > 0) {
		respondErr(c, 400, "folder is not empty; pass ?cascade=1 to force delete")
		return
	}

	// 清理物理对象
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, folder.ConfigID).Error; err == nil {
		if store, err := storage.New(&cfg); err == nil {
			if err := store.DeletePrefix(c, folder.Path); err != nil {
				respondErr(c, 500, "remove storage prefix: "+err.Error())
				return
			}
		}
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("folder_id IN ?", folderIDs).Delete(&models.Media{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", folderIDs).Delete(&models.MediaFolder{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// --- 存储配置 ---

type storageConfigRequest struct {
	Name            string `json:"name" binding:"required"`
	Type            string `json:"type" binding:"required,oneof=LOCAL S3"`
	Endpoint        string `json:"endpoint"`
	Region          string `json:"region"`
	Bucket          string `json:"bucket"`
	AccessKey       string `json:"accessKey"`
	SecretKey       string `json:"secretKey"`
	UseSSL          *bool  `json:"useSSL"`
	CustomDomain    string `json:"customDomain"`
	LocalDir        string `json:"localDir"`
	PublicPrefix    string `json:"publicPrefix"`
	AllowExtensions string `json:"allowExtensions"`
	MaxSizeMB       int    `json:"maxSizeMB"`
	Enabled         *bool  `json:"enabled"`
	IsDefault       *bool  `json:"isDefault"`
}

func (mc *MediaController) ListConfigs(c *gin.Context) {
	var rows []models.StorageConfig
	if err := config.DB.Order("is_default DESC, id ASC").Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, rows)
}

func (mc *MediaController) CreateConfig(c *gin.Context) {
	var req storageConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	cfg := models.StorageConfig{
		Name:            req.Name,
		Type:            req.Type,
		Endpoint:        req.Endpoint,
		Region:          req.Region,
		Bucket:          req.Bucket,
		AccessKey:       req.AccessKey,
		SecretKey:       req.SecretKey,
		CustomDomain:    req.CustomDomain,
		LocalDir:        req.LocalDir,
		PublicPrefix:    req.PublicPrefix,
		AllowExtensions: req.AllowExtensions,
		MaxSizeMB:       req.MaxSizeMB,
	}
	if req.UseSSL != nil {
		cfg.UseSSL = *req.UseSSL
	} else {
		cfg.UseSSL = true
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	} else {
		cfg.Enabled = true
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 50
	}

	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault != nil && *req.IsDefault {
			if err := tx.Model(&models.StorageConfig{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
			cfg.IsDefault = true
		}
		return tx.Create(&cfg).Error
	})
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, cfg)
}

func (mc *MediaController) UpdateConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req storageConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, id).Error; err != nil {
		respondErr(c, 404, "config not found")
		return
	}
	cfg.Name = req.Name
	cfg.Type = req.Type
	cfg.Endpoint = req.Endpoint
	cfg.Region = req.Region
	cfg.Bucket = req.Bucket
	cfg.AccessKey = req.AccessKey
	if req.SecretKey != "" { // 空字符串保留原值
		cfg.SecretKey = req.SecretKey
	}
	cfg.CustomDomain = req.CustomDomain
	cfg.LocalDir = req.LocalDir
	cfg.PublicPrefix = req.PublicPrefix
	cfg.AllowExtensions = req.AllowExtensions
	if req.MaxSizeMB > 0 {
		cfg.MaxSizeMB = req.MaxSizeMB
	}
	if req.UseSSL != nil {
		cfg.UseSSL = *req.UseSSL
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if req.IsDefault != nil {
			if *req.IsDefault {
				if err := tx.Model(&models.StorageConfig{}).Where("id <> ?", cfg.ID).Update("is_default", false).Error; err != nil {
					return err
				}
				cfg.IsDefault = true
			} else {
				cfg.IsDefault = false
			}
		}
		return tx.Save(&cfg).Error
	})
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, cfg)
}

func (mc *MediaController) DeleteConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, id).Error; err != nil {
		respondErr(c, 404, "config not found")
		return
	}
	if cfg.IsDefault {
		respondErr(c, 400, "cannot delete default config; set another as default first")
		return
	}
	var inUse int64
	config.DB.Model(&models.Media{}).Where("config_id = ?", cfg.ID).Count(&inUse)
	if inUse > 0 {
		respondErr(c, 400, fmt.Sprintf("config in use by %d media files", inUse))
		return
	}
	if err := config.DB.Delete(&cfg).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

func (mc *MediaController) SetDefault(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		var cfg models.StorageConfig
		if err := tx.First(&cfg, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.StorageConfig{}).Where("id <> ?", cfg.ID).Update("is_default", false).Error; err != nil {
			return err
		}
		cfg.IsDefault = true
		return tx.Save(&cfg).Error
	})
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

func (mc *MediaController) TestConfig(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, id).Error; err != nil {
		respondErr(c, 404, "config not found")
		return
	}
	store, err := storage.New(&cfg)
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	probeKey := fmt.Sprintf(".probe/%d_%d.txt", time.Now().UnixNano(), cfg.ID)
	start := time.Now()
	body := []byte("ok")
	if _, err := store.Put(c, probeKey, bytes.NewReader(body), int64(len(body)), "text/plain"); err != nil {
		respondErr(c, 500, "put probe: "+err.Error())
		return
	}
	_ = store.Delete(c, probeKey)
	respondOK(c, gin.H{"ok": true, "elapsedMs": time.Since(start).Milliseconds()})
}

func pickStorageConfig(idStr string) (*models.StorageConfig, error) {
	var cfg models.StorageConfig
	if idStr != "" {
		id, _ := strconv.Atoi(idStr)
		if err := config.DB.First(&cfg, id).Error; err != nil {
			return nil, fmt.Errorf("storage config not found: %d", id)
		}
		return &cfg, nil
	}
	if err := config.DB.Where("is_default = ?", true).First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no default storage config; please configure one first")
		}
		return nil, err
	}
	return &cfg, nil
}

// resolveFolder 返回 folder 或 nil（表示根目录）；校验文件夹归属于该存储
func resolveFolder(idStr string, configID uint) (*models.MediaFolder, error) {
	if idStr == "" || idStr == "0" {
		return nil, nil
	}
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		return nil, nil
	}
	var f models.MediaFolder
	if err := config.DB.First(&f, id).Error; err != nil {
		return nil, fmt.Errorf("folder not found: %d", id)
	}
	if f.ConfigID != configID {
		return nil, fmt.Errorf("folder belongs to a different storage")
	}
	return &f, nil
}

func deleteMedia(c *gin.Context, m *models.Media) error {
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, m.ConfigID).Error; err == nil {
		if store, err := storage.New(&cfg); err == nil {
			_ = store.Delete(c, m.StorageKey)
		}
	}
	return config.DB.Delete(m).Error
}

func detectMime(fh *multipart.FileHeader) string {
	if t := fh.Header.Get("Content-Type"); t != "" {
		return t
	}
	f, err := fh.Open()
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	if n == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

func containsCI(list []string, s string) bool {
	low := strings.ToLower(s)
	for _, v := range list {
		if v == low {
			return true
		}
	}
	return false
}

// --- 所有权 & 可见范围 ---

// currentUserID 从 gin.Context 读取当前用户 ID
func currentUserID(c *gin.Context) uint {
	if v, ok := c.Get("userID"); ok {
		if id, ok2 := v.(uint); ok2 {
			return id
		}
	}
	return 0
}

// canViewAllMedia SUPER_ADMIN 或拥有 ViewAllMedia 权限的角色可以跨用户查看/管理媒体
func canViewAllMedia(c *gin.Context) bool {
	roleCode := c.GetString("roleCode")
	if roleCode == "SUPER_ADMIN" {
		return true
	}
	var role models.Role
	if err := config.DB.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
		return false
	}
	for _, p := range role.Permissions {
		if p.Code == "ViewAllMedia" && p.Enable != nil && *p.Enable {
			return true
		}
	}
	return false
}
