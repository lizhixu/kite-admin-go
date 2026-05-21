package controllers

import (
	"backend/config"
	"backend/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SysLogController struct{}

func (sl *SysLogController) GetLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	username := c.Query("username")
	method := c.Query("method")
	statusCode := c.Query("statusCode")

	query := config.DB.Model(&models.SysLog{})

	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}
	if method != "" {
		query = query.Where("method = ?", method)
	}
	if statusCode != "" {
		query = query.Where("status_code = ?", statusCode)
	}

	var total int64
	query.Count(&total)

	var logs []models.SysLog
	if err := query.Order("create_time DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to fetch logs",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:    0,
		Message: "OK",
		Data: gin.H{
			"pageData": logs,
			"total":    total,
		},
		OriginUrl: c.Request.URL.Path,
	})
}
