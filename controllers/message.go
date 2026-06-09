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

// createMessageRequest 创建消息请求
type createMessageRequest struct {
	Title      string `json:"title" binding:"required"`
	Content    string `json:"content" binding:"required"`
	Type       string `json:"type" binding:"required,oneof=SYSTEM NOTICE ANNOUNCEMENT"`
	TargetType string `json:"targetType" binding:"required,oneof=ALL USER ROLE"`
	RoleIDs    []uint `json:"roleIds"`
	UserIDs    []uint `json:"userIds"`
}

// GetPage 分页查询消息
// @Summary      分页查询消息
// @Description  分页查询消息列表，支持按标题和类型筛选
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Param        title    query    string false "标题（模糊搜索）"
// @Param        type     query    string false "类型（SYSTEM/NOTICE/ANNOUNCEMENT）"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.Message}} "成功"
// @Router       /message/page [get]
func (mc *MessageController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}
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
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		log.Printf("GetPage find error: %v", err)
	}

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// Create 发送消息
// @Summary      发送消息
// @Description  创建并发送消息，支持按用户、角色或全员发送
// @Tags         消息管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     createMessageRequest true "消息信息"
// @Success      200  {object} models.Response{data=models.Message} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /message [post]
func (mc *MessageController) Create(c *gin.Context) {
	var req createMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	userID := c.GetUint("userID")
	username := c.GetString("username")

	// Determine target users
	var userIDs []uint
	switch req.TargetType {
	case "ALL":
		var users []models.User
		config.DB.Where("enable = ?", true).Find(&users)
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
	case "ROLE":
		if len(req.RoleIDs) == 0 {
			respondBadRequest(c, "roleIds required")
			return
		}
		// Find all users that belong to any of the specified roles
		var users []models.User
		config.DB.Joins("JOIN user_roles ON user_roles.user_id = users.id").
			Where("user_roles.role_id IN ? AND users.enable = ?", req.RoleIDs, true).
			Distinct("users.id").
			Find(&users)
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
	default:
		if len(req.UserIDs) == 0 {
			respondBadRequest(c, "userIds required")
			return
		}
		var users []models.User
		config.DB.Where("id IN ? AND enable = ?", req.UserIDs, true).Find(&users)
		for _, u := range users {
			userIDs = append(userIDs, u.ID)
		}
	}
	userIDs = uniqueUintIDs(userIDs)
	if len(userIDs) == 0 {
		respondBadRequest(c, "no recipients matched")
		return
	}

	targetRolesJSON := ""
	if len(req.RoleIDs) > 0 {
		b, _ := json.Marshal(req.RoleIDs)
		targetRolesJSON = string(b)
	}

	msg := models.Message{
		Title:       req.Title,
		Content:     req.Content,
		Type:        req.Type,
		SenderID:    userID,
		SenderName:  username,
		TargetType:  req.TargetType,
		TargetRoles: targetRolesJSON,
		Status:      "SENT",
	}

	if err := config.DB.Create(&msg).Error; err != nil {
		respondInternal(c, "Failed to create message")
		return
	}

	// Create recipients
	recipients := make([]models.MessageRecipient, 0, len(userIDs))
	for _, uid := range userIDs {
		recipients = append(recipients, models.MessageRecipient{
			MessageID: msg.ID,
			UserID:    uid,
		})
	}
	if err := config.DB.CreateInBatches(recipients, 100).Error; err != nil {
		respondInternal(c, "Failed to create recipients")
		return
	}

	// Notify via SSE
	data, _ := json.Marshal(gson{"messageId": msg.ID, "title": msg.Title, "type": msg.Type})
	sse.Default().NotifyUsers(userIDs, string(data))

	// Push one email job per recipient (batched)
	items := make([]queue.BulkPushItem, 0, len(userIDs))
	for _, uid := range userIDs {
		payload, _ := json.Marshal(emailPayload{MessageID: msg.ID, UserID: uid})
		items = append(items, queue.BulkPushItem{
			Payload: string(payload),
			Opts:    queue.PushOpts{UniqueKey: fmt.Sprintf("message.email:%d:%d", msg.ID, uid)},
		})
	}
	if _, err := queue.BulkPush(context.Background(), "message.email", items); err != nil {
		log.Printf("queue bulk push message.email msg=%d: %v", msg.ID, err)
	}

	respondOK(c, msg)
}

// Delete 删除消息
// @Summary      删除消息
// @Description  删除指定消息及其收件人记录
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "消息ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "删除失败"
// @Router       /message/{id} [delete]
func (mc *MessageController) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := config.DB.Where("message_id = ?", id).Delete(&models.MessageRecipient{}).Error; err != nil {
		log.Printf("Delete message recipients error: %v", err)
	}
	if err := config.DB.Delete(&models.Message{}, id).Error; err != nil {
		respondInternal(c, "Failed to delete message")
		return
	}
	respondOK(c, true)
}

