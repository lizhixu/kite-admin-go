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

type emailConfigRequest struct {
	Host      string `json:"host" binding:"required"`
	Port      int    `json:"port" binding:"required"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password"`
	FromName  string `json:"fromName" binding:"required"`
	FromEmail string `json:"fromEmail" binding:"required,email"`
	Enabled   bool   `json:"enabled"`
}

func (ec *EmailConfigController) Get(c *gin.Context) {
	var cfg models.EmailConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondOK(c, nil)
		return
	}
	respondOK(c, cfg)
}

func (ec *EmailConfigController) Save(c *gin.Context) {
	var req emailConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
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
			respondErr(c, 500, "Failed to save email config")
			return
		}
	} else {
		// Update
		if err := config.DB.Save(&cfg).Error; err != nil {
			respondErr(c, 500, "Failed to save email config")
			return
		}
	}

	respondOK(c, cfg)
}

func (ec *EmailConfigController) Test(c *gin.Context) {
	var cfg models.EmailConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondErr(c, 400, "Email config not found")
		return
	}

	if !cfg.Enabled {
		respondErr(c, 400, "Email service is disabled")
		return
	}

	userID := c.GetUint("userID")
	var profile models.Profile
	if err := config.DB.Where("user_id = ?", userID).First(&profile).Error; err != nil || profile.Email == nil || *profile.Email == "" {
		respondErr(c, 400, "请先绑定邮箱")
		return
	}

	// Get SYSTEM template for test email
	tmpl := services.GetTemplate("SYSTEM")
	if tmpl == nil {
		respondErr(c, 500, "Email template not found")
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
		respondErr(c, 500, fmt.Sprintf("Failed to queue test email: %v", err))
		return
	}

	respondOK(c, "Test email queued successfully")
}
