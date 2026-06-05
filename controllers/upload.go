package controllers

import (
	"backend/config"
	"backend/models"
	"backend/storage"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UploadController 业务文件上传：登录用户均可使用，按 bizType 白名单控制
// 与 /media/upload 区分：业务上传不进文件夹树，路径固定为 {bizType}/YYYY/MM/DD/
type UploadController struct{}

// bizUploadSpec 业务上传白名单
type bizUploadSpec struct {
	AllowExtensions []string `json:"allowExtensions"`
	AllowMimePrefix []string `json:"allowMimePrefix"`
	MaxSizeMB       int      `json:"maxSizeMB"`
}

var bizUploadSpecs = map[string]bizUploadSpec{
	"avatar": {
		AllowExtensions: []string{"jpg", "jpeg", "png", "webp", "gif"},
		AllowMimePrefix: []string{"image/"},
		MaxSizeMB:       2,
	},
	"rich-text": {
		AllowExtensions: []string{"jpg", "jpeg", "png", "webp", "gif"},
		AllowMimePrefix: []string{"image/"},
		MaxSizeMB:       5,
	},
	"message-attach": {
		AllowExtensions: nil, // 不限扩展名
		AllowMimePrefix: nil,
		MaxSizeMB:       20,
	},
}

// Specs 获取业务上传白名单
// @Summary      获取业务上传白名单
// @Description  返回每个 bizType 的扩展名/mime/大小限制，供前端做客户端校验
// @Tags         上传
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Router       /upload/specs [get]
func (uc *UploadController) Specs(c *gin.Context) {
	respondOK(c, bizUploadSpecs)
}

// Biz 业务上传
// @Summary      业务上传
// @Description  登录用户即可使用，按 bizType 白名单（avatar/rich-text/message-attach）限制；存储固定走系统默认；路径 {bizType}/YYYY/MM/DD
// @Tags         上传
// @Security     BearerAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        bizType formData   string true  "业务类型 avatar/rich-text/message-attach"
// @Param        file    formData   file   true  "上传文件"
// @Success      200     {object} models.Response{data=models.Media} "成功"
// @Failure      400     {object} models.Response "请求参数错误"
// @Router       /upload [post]
func (uc *UploadController) Biz(c *gin.Context) {
	bizType := strings.TrimSpace(c.PostForm("bizType"))
	spec, ok := bizUploadSpecs[bizType]
	if !ok {
		respondBadRequest(c, "invalid bizType")
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		respondBadRequest(c, "file required: "+err.Error())
		return
	}

	cfg, err := pickStorageConfig("")
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if !cfg.Enabled {
		respondBadRequest(c, "default storage disabled")
		return
	}

	originalName := fileHeader.Filename
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(originalName)), ".")
	if len(spec.AllowExtensions) > 0 && !containsCI(spec.AllowExtensions, ext) {
		respondBadRequest(c, "extension not allowed: "+ext)
		return
	}
	if spec.MaxSizeMB > 0 && fileHeader.Size > int64(spec.MaxSizeMB)*1024*1024 {
		respondBadRequest(c, fmt.Sprintf("file too large (>%dMB)", spec.MaxSizeMB))
		return
	}

	mime := detectMime(fileHeader)
	if len(spec.AllowMimePrefix) > 0 {
		matched := false
		for _, p := range spec.AllowMimePrefix {
			if strings.HasPrefix(mime, p) {
				matched = true
				break
			}
		}
		if !matched {
			respondBadRequest(c, "mime type not allowed: "+mime)
			return
		}
	}

	store, err := storage.New(cfg)
	if err != nil {
		respondInternal(c, "init storage: "+err.Error())
		return
	}
	src, err := fileHeader.Open()
	if err != nil {
		respondInternal(c, "open upload: "+err.Error())
		return
	}
	defer src.Close()

	now := time.Now()
	folderPath := fmt.Sprintf("%s/%04d/%02d/%02d", bizType, now.Year(), int(now.Month()), now.Day())
	id := strings.ReplaceAll(uuid.New().String(), "-", "")
	filename := id
	if ext != "" {
		filename += "." + ext
	}
	key := folderPath + "/" + filename

	if err := store.Put(c, key, src, fileHeader.Size, mime); err != nil {
		respondInternal(c, "store: "+err.Error())
		return
	}

	uid := currentUserID(c)
	uname, _ := c.Get("username")
	unameStr, _ := uname.(string)

	rec := models.Media{
		Filename:     originalName,
		StorageKey:   key,
		StorageType:  cfg.Type,
		ConfigID:     cfg.ID,
		FolderID:     0,
		FolderPath:   folderPath,
		BizType:      bizType,
		LegacyURL:    key,
		MimeType:     mime,
		Ext:          ext,
		Size:         fileHeader.Size,
		UploaderID:   uid,
		UploaderName: unameStr,
	}
	if err := config.DB.Create(&rec).Error; err != nil {
		_ = store.Delete(c, key)
		respondInternal(c, err.Error())
		return
	}
	setMediaAccessURL(&rec, store.PublicURL(rec.StorageKey))
	respondOK(c, rec)
}
