package controllers

import (
	"backend/config"
	"backend/models"
	"backend/queue"
	"backend/sse"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type MessageController struct{}

type createMessageRequest struct {
	Title      string `json:"title" binding:"required"`
	Content    string `json:"content" binding:"required"`
	Type       string `json:"type" binding:"required,oneof=SYSTEM NOTICE ANNOUNCEMENT"`
	TargetType string `json:"targetType" binding:"required,oneof=ALL USER"`
	UserIDs    []uint `json:"userIds"`
}

func (mc *MessageController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	title := c.Query("title")
	msgType := c.Query("type")

	q := config.DB.Model(&models.Message{})
	if title != "" {
		q = q.Where("title LIKE ?", "%"+title+"%")
	}
	if msgType != "" {
		q = q.Where("type = ?", msgType)
	}

	var total int64
	q.Count(&total)

	var rows []models.Message
	q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows)

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

func (mc *MessageController) Create(c *gin.Context) {
	var req createMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}

	userID := c.GetUint("userID")
	username := c.GetString("username")

	msg := models.Message{
		Title:      req.Title,
		Content:    req.Content,
		Type:       req.Type,
		SenderID:   userID,
		SenderName: username,
		TargetType: req.TargetType,
		Status:     "SENT",
	}

	if err := config.DB.Create(&msg).Error; err != nil {
		respondErr(c, 500, "Failed to create message")
		return
	}

	// Determine target users
	var userIDs []uint
	if req.TargetType == "ALL" {
		var users []models.User
		config.DB.Where("enable = ?", true).Find(&users)
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
	} else {
		userIDs = req.UserIDs
	}

	// Create recipients
	if len(userIDs) > 0 {
		recipients := make([]models.MessageRecipient, 0, len(userIDs))
		for _, uid := range userIDs {
			recipients = append(recipients, models.MessageRecipient{
				MessageID: msg.ID,
				UserID:    uid,
			})
		}
		config.DB.CreateInBatches(recipients, 100)

		// Notify via SSE
		data, _ := json.Marshal(gson{"messageId": msg.ID, "title": msg.Title, "type": msg.Type})
		sse.Default().NotifyUsers(userIDs, string(data))

		// Push email job async
		payload, _ := json.Marshal(emailPayload{MessageID: msg.ID, UserIDs: userIDs})
		if _, err := queue.Push(context.Background(), "message.email", string(payload)); err != nil {
			log.Printf("queue push message.email: %v", err)
		}
	}

	respondOK(c, msg)
}

func (mc *MessageController) Delete(c *gin.Context) {
	id := c.Param("id")
	config.DB.Where("message_id = ?", id).Delete(&models.MessageRecipient{})
	if err := config.DB.Delete(&models.Message{}, id).Error; err != nil {
		respondErr(c, 500, "Failed to delete message")
		return
	}
	respondOK(c, true)
}

func (mc *MessageController) BulkDelete(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}
	config.DB.Where("message_id IN ?", req.IDs).Delete(&models.MessageRecipient{})
	config.DB.Where("id IN ?", req.IDs).Delete(&models.Message{})
	respondOK(c, len(req.IDs))
}

func (mc *MessageController) GetMyMessages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	userID := c.GetUint("userID")

	type messageWithStatus struct {
		models.Message
		IsRead bool       `json:"isRead"`
		ReadAt *time.Time `json:"readAt"`
	}

	var total int64
	config.DB.Model(&models.MessageRecipient{}).Where("user_id = ?", userID).Count(&total)

	var rows []messageWithStatus
	config.DB.Table("message_recipients mr").
		Select("messages.*, mr.is_read, mr.read_at").
		Joins("LEFT JOIN messages ON messages.id = mr.message_id").
		Where("mr.user_id = ?", userID).
		Order("messages.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows)

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

func (mc *MessageController) GetUnreadCount(c *gin.Context) {
	userID := c.GetUint("userID")

	var count int64
	config.DB.Model(&models.MessageRecipient{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count)

	respondOK(c, gin.H{"count": count})
}

func (mc *MessageController) MarkRead(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userID := c.GetUint("userID")

	now := time.Now()
	result := config.DB.Model(&models.MessageRecipient{}).
		Where("message_id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})

	if result.RowsAffected == 0 {
		respondErr(c, 404, "Message not found")
		return
	}
	respondOK(c, true)
}

func (mc *MessageController) MarkAllRead(c *gin.Context) {
	userID := c.GetUint("userID")
	now := time.Now()

	config.DB.Model(&models.MessageRecipient{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})

	respondOK(c, true)
}

func (mc *MessageController) BulkMarkRead(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}

	userID := c.GetUint("userID")
	now := time.Now()

	config.DB.Model(&models.MessageRecipient{}).
		Where("message_id IN ? AND user_id = ? AND is_read = ?", req.IDs, userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})

	respondOK(c, true)
}

func (mc *MessageController) SSE(c *gin.Context) {
	userID := c.GetUint("userID")

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	client := sse.Default().Register(userID)
	defer sse.Default().Unregister(client)

	c.Writer.Flush()

	// Send initial unread count
	var count int64
	config.DB.Model(&models.MessageRecipient{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count)
	initData := fmt.Sprintf("event: init\ndata: {\"unreadCount\":%d}\n\n", count)
	c.Writer.WriteString(initData)
	c.Writer.Flush()

	// Stream events
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case event, ok := <-client.Chan:
			if !ok {
				return
			}
			fmt.Fprint(c.Writer, event)
			c.Writer.Flush()
		case <-ticker.C:
			fmt.Fprint(c.Writer, sse.FormatSSE())
			c.Writer.Flush()
		}
	}
}

// Helper types
type gson = map[string]interface{}

type emailPayload struct {
	MessageID uint   `json:"messageId"`
	UserIDs   []uint `json:"userIds"`
}
