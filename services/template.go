package services

import (
	"backend/config"
	"backend/models"
	"strings"
)

func GetTemplate(scene string) *models.EmailTemplate {
	var tmpl models.EmailTemplate
	if err := config.DB.Where("scene = ?", scene).First(&tmpl).Error; err != nil {
		return nil
	}
	return &tmpl
}

func GetTemplateWithFallback(scene string) *models.EmailTemplate {
	tmpl := GetTemplate(scene)
	if tmpl == nil {
		tmpl = GetTemplate("SYSTEM")
	}
	return tmpl
}

func RenderTemplate(tmpl *models.EmailTemplate, vars map[string]string) (subject, htmlBody string) {
	if tmpl == nil {
		return "", ""
	}
	subject = replaceVars(tmpl.Subject, vars)
	htmlBody = replaceVars(tmpl.Content, vars)
	return
}

func replaceVars(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{{"+k+"}}", v)
	}
	return s
}

func MarkdownToHTML(markdown string) string {
	return markdownToHTML(markdown)
}
