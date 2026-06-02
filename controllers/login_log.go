package controllers

import (
	"backend/config"
	"backend/models"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type LoginLogController struct{}

// GetLogs 查询登录日志
// @Summary      查询登录日志
// @Description  分页查询登录成功日志，支持按用户名筛选
// @Tags         登录日志
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query int    false "页码"       default(1)
// @Param        pageSize query int    false "每页数量"   default(10)
// @Param        username query string false "用户名（模糊搜索）"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.LoginLog}} "成功"
// @Router       /loginlog/list [get]
func (ll *LoginLogController) GetLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}
	username := c.Query("username")

	query := config.DB.Model(&models.LoginLog{}).Where("success = ?", true)

	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	respondLoginLogs(c, query, page, pageSize)
}

// GetMyLogs 查询当前用户登录日志
// @Summary      查询当前用户登录日志
// @Description  首页使用：分页查询当前登录用户的登录成功日志，不需要 LoginLog 菜单权限
// @Tags         登录日志
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query int false "页码"     default(1)
// @Param        pageSize query int false "每页数量" default(10)
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.LoginLog}} "成功"
// @Router       /loginlog/mine [get]
func (ll *LoginLogController) GetMyLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}

	query := config.DB.Model(&models.LoginLog{}).
		Where("success = ? AND user_id = ?", true, c.GetUint("userID"))

	respondLoginLogs(c, query, page, pageSize)
}

func respondLoginLogs(c *gin.Context, query *gorm.DB, page, pageSize int) {
	var total int64
	query.Count(&total)

	var logs []models.LoginLog
	if err := query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		respondInternal(c, "Failed to fetch login logs")
		return
	}

	respondOK(c, gin.H{"pageData": logs, "total": total})
}
