package controllers

import (
	"backend/config"
	"backend/models"
	"backend/scheduler"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type TaskController struct{}

// taskRequest 创建/更新定时任务请求
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

// GetPage 分页查询任务
// @Summary      分页查询任务
// @Description  分页查询定时任务列表，支持按名称和类型筛选
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Param        name     query    string false "任务名称（模糊搜索）"
// @Param        type     query    string false "任务类型（HTTP/SHELL/FUNC）"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.Task}} "成功"
// @Router       /task/page [get]
func (tc *TaskController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}
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
		respondInternal(c, err.Error())
		return
	}

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// Create 创建定时任务
// @Summary      创建定时任务
// @Description  创建新的定时任务
// @Tags         定时任务
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     taskRequest true "任务信息"
// @Success      200  {object} models.Response{data=models.Task} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /task [post]
func (tc *TaskController) Create(c *gin.Context) {
	var req taskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if err := scheduler.ValidateSpec(req.Spec); err != nil {
		respondBadRequest(c, "invalid cron spec: "+err.Error())
		return
	}
	if !shellTasksAllowed(req.Type) {
		respondBadRequest(c, "SHELL tasks are disabled")
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
		respondInternal(c, err.Error())
		return
	}
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondInternal(c, "schedule failed: "+err.Error())
			return
		}
	}
	respondOK(c, task)
}

// Update 更新定时任务
// @Summary      更新定时任务
// @Description  更新指定定时任务的信息
// @Tags         定时任务
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int           true "任务ID"
// @Param        body body     taskRequest true "任务信息"
// @Success      200  {object} models.Response{data=models.Task} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "任务不存在"
// @Router       /task/{id} [patch]
func (tc *TaskController) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req taskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	if err := scheduler.ValidateSpec(req.Spec); err != nil {
		respondBadRequest(c, "invalid cron spec: "+err.Error())
		return
	}
	if !shellTasksAllowed(req.Type) {
		respondBadRequest(c, "SHELL tasks are disabled")
		return
	}

	var task models.Task
	if err := config.DB.First(&task, id).Error; err != nil {
		respondNotFound(c, "task not found")
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
		respondInternal(c, err.Error())
		return
	}

	scheduler.Default().Remove(task.ID)
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondInternal(c, "schedule failed: "+err.Error())
			return
		}
	}
	respondOK(c, task)
}

// Delete 删除定时任务
// @Summary      删除定时任务
// @Description  删除指定定时任务
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "任务ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "删除失败"
// @Router       /task/{id} [delete]
func (tc *TaskController) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := config.DB.Delete(&models.Task{}, id).Error; err != nil {
		respondInternal(c, err.Error())
		return
	}
	scheduler.Default().Remove(uint(id))
	respondOK(c, true)
}

// Toggle 切换任务启用状态
// @Summary      切换任务启用状态
// @Description  切换指定任务的启用/停用状态
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "任务ID"
// @Success      200 {object} models.Response{data=models.Task} "成功"
// @Failure      404 {object} models.Response "任务不存在"
// @Router       /task/{id}/toggle [patch]
func (tc *TaskController) Toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var task models.Task
	if err := config.DB.First(&task, id).Error; err != nil {
		respondNotFound(c, "task not found")
		return
	}
	task.Enabled = !task.Enabled
	if task.Enabled && !shellTasksAllowed(task.Type) {
		respondBadRequest(c, "SHELL tasks are disabled")
		return
	}
	if err := config.DB.Save(&task).Error; err != nil {
		respondInternal(c, err.Error())
		return
	}
	if task.Enabled {
		if err := scheduler.Default().Add(&task); err != nil {
			respondInternal(c, "schedule failed: "+err.Error())
			return
		}
	} else {
		scheduler.Default().Remove(task.ID)
	}
	respondOK(c, task)
}

// Run 手动执行任务
// @Summary      手动执行任务
// @Description  立即手动触发一次任务执行
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "任务ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "执行失败"
// @Router       /task/{id}/run [post]
func (tc *TaskController) Run(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := scheduler.Default().RunOnce(uint(id)); err != nil {
		respondInternal(c, err.Error())
		return
	}
	respondOK(c, true)
}

