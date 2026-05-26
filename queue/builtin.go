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
			ToEmail   string `json:"toEmail"`
			Subject   string `json:"subject"`
			HtmlBody  string `json:"htmlBody"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return "", fmt.Errorf("invalid payload: %w", err)
		}

		var emailCfg models.EmailConfig
		if err := config.DB.First(&emailCfg).Error; err != nil {
			return "", fmt.Errorf("email config not found: %w", err)
		}
		if !emailCfg.Enabled {
			return "", fmt.Errorf("email service is disabled")
		}

		svc := services.NewEmailService()
		if err := svc.Send(emailCfg, p.ToEmail, p.Subject, p.HtmlBody); err != nil {
			return "", fmt.Errorf("send failed: %w", err)
		}

		return fmt.Sprintf("test email sent to %s", p.ToEmail), nil
	})
	log.Println("Queue builtin handler registered: email.test")

	// Message email notification handler
	setHandler("message.email", func(ctx context.Context, payload string) (string, error) {
		var p struct {
			MessageID uint   `json:"messageId"`
			UserIDs   []uint `json:"userIds"`
		}
		if err := json.Unmarshal([]byte(payload), &p); err != nil {
			return "", fmt.Errorf("invalid payload: %w", err)
		}

		// Check email config
		var emailCfg models.EmailConfig
		if err := config.DB.First(&emailCfg).Error; err != nil || !emailCfg.Enabled {
			return "email not configured or disabled, skipped", nil
		}

		// Get message
		var msg models.Message
		if err := config.DB.First(&msg, p.MessageID).Error; err != nil {
			return "", fmt.Errorf("message not found: %w", err)
		}

		// Get template based on message type
		tmpl := services.GetTemplateWithFallback(msg.Type)
		if tmpl == nil {
			return "", fmt.Errorf("email template not found for type: %s", msg.Type)
		}

		// Get recipients that haven't been emailed
		var recipients []models.MessageRecipient
		config.DB.Where("message_id = ? AND emailed = ? AND user_id IN ?", p.MessageID, false, p.UserIDs).
			Find(&recipients)

		if len(recipients) == 0 {
			return "no recipients to email", nil
		}

		// Get user emails and profiles
		userIDSet := make(map[uint]struct{}, len(recipients))
		for _, r := range recipients {
			userIDSet[r.UserID] = struct{}{}
		}
		userIDs := make([]uint, 0, len(userIDSet))
		for id := range userIDSet {
			userIDs = append(userIDs, id)
		}

		var profiles []models.Profile
		config.DB.Where("user_id IN ?", userIDs).Find(&profiles)

		svc := services.NewEmailService()
		sent := 0
		now := time.Now().Format("2006-01-02 15:04:05")

		for _, profile := range profiles {
			if profile.Email == nil || *profile.Email == "" {
				continue
			}

			// Render template with recipient-specific variables
			vars := map[string]string{
				"title":       msg.Title,
				"content":     services.MarkdownToHTML(msg.Content),
				"username":    profile.NickName,
				"currentTime": now,
			}
			subject, htmlBody := services.RenderTemplate(tmpl, vars)

			if err := svc.Send(emailCfg, *profile.Email, subject, htmlBody); err != nil {
				log.Printf("email send failed user=%d: %v", profile.UserID, err)
				continue
			}
			sent++
		}

		// Mark as emailed
		config.DB.Model(&models.MessageRecipient{}).
			Where("message_id = ? AND user_id IN ?", p.MessageID, userIDs).
			Update("emailed", true)

		return fmt.Sprintf("emailed %d/%d recipients", sent, len(profiles)), nil
	})
	log.Println("Queue builtin handler registered: message.email")
}
