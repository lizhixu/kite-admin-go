package controllers

import (
	"backend/config"
	"backend/models"
	"backend/queue"
	"backend/services"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
)

type EmailConfigController struct{}

// emailConfigRequest 邮件配置请求
type emailConfigRequest struct {
	Host      string `json:"host" binding:"required"`
	Port      int    `json:"port" binding:"required"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password"`
	FromName  string `json:"fromName" binding:"required"`
	FromEmail string `json:"fromEmail" binding:"required,email"`
	Enabled   bool   `json:"enabled"`
}

// Get 获取邮件配置
// @Summary      获取邮件配置
// @Description  获取当前邮件服务配置
// @Tags         邮件配置
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=models.EmailConfig} "成功"
// @Router       /email/config [get]
func (ec *EmailConfigController) Get(c *gin.Context) {
	var cfg models.EmailConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondOK(c, nil)
		return
	}
	respondOK(c, cfg)
}

// Save 保存邮件配置
// @Summary      保存邮件配置
// @Description  创建或更新邮件服务配置
// @Tags         邮件配置
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     emailConfigRequest true "配置信息"
// @Success      200  {object} models.Response{data=models.EmailConfig} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /email/config [put]
func (ec *EmailConfigController) Save(c *gin.Context) {
	var req emailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var cfg models.EmailConfig
	err := config.DB.First(&cfg).Error

	cfg.Host = req.Host
	cfg.Port = req.Port
	cfg.Username = req.Username
	if req.Password != "" {
		cfg.Password = req.Password
	}
	cfg.FromName = req.FromName
	cfg.FromEmail = req.FromEmail
	cfg.Enabled = req.Enabled

	if err != nil {
		// Create
		if err := config.DB.Create(&cfg).Error; err != nil {
			respondInternal(c, "Failed to save email config")
			return
		}
	} else {
		// Update
		if err := config.DB.Save(&cfg).Error; err != nil {
			respondInternal(c, "Failed to save email config")
			return
		}
	}

	respondOK(c, cfg)
}

// Test 测试邮件配置
// @Summary      测试邮件配置
// @Description  发送测试邮件到当前用户的邮箱
// @Tags         邮件配置
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "测试邮件已加入队列"
// @Failure      400 {object} models.Response "配置未启用或未绑定邮箱"
// @Router       /email/config/test [post]
func (ec *EmailConfigController) Test(c *gin.Context) {
	var cfg models.EmailConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondBadRequest(c, "Email config not found")
		return
	}

	if !cfg.Enabled {
		respondBadRequest(c, "Email service is disabled")
		return
	}

	userID := c.GetUint("userID")
	var profile models.Profile
	if err := config.DB.Where("user_id = ?", userID).First(&profile).Error; err != nil || profile.Email == nil || *profile.Email == "" {
		respondBadRequest(c, "请先绑定邮箱")
		return
	}

	// Get SYSTEM template for test email
	tmpl := services.GetTemplate("SYSTEM")
	if tmpl == nil {
		respondInternal(c, "Email template not found")
		return
	}

	// Prepare template variables
	vars := map[string]string{
		"title":       "测试邮件",
		"content":     "这是一封测试邮件，用于验证邮件配置是否正确。如果你收到此邮件，说明配置成功！",
		"username":    profile.NickName,
		"currentTime": time.Now().Format("2006-01-02 15:04:05"),
	}

	// Render template
	subject, htmlBody := services.RenderTemplate(tmpl, vars)

	payload, _ := json.Marshal(map[string]string{
		"toEmail":  *profile.Email,
		"subject":  subject,
		"htmlBody": htmlBody,
	})

	if _, err := queue.Push(context.Background(), "email.test", string(payload)); err != nil {
		respondInternal(c, fmt.Sprintf("Failed to queue test email: %v", err))
		return
	}

	respondOK(c, "Test email queued successfully")
}
