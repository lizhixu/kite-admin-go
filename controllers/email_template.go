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

type updateEmailTemplateRequest struct {
	Name    string `json:"name" binding:"required"`
	Subject string `json:"subject" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type previewEmailTemplateRequest struct {
	Vars map[string]string `json:"vars"`
}

// GetList 获取邮件模板列表
// @Summary      获取邮件模板列表
// @Description  获取所有邮件模板
// @Tags         邮件模板
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.EmailTemplate} "成功"
// @Router       /email-template/list [get]
func (ec *EmailTemplateController) GetList(c *gin.Context) {
	var templates []models.EmailTemplate
	if err := config.DB.Order("scene ASC").Find(&templates).Error; err != nil {
		respondInternal(c, "Failed to query templates")
		return
	}
	respondOK(c, templates)
}

// GetOne 获取邮件模板详情
// @Summary      获取邮件模板详情
// @Description  根据ID获取邮件模板详情
// @Tags         邮件模板
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "模板ID"
// @Success      200 {object} models.Response{data=models.EmailTemplate} "成功"
// @Failure      404 {object} models.Response "模板不存在"
// @Router       /email-template/{id} [get]
func (ec *EmailTemplateController) GetOne(c *gin.Context) {
	id := c.Param("id")
	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondNotFound(c, "Template not found")
		return
	}
	respondOK(c, tmpl)
}

// Update 更新邮件模板
// @Summary      更新邮件模板
// @Description  更新指定邮件模板的内容
// @Tags         邮件模板
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                       true "模板ID"
// @Param        body body     updateEmailTemplateRequest true "模板信息"
// @Success      200  {object} models.Response{data=models.EmailTemplate} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "模板不存在"
// @Router       /email-template/{id} [put]
func (ec *EmailTemplateController) Update(c *gin.Context) {
	id := c.Param("id")
	var req updateEmailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondNotFound(c, "Template not found")
		return
	}

	tmpl.Name = req.Name
	tmpl.Subject = req.Subject
	tmpl.Content = req.Content

	if err := config.DB.Save(&tmpl).Error; err != nil {
		respondInternal(c, "Failed to update template")
		return
	}

	respondOK(c, tmpl)
}

// Preview 预览邮件模板
// @Summary      预览邮件模板
// @Description  使用变量渲染邮件模板并返回预览
// @Tags         邮件模板
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                        true  "模板ID"
// @Param        body body     previewEmailTemplateRequest false "模板变量"
// @Success      200  {object} models.Response "成功，data包含subject和htmlBody"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "模板不存在"
// @Router       /email-template/{id}/preview [post]
func (ec *EmailTemplateController) Preview(c *gin.Context) {
	id := c.Param("id")
	var req previewEmailTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var tmpl models.EmailTemplate
	if err := config.DB.First(&tmpl, id).Error; err != nil {
		respondNotFound(c, "Template not found")
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