// GetLogs 查询任务日志
// @Summary      查询任务日志
// @Description  分页查询任务执行日志，支持按任务ID、状态、触发方式筛选
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Param        taskId   query    string false "任务ID"
// @Param        status   query    string false "状态（SUCCESS/FAILED/TIMEOUT）"
// @Param        trigger  query    string false "触发方式（CRON/MANUAL）"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.TaskLog}} "成功"
// @Router       /task/log/page [get]
func (tc *TaskController) GetLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize > 100 {
		pageSize = 100
	}
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
		respondInternal(c, err.Error())
		return
	}
	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// GetFuncs 获取内置函数列表
// @Summary      获取内置函数列表
// @Description  列出所有可用的内置函数（FUNC类型任务使用）
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]string} "成功"
// @Router       /task/funcs [get]
func (tc *TaskController) GetFuncs(c *gin.Context) {
	respondOK(c, scheduler.FuncList())
}

// Stats 获取任务统计
// @Summary      获取任务统计
// @Description  获取任务总数、启用数、今日执行统计和最近执行记录
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Router       /task/stats [get]
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

// PreviewNext 预览Cron表达式
// @Summary      预览Cron表达式
// @Description  返回cron表达式接下来n次的执行时间
// @Tags         定时任务
// @Security     BearerAuth
// @Produce      json
// @Param        spec query    string true  "Cron表达式"
// @Param        n    query    int    false "预览次数" default(5)
// @Success      200  {object} models.Response{data=[]string} "成功"
// @Failure      400  {object} models.Response "表达式无效"
// @Router       /task/preview-next [get]
func (tc *TaskController) PreviewNext(c *gin.Context) {
	spec := c.Query("spec")
	n, _ := strconv.Atoi(c.DefaultQuery("n", "5"))
	if spec == "" {
		respondBadRequest(c, "spec required")
		return
	}
	times, err := scheduler.PreviewNext(spec, n)
	if err != nil {
		respondBadRequest(c, err.Error())
		return
	}
	respondOK(c, times)
}

// bulkIDsRequest 批量ID请求
type bulkIDsRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// bulkToggleRequest 批量切换启用状态请求
type bulkToggleRequest struct {
	IDs     []uint `json:"ids" binding:"required"`
	Enabled bool   `json:"enabled"`
}

func shellTasksAllowed(taskType string) bool {
	return taskType != scheduler.TypeShell || os.Getenv("ALLOW_SHELL_TASKS") == "true"
}

// BulkDelete 批量删除任务
// @Summary      批量删除任务
// @Description  批量删除指定的定时任务
// @Tags         定时任务
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     bulkIDsRequest true "任务ID列表"
// @Success      200  {object} models.Response{data=int} "成功删除数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /task/bulk/delete [post]
func (tc *TaskController) BulkDelete(c *gin.Context) {
	var req bulkIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondBadRequest(c, "ids required")
		return
	}
	if err := config.DB.Where("id IN ?", req.IDs).Delete(&models.Task{}).Error; err != nil {
		respondInternal(c, err.Error())
		return
	}
	for _, id := range req.IDs {
		scheduler.Default().Remove(id)
	}
	respondOK(c, len(req.IDs))
}

// BulkToggle 批量切换启用状态
// @Summary      批量切换启用状态
// @Description  批量启用或停用定时任务
// @Tags         定时任务
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     bulkToggleRequest true "任务ID列表和目标状态"
// @Success      200  {object} models.Response{data=int} "影响数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /task/bulk/toggle [post]
func (tc *TaskController) BulkToggle(c *gin.Context) {
	var req bulkToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondBadRequest(c, "ids required")
		return
	}
	var tasks []models.Task
	config.DB.Where("id IN ?", req.IDs).Find(&tasks)
	if req.Enabled {
		for i := range tasks {
			if !shellTasksAllowed(tasks[i].Type) {
				respondBadRequest(c, "SHELL tasks are disabled")
				return
			}
		}
	}
	if err := config.DB.Model(&models.Task{}).Where("id IN ?", req.IDs).Update("enabled", req.Enabled).Error; err != nil {
		respondInternal(c, err.Error())
		return
	}
	for i := range tasks {
		if req.Enabled {
			tasks[i].Enabled = true
			_ = scheduler.Default().Add(&tasks[i])
		} else {
			scheduler.Default().Remove(tasks[i].ID)
		}
	}
	respondOK(c, len(req.IDs))
}