// BulkDelete 批量删除消息
// @Summary      批量删除消息
// @Description  批量删除指定消息
// @Tags         消息管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     bulkIDsRequest true "消息ID列表"
// @Success      200  {object} models.Response{data=int} "成功删除数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /message/bulk/delete [post]
func (mc *MessageController) BulkDelete(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondBadRequest(c, "ids required")
		return
	}
	if err := config.DB.Where("message_id IN ?", req.IDs).Delete(&models.MessageRecipient{}).Error; err != nil {
		log.Printf("BulkDelete recipients error: %v", err)
	}
	if err := config.DB.Where("id IN ?", req.IDs).Delete(&models.Message{}).Error; err != nil {
		log.Printf("BulkDelete messages error: %v", err)
	}
	respondOK(c, len(req.IDs))
}

// GetMyMessages 获取我的消息
// @Summary      获取我的消息
// @Description  获取当前用户收到的消息列表
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query int false "页码"     default(1)
// @Param        pageSize query int false "每页数量" default(10)
// @Success      200      {object} models.Response "成功"
// @Router       /message/mine [get]
func (mc *MessageController) GetMyMessages(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}
	userID := c.GetUint("userID")

	type messageWithStatus struct {
		models.Message
		IsRead bool       `json:"isRead"`
		ReadAt *time.Time `json:"readAt"`
	}

	var total int64
	if err := config.DB.Model(&models.MessageRecipient{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		log.Printf("GetMyMessages count error: %v", err)
	}

	var rows []messageWithStatus
	if err := config.DB.Table("message_recipients mr").
		Select("messages.*, mr.is_read, mr.read_at").
		Joins("LEFT JOIN messages ON messages.id = mr.message_id").
		Where("mr.user_id = ?", userID).
		Order("messages.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error; err != nil {
		log.Printf("GetMyMessages scan error: %v", err)
	}

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// GetUnreadCount 获取未读消息数
// @Summary      获取未读消息数
// @Description  获取当前用户的未读消息数量
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功，data包含count字段"
// @Router       /message/unread/count [get]
func (mc *MessageController) GetUnreadCount(c *gin.Context) {
	userID := c.GetUint("userID")

	var count int64
	config.DB.Model(&models.MessageRecipient{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count)

	respondOK(c, gin.H{"count": count})
}

// MarkRead 标记消息已读
// @Summary      标记消息已读
// @Description  标记指定消息为已读
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "消息ID"
// @Success      200 {object} models.Response "成功"
// @Failure      404 {object} models.Response "消息不存在"
// @Router       /message/{id}/read [patch]
func (mc *MessageController) MarkRead(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userID := c.GetUint("userID")

	now := time.Now()
	result := config.DB.Model(&models.MessageRecipient{}).
		Where("message_id = ? AND user_id = ?", id, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now})

	if result.RowsAffected == 0 {
		respondNotFound(c, "Message not found")
		return
	}
	respondOK(c, true)
}

// MarkAllRead 全部标记已读
// @Summary      全部标记已读
// @Description  将当前用户所有未读消息标记为已读
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Router       /message/read/all [patch]
func (mc *MessageController) MarkAllRead(c *gin.Context) {
	userID := c.GetUint("userID")
	now := time.Now()

	if err := config.DB.Model(&models.MessageRecipient{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error; err != nil {
		log.Printf("MarkAllRead error: %v", err)
	}

	respondOK(c, true)
}

// BulkMarkRead 批量标记已读
// @Summary      批量标记已读
// @Description  批量标记指定消息为已读
// @Tags         消息管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     bulkIDsRequest true "消息ID列表"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /message/bulk/read [patch]
func (mc *MessageController) BulkMarkRead(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondBadRequest(c, "ids required")
		return
	}

	userID := c.GetUint("userID")
	now := time.Now()

	if err := config.DB.Model(&models.MessageRecipient{}).
		Where("message_id IN ? AND user_id = ? AND is_read = ?", req.IDs, userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error; err != nil {
		log.Printf("BulkMarkRead error: %v", err)
	}

	respondOK(c, true)
}

// SSE 消息推送SSE连接
// @Summary      消息推送SSE连接
// @Description  建立SSE连接，实时接收新消息推送
// @Tags         消息管理
// @Security     BearerAuth
// @Produce      text/event-stream
// @Success      200 {string} string "SSE事件流"
// @Router       /message/sse [get]
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

func uniqueUintIDs(ids []uint) []uint {
	unique := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

// Helper types
type gson = map[string]interface{}

type emailPayload struct {
	MessageID uint `json:"messageId"`
	UserID    uint `json:"userId"`
}
