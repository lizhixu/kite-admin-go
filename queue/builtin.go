package queue

import (
	"backend/config"
	"backend/models"
	"backend/services"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// init 进程启动时把示例 handler 注册进去（此时 DB 未必就绪，所以仅 setHandler；
// 实际的 Queue 行在 manager.Init() -> rehydrateHandlers() 里 FirstOrCreate）。
func init() {
	setHandler("demo.echo", func(ctx context.Context, payload string) (string, error) {
		return payload, nil
	})
	setHandler("demo.sleep", func(ctx context.Context, payload string) (string, error) {
		select {
		case <-time.After(time.Second):
			return fmt.Sprintf("slept 1s, payload=%s", payload), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	log.Println("Queue builtin handlers registered: demo.echo, demo.sleep")

	// Test email handler
	setHandler("email.test", func(ctx context.Context, payload string) (string, error) {
		var p struct {
			ToEmail  string `json:"toEmail"`
			Subject  string `json:"subject"`
			HtmlBody string `json:"htmlBody"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return "", fmt.Errorf("invalid payload: %w", err)
		}

		var emailCfg models.EmailConfig
		if err := config.DB.WithContext(ctx).First(&emailCfg).Error; err != nil {
			return "", fmt.Errorf("email config not found: %w", err)
		}
		if !emailCfg.Enabled {
			return "", fmt.Errorf("email service is disabled")
		}

		svc := services.NewEmailService()
		if err := svc.Send(ctx, emailCfg, p.ToEmail, p.Subject, p.HtmlBody); err != nil {
			return "", fmt.Errorf("send failed: %w", err)
		}

		return fmt.Sprintf("test email sent to %s", p.ToEmail), nil
	})
	log.Println("Queue builtin handler registered: email.test")

	// Message email notification handler
	setHandler("message.email", func(ctx context.Context, payload string) (string, error) {
		var p struct {
			MessageID uint `json:"messageId"`
			UserID    uint `json:"userId"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return "", fmt.Errorf("invalid payload: %w", err)
		}

		// Check email config
		var emailCfg models.EmailConfig
		if err := config.DB.WithContext(ctx).First(&emailCfg).Error; err != nil || !emailCfg.Enabled {
			return "email not configured or disabled, skipped", nil
		}

		// Get message
		var msg models.Message
		if err := config.DB.WithContext(ctx).First(&msg, p.MessageID).Error; err != nil {
			return "", fmt.Errorf("message not found: %w", err)
		}

		// Get template based on message type
		tmpl := services.GetTemplateWithFallback(msg.Type)
		if tmpl == nil {
			return "", fmt.Errorf("email template not found for type: %s", msg.Type)
		}

		var recipient models.MessageRecipient
		if err := config.DB.WithContext(ctx).Where("message_id = ? AND user_id = ?", p.MessageID, p.UserID).First(&recipient).Error; err != nil {
			return "recipient not found, skipped", nil
		}
		if recipient.Emailed {
			return fmt.Sprintf("recipient %d already emailed", p.UserID), nil
		}

		var profile models.Profile
		if err := config.DB.WithContext(ctx).Where("user_id = ?", p.UserID).First(&profile).Error; err != nil {
			return fmt.Sprintf("profile not found for user %d, skipped", p.UserID), nil
		}
		if profile.Email == nil || *profile.Email == "" {
			return fmt.Sprintf("user %d has no email, skipped", p.UserID), nil
		}

		claim := config.DB.WithContext(ctx).Model(&models.MessageRecipient{}).
			Where("message_id = ? AND user_id = ? AND emailed = ?", p.MessageID, p.UserID, false).
			Update("emailed", true)
		if claim.Error != nil {
			return "", fmt.Errorf("email claim failed user=%d: %w", p.UserID, claim.Error)
		}
		if claim.RowsAffected == 0 {
			return fmt.Sprintf("recipient %d already claimed", p.UserID), nil
		}

		vars := map[string]string{
			"title":       msg.Title,
			"content":     services.MarkdownToHTML(msg.Content),
			"username":    profile.NickName,
			"currentTime": time.Now().Format("2006-01-02 15:04:05"),
		}
		subject, htmlBody := services.RenderTemplate(tmpl, vars)

		svc := services.NewEmailService()
		if err := svc.Send(ctx, emailCfg, *profile.Email, subject, htmlBody); err != nil {
			// Rollback emailed flag so the job can be retried. Do not use ctx here:
			// it may already be canceled after a timeout, but the rollback still needs to run.
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			config.DB.WithContext(rollbackCtx).Model(&models.MessageRecipient{}).
				Where("message_id = ? AND user_id = ? AND emailed = ?", p.MessageID, p.UserID, true).
				Update("emailed", false)
			return "", fmt.Errorf("email send failed user=%d: %w", p.UserID, err)
		}

		return fmt.Sprintf("emailed message %d to user %d", p.MessageID, p.UserID), nil
	})
	log.Println("Queue builtin handler registered: message.email")
}
