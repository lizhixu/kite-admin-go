package controllers

import (
	"backend/config"
	"backend/models"
	"backend/services"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type EmailTemplateController struct{}

func (ec *EmailTemplateController) GetList(c *gin.Context) {
	var templates []models.EmailTemplate
	if err := config.DB.Order("scene ASC").Find(&templates).Error; err != nil {
		respondErr(c, 500, "Failed to query templates")
		return
	}
	respondOK(c, templates)
}

func (ec *EmailTemplateController) GetOne(c *gin.Context) {
	id := c.Param("id")
	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondErr(c, 404, "Template not found")
		return
	}
	respondOK(c, tmpl)
}

func (ec *EmailTemplateController) Update(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name    string `json:"name" binding:"required"`
		Subject string `json:"subject" binding:"required"`
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}

	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondErr(c, 404, "Template not found")
		return
	}

	tmpl.Name = req.Name
	tmpl.Subject = req.Subject
	tmpl.Content = req.Content

	if err := config.DB.Save(&tmpl).Error; err != nil {
		respondErr(c, 500, "Failed to update template")
		return
	}

	respondOK(c, tmpl)
}

func (ec *EmailTemplateController) Preview(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Vars map[string]string `json:"vars"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}

	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondErr(c, 404, "Template not found")
		return
	}

	if req.Vars == nil {
		req.Vars = map[string]string{
			"title":       "示例标题",
			"content":     "这是一段示例内容，用于预览邮件模板效果。",
			"username":    "testuser",
			"siteURL":     "https://example.com",
			"currentTime": time.Now().Format("2006-01-02 15:04:05"),
		}
	}

	if _, ok := req.Vars["content"]; ok {
		req.Vars["content"] = services.MarkdownToHTML(req.Vars["content"])
	}

	subject, htmlBody := services.RenderTemplate(&tmpl, req.Vars)

	respondOK(c, gin.H{
		"subject":  subject,
		"htmlBody": htmlBody,
	})
}

func (ec *EmailTemplateController) GetByIDParam(id string) *models.EmailTemplate {
	idNum, _ := strconv.Atoi(id)
	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, idNum).Error; err != nil {
		return nil
	}
	return &tmpl
}
