package controllers

import (
	"backend/config"
	"backend/models"
	"backend/scheduler"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type TaskController struct{}

type taskRequest struct {
	Name        string `json:"name" binding:"required"`
	Spec        string `json:"spec" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=HTTP SHELL FUNC"`
	Command     string `json:"command" binding:"required"`
	HttpMethod  string `json:"httpMethod"`
	HttpHeaders string `json:"httpHeaders"`
	HttpBody    string `json:"httpBody"`
	Timeout     int    `json:"timeout"`
	Enabled     *bool  `json:"enabled"`
	Description string `json:"description"`
}

func (tc *TaskController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	name := c.Query("name")
	typ := c.Query("type")

	q := config.DB.Model(&models.Task{})
	if name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	if typ != "" {
		q = q.Where("type = ?", typ)
	}

	var total int64
	q.Count(&total)

	var rows []models.Task
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

func (tc *TaskController) Create(c *gin.Context) {
	var req taskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	if err := scheduler.ValidateSpec(req.Spec); err != nil {
		respondErr(c, 400, "invalid cron spec: "+err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	task := models.Task{
		Name: req.Name, Spec: req.Spec, Type: req.Type, Command: req.Command,
		HttpMethod: req.HttpMethod, HttpHeaders: req.HttpHeaders, HttpBody: req.HttpBody,
		Timeout: req.Timeout, Enabled: enabled, Description: req.Description,
	}
	if err := config.DB.Create(&task).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondErr(c, 500, "schedule failed: "+err.Error())
			return
		}
	}
	respondOK(c, task)
}

func (tc *TaskController) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req taskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	if err := scheduler.ValidateSpec(req.Spec); err != nil {
		respondErr(c, 400, "invalid cron spec: "+err.Error())
		return
	}

	var task models.Task
	if err := config.DB.First(&task, id).Error; err != nil {
		respondErr(c, 404, "task not found")
		return
	}

	task.Name = req.Name
	task.Spec = req.Spec
	task.Type = req.Type
	task.Command = req.Command
	task.HttpMethod = req.HttpMethod
	task.HttpHeaders = req.HttpHeaders
	task.HttpBody = req.HttpBody
	task.Timeout = req.Timeout
	task.Description = req.Description
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}

	if err := config.DB.Save(&task).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}

	scheduler.Default().Remove(task.ID)
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondErr(c, 500, "schedule failed: "+err.Error())
			return
		}
	}
	respondOK(c, task)
}

func (tc *TaskController) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := config.DB.Delete(&models.Task{}, id).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	scheduler.Default().Remove(uint(id))
	respondOK(c, true)
}

// Toggle 切换启用/停用状态
func (tc *TaskController) Toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var task models.Task
	if err := config.DB.First(&task, id).Error; err != nil {
		respondErr(c, 404, "task not found")
		return
	}
	task.Enabled = !task.Enabled
	if err := config.DB.Save(&task).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondErr(c, 500, "schedule failed: "+err.Error())
			return
		}
	} else {
		scheduler.Default().Remove(task.ID)
	}
	respondOK(c, task)
}

// Run 立即手动触发一次
func (tc *TaskController) Run(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := scheduler.Default().RunOnce(uint(id)); err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// GetLogs 按任务ID分页查询执行日志
func (tc *TaskController) GetLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	taskID := c.Query("taskId")
	status := c.Query("status")
	trigger := c.Query("trigger")

	q := config.DB.Model(&models.TaskLog{})
	if taskID != "" {
		q = q.Where("task_id = ?", taskID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if trigger != "" {
		q = q.Where("trigger = ?", trigger)
	}

	var total int64
	q.Count(&total)

	var rows []models.TaskLog
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// GetFuncs 列出内置函数清单（用于前端选择）
func (tc *TaskController) GetFuncs(c *gin.Context) {
	respondOK(c, scheduler.FuncList())
}

// Stats 返回任务总数、运行中、成功/失败次数等仪表盘数据
func (tc *TaskController) Stats(c *gin.Context) {
	var (
		total, enabled, disabled                            int64
		successToday, failedToday, timeoutToday, totalToday int64
	)
	config.DB.Model(&models.Task{}).Count(&total)
	config.DB.Model(&models.Task{}).Where("enabled = ?", true).Count(&enabled)
	disabled = total - enabled

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayQ := config.DB.Model(&models.TaskLog{}).Where("start_time >= ?", start)
	dayQ.Count(&totalToday)
	config.DB.Model(&models.TaskLog{}).Where("start_time >= ? AND status = ?", start, "SUCCESS").Count(&successToday)
	config.DB.Model(&models.TaskLog{}).Where("start_time >= ? AND status = ?", start, "FAILED").Count(&failedToday)
	config.DB.Model(&models.TaskLog{}).Where("start_time >= ? AND status = ?", start, "TIMEOUT").Count(&timeoutToday)

	// 最近 10 次执行
	var recent []models.TaskLog
	config.DB.Order("id DESC").Limit(10).Find(&recent)

	respondOK(c, gin.H{
		"total":        total,
		"enabled":      enabled,
		"disabled":     disabled,
		"totalToday":   totalToday,
		"successToday": successToday,
		"failedToday":  failedToday,
		"timeoutToday": timeoutToday,
		"recent":       recent,
	})
}

// PreviewNext 返回 cron 表达式接下来 n 次的执行时间，供前端可视化
func (tc *TaskController) PreviewNext(c *gin.Context) {
	spec := c.Query("spec")
	n, _ := strconv.Atoi(c.DefaultQuery("n", "5"))
	if spec == "" {
		respondErr(c, 400, "spec required")
		return
	}
	times, err := scheduler.PreviewNext(spec, n)
	if err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	respondOK(c, times)
}

type bulkIDsRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// BulkDelete 批量删除
func (tc *TaskController) BulkDelete(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}
	if err := config.DB.Where("id IN ?", req.IDs).Delete(&models.Task{}).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	for _, id := range req.IDs {
		scheduler.Default().Remove(id)
	}
	respondOK(c, len(req.IDs))
}

// BulkToggle 批量启用/停用，body: {ids, enabled}
func (tc *TaskController) BulkToggle(c *gin.Context) {
	var req struct {
		IDs     []uint `json:"ids" binding:"required"`
		Enabled bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondErr(c, 400, "ids required")
		return
	}
	if err := config.DB.Model(&models.Task{}).Where("id IN ?", req.IDs).Update("enabled", req.Enabled).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}

	var tasks []models.Task
	config.DB.Where("id IN ?", req.IDs).Find(&tasks)
	for i := range tasks {
		if req.Enabled {
			_ = scheduler.Default().Add(&tasks[i])
		} else {
			scheduler.Default().Remove(tasks[i].ID)
		}
	}
	respondOK(c, len(req.IDs))
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, models.Response{Code: 0, Message: "OK", Data: data, OriginUrl: c.Request.URL.Path})
}

func respondErr(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, models.Response{Code: code, Message: msg, OriginUrl: c.Request.URL.Path})
}
