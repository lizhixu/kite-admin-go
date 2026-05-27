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

// moveMediaRequest 批量移动媒体请求
type moveMediaRequest struct {
	IDs      []uint `json:"ids" binding:"required"`
	FolderID uint   `json:"folderId"`
}

// createFolderRequest 创建文件夹请求
type createFolderRequest struct {
	Name     string `json:"name" binding:"required"`
	ParentID *uint  `json:"parentId"`
	ConfigID uint   `json:"configId" binding:"required"`
}

// renameFolderRequest 重命名文件夹请求
type renameFolderRequest struct {
	Name string `json:"name" binding:"required"`
}

// Upload 上传文件
// @Summary      上传文件
// @Description  上传文件到指定存储配置和文件夹
// @Tags         媒体管理
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        configId formData   string false "存储配置ID"
// @Param        folderId formData   string false "文件夹ID"
// @Param        file     formData   file   true  "上传文件"
// @Success      200      {object} models.Response{data=models.Media} "成功"
// @Failure      400      {object} models.Response "请求参数错误"
// @Router       /media/upload [post]
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

// GetPage 分页查询媒体
// @Summary      分页查询媒体
// @Description  分页查询媒体文件列表，支持按文件名、MIME类型、存储类型等筛选
// @Tags         媒体管理
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo      query    int    false "页码"     default(1)
// @Param        pageSize    query    int    false "每页数量" default(24)
// @Param        filename    query    string false "文件名（模糊搜索）"
// @Param        mimePrefix  query    string false "MIME类型前缀（如 image/）"
// @Param        storageType query    string false "存储类型（LOCAL/S3）"
// @Param        configId    query    string false "存储配置ID"
// @Param        folderId    query    string false "文件夹ID"
// @Param        scope       query    string false "范围（mine/all）"
// @Success      200         {object} models.Response{data=models.PageData{pageData=[]models.Media}} "成功"
// @Router       /media/page [get]
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

// Delete 删除媒体
// @Summary      删除媒体
// @Description  删除指定的媒体文件
// @Tags         媒体管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "媒体ID"
// @Success      200 {object} models.Response "成功"
// @Failure      404 {object} models.Response "媒体不存在"
// @Router       /media/{id} [delete]
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

// BulkDelete 批量删除媒体
// @Summary      批量删除媒体
// @Description  批量删除指定的媒体文件
// @Tags         媒体管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     bulkIDsRequest true "媒体ID列表"
// @Success      200  {object} models.Response{data=int} "成功删除数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /media/bulk/delete [post]
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

