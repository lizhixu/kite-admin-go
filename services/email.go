package services

import (
	"backend/models"
	"bytes"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var md goldmark.Markdown

func init() {
	md = goldmark.New(
		goldmark.WithExtensions(
		extension.GFM,
			extension.Strikethrough,
			extension.Table,
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
}

type EmailService struct{}

func NewEmailService() *EmailService {
	return &EmailService{}
}

// Send sends an HTML email using the provided config.
// Supports implicit SSL (port 465) and STARTTLS (port 587).
func (s *EmailService) Send(cfg models.EmailConfig, to string, subject string, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	from := fmt.Sprintf("%s <%s>", cfg.FromName, cfg.FromEmail)
	msg := buildEmail(from, to, subject, htmlBody)
	tlsConfig := &tls.Config{ServerName: cfg.Host}

	if cfg.Port == 465 {
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		return sendViaClient(client, cfg, to, msg)
	}

	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(tlsConfig); err != nil {
			client.Close()
			return fmt.Errorf("starttls: %w", err)
		}
	}
	return sendViaClient(client, cfg, to, msg)
}

func sendViaClient(client *smtp.Client, cfg models.EmailConfig, to, msg string) error {
	// After TLS, check supported auth mechanisms
	var auth smtp.Auth
	if ok, mechs := client.Extension("AUTH"); ok {
		if strings.Contains(mechs, "LOGIN") {
			auth = &loginAuth{cfg.Username, cfg.Password}
		} else if strings.Contains(mechs, "PLAIN") {
			auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		}
	}
	if auth == nil {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(cfg.FromEmail); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return client.Quit()
}

// loginAuth implements smtp.Auth using the LOGIN mechanism.
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte(a.username), nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	return []byte(a.password), nil
}

// SendBatch sends the same email to multiple recipients.
func (s *EmailService) SendBatch(cfg models.EmailConfig, recipients []string, subject string, htmlBody string) error {
	var errs []string
	for _, to := range recipients {
		if err := s.Send(cfg, to, subject, htmlBody); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", to, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to send to: %s", strings.Join(errs, "; "))
	}
	return nil
}

func buildEmail(from, to, subject, htmlBody string) string {
	return fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=\"UTF-8\"\r\n"+
		"\r\n"+
		"%s", from, to, subject, htmlBody)
}

// markdownToHTML converts Markdown to HTML using goldmark.
func markdownToHTML(markdown string) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		// Fallback: return escaped raw text
		return "<p>" + strings.ReplaceAll(strings.ReplaceAll(markdown, "<", "&lt;"), ">", "&gt;") + "</p>"
	}
	return buf.String()
}

// RenderMessageEmail renders a notification email HTML body from Markdown content.
func RenderMessageEmail(title, content, siteURL string) string {
	bodyHTML := markdownToHTML(content)

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body style="margin:0;padding:0;background:#eef1f5;">
<table width="100%%" cellpadding="0" cellspacing="0" style="background:#eef1f5;padding:32px 0;">
<tr><td align="center">
<table width="600" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:12px;overflow:hidden;box-shadow:0 4px 24px rgba(0,0,0,0.08);">

  <!-- Header -->
  <tr>
    <td style="background:linear-gradient(135deg,#18a058,#36d399);padding:28px 32px;">
      <table cellpadding="0" cellspacing="0" width="100%%"><tr>
        <td><h1 style="margin:0;color:#fff;font-size:20px;font-weight:600;">%s</h1></td>
        <td align="right" valign="middle" style="color:#fff;font-size:13px;opacity:0.85;">Kite Admin</td>
      </tr></table>
    </td>
  </tr>

  <!-- Body -->
  <tr>
    <td style="padding:32px;">
      <div style="color:#2c3e50;line-height:1.8;font-size:15px;">
        %s
      </div>
    </td>
  </tr>

  <!-- Action button -->
  %s

  <!-- Divider -->
  <tr>
    <td style="padding:0 32px;">
      <div style="border-top:1px solid #e8ecf1;"></div>
    </td>
  </tr>

  <!-- Footer -->
  <tr>
    <td style="padding:20px 32px;text-align:center;color:#8c939d;font-size:12px;">
      此邮件由 Kite Admin 系统自动发送，请勿直接回复
    </td>
  </tr>

</table>
</td></tr>
</table>
</body>
</html>`, title, bodyHTML, renderSiteLink(siteURL))
}

func renderSiteLink(siteURL string) string {
	if siteURL == "" {
		return ""
	}
	return fmt.Sprintf(`<tr>
    <td style="padding:24px 32px 0;text-align:center;">
      <a href="%s" style="display:inline-block;background:#18a058;color:#fff;padding:12px 32px;border-radius:8px;text-decoration:none;font-size:15px;font-weight:500;">查看详情 &rarr;</a>
    </td>
  </tr>`, siteURL)
}
