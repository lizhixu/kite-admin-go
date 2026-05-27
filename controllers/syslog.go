package controllers

import (
	"backend/config"
	"backend/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SysLogController struct{}

// GetLogs 查询系统日志
// @Summary      查询系统日志
// @Description  分页查询系统操作日志，支持按用户名、HTTP方法、状态码筛选
// @Tags         系统日志
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo     query    int    false "页码"       default(1)
// @Param        pageSize   query    int    false "每页数量"   default(10)
// @Param        username   query    string false "用户名（模糊搜索）"
// @Param        method     query    string false "HTTP方法"
// @Param        statusCode query    string false "状态码"
// @Success      200        {object} models.Response{data=models.PageData{pageData=[]models.SysLog}} "成功"
// @Router       /syslog/list [get]
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