// MoveMedia 移动媒体文件
// @Summary      移动媒体文件
// @Description  批量移动媒体文件到目标文件夹
// @Tags         媒体管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     moveMediaRequest true "媒体ID列表和目标文件夹"
// @Success      200  {object} models.Response{data=int} "移动数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /media/move [post]
func (mc *MediaController) MoveMedia(c *gin.Context) {
	var req moveMediaRequest
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

// ListFolders 获取文件夹列表
// @Summary      获取文件夹列表
// @Description  获取指定存储配置下的文件夹列表（扁平结构）
// @Tags         媒体文件夹
// @Security     BearerAuth
// @Produce      json
// @Param        configId query    string false "存储配置ID"
// @Success      200      {object} models.Response{data=[]models.MediaFolder} "成功"
// @Router       /media/folder/tree [get]
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

// CreateFolder 创建文件夹
// @Summary      创建文件夹
// @Description  在指定存储配置下创建新文件夹
// @Tags         媒体文件夹
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     createFolderRequest true "文件夹信息"
// @Success      200  {object} models.Response{data=models.MediaFolder} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /media/folder [post]
func (mc *MediaController) CreateFolder(c *gin.Context) {
	var req createFolderRequest
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

// RenameFolder 重命名文件夹
// @Summary      重命名文件夹
// @Description  重命名文件夹，联动更新所有后代路径
// @Tags         媒体文件夹
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                 true "文件夹ID"
// @Param        body body     renameFolderRequest true "新名称"
// @Success      200  {object} models.Response{data=models.MediaFolder} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "文件夹不存在"
// @Router       /media/folder/{id} [patch]
func (mc *MediaController) RenameFolder(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req renameFolderRequest
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

// DeleteFolder 删除文件夹
// @Summary      删除文件夹
// @Description  删除文件夹，默认仅允许空文件夹，传cascade=1可级联删除
// @Tags         媒体文件夹
// @Security     BearerAuth
// @Produce      json
// @Param        id      path     int    true  "文件夹ID"
// @Param        cascade query    string false "是否级联删除（1=是）"
// @Success      200     {object} models.Response "成功"
// @Failure      400     {object} models.Response "文件夹不为空"
// @Failure      404     {object} models.Response "文件夹不存在"
// @Router       /media/folder/{id} [delete]
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

// ResolveFolder 按路径解析文件夹
// @Summary      按路径解析文件夹
// @Description  按路径解析或自动创建文件夹，返回最终文件夹对象
// @Tags         媒体文件夹
// @Security     BearerAuth
// @Produce      json
// @Param        configId   query    string true  "存储配置ID"
// @Param        path       query    string true  "文件夹路径（如 avatars/2024）"
// @Param        autoCreate query    string false "不存在时是否自动创建（1=是）"
// @Success      200        {object} models.Response{data=models.MediaFolder} "成功"
// @Failure      400        {object} models.Response "请求参数错误"
// @Failure      404        {object} models.Response "文件夹不存在"
// @Router       /media/folder/resolve [get]
func (mc *MediaController) ResolveFolder(c *gin.Context) {
	configIDStr := c.Query("configId")
	folderPath := strings.TrimSpace(c.Query("path"))
	if folderPath == "" {
		respondErr(c, 400, "path is required")
		return
	}
	configID, err := strconv.Atoi(configIDStr)
	if err != nil || configID <= 0 {
		respondErr(c, 400, "valid configId is required")
		return
	}
	// 校验存储存在
	var cfg models.StorageConfig
	if err := config.DB.First(&cfg, configID).Error; err != nil {
		respondErr(c, 404, "storage config not found")
		return
	}
	// 规范化路径：去首尾 /，拆分段
	folderPath = strings.Trim(folderPath, "/")
	if folderPath == "" {
		respondErr(c, 400, "path cannot be empty")
		return
	}
	segments := strings.Split(folderPath, "/")
	autoCreate := c.Query("autoCreate") == "1"

	var currentParentID *uint
	var currentFolder models.MediaFolder
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		// 查找同级同名文件夹
		q := config.DB.Model(&models.MediaFolder{}).Where("config_id = ? AND name = ?", configID, seg)
		if currentParentID != nil {
			q = q.Where("parent_id = ?", *currentParentID)
		} else {
			q = q.Where("parent_id IS NULL")
		}
		err := q.First(&currentFolder).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !autoCreate {
				respondErr(c, 404, fmt.Sprintf("folder not found: %s", seg))
				return
			}
			// 自动创建
			parentPath := ""
			if currentParentID != nil {
				parentPath = currentFolder.Path + "/"
			}
			newFolder := models.MediaFolder{
				Name:     seg,
				ParentID: currentParentID,
				ConfigID: uint(configID),
				Path:     parentPath + seg,
			}
			if err := config.DB.Create(&newFolder).Error; err != nil {
				respondErr(c, 500, err.Error())
				return
			}
			currentFolder = newFolder
		} else if err != nil {
			respondErr(c, 500, err.Error())
			return
		}
		pid := currentFolder.ID
		currentParentID = &pid
	}
	respondOK(c, currentFolder)
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

// ListConfigs 获取存储配置列表
// @Summary      获取存储配置列表
// @Description  获取所有存储配置，按默认优先排序
// @Tags         存储配置
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.StorageConfig} "成功"
// @Router       /storage/config [get]
func (mc *MediaController) ListConfigs(c *gin.Context) {
	var rows []models.StorageConfig
	if err := config.DB.Order("is_default DESC, id ASC").Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, rows)
}

// CreateConfig 创建存储配置
// @Summary      创建存储配置
// @Description  创建新的存储配置（LOCAL或S3）
// @Tags         存储配置
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     storageConfigRequest true "配置信息"
// @Success      200  {object} models.Response{data=models.StorageConfig} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /storage/config [post]
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

// UpdateConfig 更新存储配置
// @Summary      更新存储配置
// @Description  更新指定的存储配置
// @Tags         存储配置
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                    true "配置ID"
// @Param        body body     storageConfigRequest true "配置信息"
// @Success      200  {object} models.Response{data=models.StorageConfig} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "配置不存在"
// @Router       /storage/config/{id} [patch]
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

// DeleteConfig 删除存储配置
// @Summary      删除存储配置
// @Description  删除指定的存储配置（默认配置不可删除，被使用的配置不可删除）
// @Tags         存储配置
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "配置ID"
// @Success      200 {object} models.Response "成功"
// @Failure      400 {object} models.Response "不可删除默认配置或被使用的配置"
// @Failure      404 {object} models.Response "配置不存在"
// @Router       /storage/config/{id} [delete]
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

// SetDefault 设置默认存储
// @Summary      设置默认存储
// @Description  将指定存储配置设为默认
// @Tags         存储配置
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "配置ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "设置失败"
// @Router       /storage/config/{id}/default [patch]
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

// TestConfig 测试存储配置
// @Summary      测试存储配置
// @Description  测试指定存储配置的连通性
// @Tags         存储配置
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "配置ID"
// @Success      200 {object} models.Response "成功，data包含ok和elapsedMs"
// @Failure      404 {object} models.Response "配置不存在"
// @Failure      500 {object} models.Response "测试失败"
// @Router       /storage/config/{id}/test [post]
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
