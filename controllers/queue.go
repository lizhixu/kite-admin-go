package controllers

import (
	"backend/config"
	"backend/models"
	"backend/queue"
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type QueueController struct{}

// queueUpdateRequest 队列更新请求（仅允许编辑运维参数）
type queueUpdateRequest struct {
	Description string `json:"description"`
	Concurrency int    `json:"concurrency"`
	Timeout     int    `json:"timeout"`
	MaxRetries  int    `json:"maxRetries"`
	Status      string `json:"status"`
}

// GetOne 获取队列详情
// @Summary      获取队列详情
// @Description  根据ID获取队列详情
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "队列ID"
// @Success      200 {object} models.Response{data=models.Queue} "成功"
// @Failure      404 {object} models.Response "队列不存在"
// @Router       /queue/{id} [get]
func (qc *QueueController) GetOne(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var q models.Queue
	if err := config.DB.First(&q, id).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}
	respondOK(c, q)
}

// GetPage 分页查询队列
// @Summary      分页查询队列
// @Description  分页查询队列列表，支持按名称和状态筛选
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Param        name     query    string false "队列名称（模糊搜索）"
// @Param        status   query    string false "状态（RUNNING/PAUSED）"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.Queue}} "成功"
// @Router       /queue/page [get]
func (qc *QueueController) GetPage(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	name := c.Query("name")
	status := c.Query("status")

	q := config.DB.Model(&models.Queue{})
	if name != "" {
		q = q.Where("name LIKE ?", "%"+name+"%")
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	var total int64
	q.Count(&total)

	var rows []models.Queue
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}

	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// Update 更新队列配置
// @Summary      更新队列配置
// @Description  更新队列的运维参数（并发数、超时、重试等）
// @Tags         消息队列
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                  true "队列ID"
// @Param        body body     queueUpdateRequest true "队列配置"
// @Success      200  {object} models.Response{data=models.Queue} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "队列不存在"
// @Router       /queue/{id} [patch]
func (qc *QueueController) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req queueUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	var q models.Queue
	if err := config.DB.First(&q, id).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}
	q.Description = req.Description
	if req.Concurrency > 0 {
		q.Concurrency = req.Concurrency
	}
	if req.Timeout > 0 {
		q.Timeout = req.Timeout
	}
	q.MaxRetries = req.MaxRetries
	if req.Status != "" {
		q.Status = req.Status
	}
	if err := config.DB.Save(&q).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, q)
}

// Delete 删除队列
// @Summary      删除队列
// @Description  删除队列及其所有任务
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "队列ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "删除失败"
// @Router       /queue/{id} [delete]
func (qc *QueueController) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := config.DB.Where("queue_id = ?", id).Delete(&models.QueueJob{}).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	if err := config.DB.Delete(&models.Queue{}, id).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// Toggle 切换队列状态
// @Summary      切换队列状态
// @Description  切换队列的运行/暂停状态
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "队列ID"
// @Success      200 {object} models.Response{data=models.Queue} "成功"
// @Failure      404 {object} models.Response "队列不存在"
// @Router       /queue/{id}/toggle [patch]
func (qc *QueueController) Toggle(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var q models.Queue
	if err := config.DB.First(&q, id).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}
	if q.Status == queue.StatusRunning {
		q.Status = queue.StatusPaused
	} else {
		q.Status = queue.StatusRunning
	}
	if err := config.DB.Save(&q).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, q)
}

// Stats 获取队列统计
// @Summary      获取队列统计
// @Description  获取队列和任务的汇总统计数据
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Router       /queue/stats [get]
func (qc *QueueController) Stats(c *gin.Context) {
	var (
		total, running, paused int64
		jobTotal, jobPending   int64
		jobRunning             int64
		jobSuccess, jobFailed  int64
		successToday           int64
		failedToday            int64
	)
	config.DB.Model(&models.Queue{}).Count(&total)
	config.DB.Model(&models.Queue{}).Where("status = ?", queue.StatusRunning).Count(&running)
	paused = total - running

	config.DB.Model(&models.QueueJob{}).Count(&jobTotal)
	config.DB.Model(&models.QueueJob{}).Where("status = ?", queue.JobPending).Count(&jobPending)
	config.DB.Model(&models.QueueJob{}).Where("status = ?", queue.JobRunning).Count(&jobRunning)
	config.DB.Model(&models.QueueJob{}).Where("status = ?", queue.JobSuccess).Count(&jobSuccess)
	config.DB.Model(&models.QueueJob{}).Where("status = ?", queue.JobFailed).Count(&jobFailed)

	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	config.DB.Model(&models.QueueJob{}).Where("completed_at >= ? AND status = ?", start, queue.JobSuccess).Count(&successToday)
	config.DB.Model(&models.QueueJob{}).Where("completed_at >= ? AND status = ?", start, queue.JobFailed).Count(&failedToday)

	respondOK(c, gin.H{
		"total":        total,
		"running":      running,
		"paused":       paused,
		"jobTotal":     jobTotal,
		"jobPending":   jobPending,
		"jobRunning":   jobRunning,
		"jobSuccess":   jobSuccess,
		"jobFailed":    jobFailed,
		"successToday": successToday,
		"failedToday":  failedToday,
	})
}

// GetHandlers 获取已注册处理器
// @Summary      获取已注册处理器
// @Description  列出代码侧已注册的队列处理器名称
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]string} "成功"
// @Router       /queue/handlers [get]
func (qc *QueueController) GetHandlers(c *gin.Context) {
	respondOK(c, queue.HandlerList())
}

// ====== Jobs ======

type jobRequest struct {
	Payload    string `json:"payload"`
	MaxRetries int    `json:"maxRetries"`
}

type bulkJobsRequest struct {
	Items []jobRequest `json:"items"`
}

// GetJobs 查询队列任务
// @Summary      查询队列任务
// @Description  分页查询指定队列的任务，支持状态和日期范围筛选
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id       path     int    true  "队列ID"
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Param        status   query    string false "状态（PENDING/RUNNING/SUCCESS/FAILED）"
// @Param        from     query    string false "开始时间"
// @Param        to       query    string false "结束时间"
// @Success      200      {object} models.Response{data=models.PageData{pageData=[]models.QueueJob}} "成功"
// @Router       /queue/{id}/jobs [get]
func (qc *QueueController) GetJobs(c *gin.Context) {
	queueID, _ := strconv.Atoi(c.Param("id"))
	page, _ := strconv.Atoi(c.DefaultQuery("pageNo", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	status := c.Query("status")
	from := c.Query("from")
	to := c.Query("to")

	q := config.DB.Model(&models.QueueJob{}).Where("queue_id = ?", queueID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if from != "" {
		if t, err := parseTime(from); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if to != "" {
		if t, err := parseTime(to); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}

	var total int64
	q.Count(&total)

	var rows []models.QueueJob
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, gin.H{"pageData": rows, "total": total})
}

// AddJob 投递单个任务
// @Summary      投递单个任务
// @Description  向指定队列投递一个任务
// @Tags         消息队列
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int          true "队列ID"
// @Param        body body     jobRequest   true "任务信息"
// @Success      200  {object} models.Response{data=models.QueueJob} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "队列不存在"
// @Router       /queue/{id}/job [post]
func (qc *QueueController) AddJob(c *gin.Context) {
	queueID, _ := strconv.Atoi(c.Param("id"))
	var req jobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	var qm models.Queue
	if err := config.DB.First(&qm, queueID).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}

	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = qm.MaxRetries
	}

	job := models.QueueJob{
		QueueID:    qm.ID,
		Payload:    req.Payload,
		Status:     queue.JobPending,
		MaxRetries: maxRetries,
	}
	if err := config.DB.Create(&job).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	config.DB.Model(&models.Queue{}).Where("id = ?", qm.ID).
		UpdateColumn("total_jobs", config.DB.Raw("total_jobs + 1"))
	respondOK(c, job)
}

// BulkAddJobs 批量投递任务
// @Summary      批量投递任务
// @Description  向指定队列批量投递多个任务
// @Tags         消息队列
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int              true "队列ID"
// @Param        body body     bulkJobsRequest  true "任务列表"
// @Success      200  {object} models.Response{data=int} "成功创建数量"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "队列不存在"
// @Router       /queue/{id}/jobs/bulk [post]
func (qc *QueueController) BulkAddJobs(c *gin.Context) {
	queueID, _ := strconv.Atoi(c.Param("id"))
	var req bulkJobsRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Items) == 0 {
		respondErr(c, 400, "items required")
		return
	}
	var qm models.Queue
	if err := config.DB.First(&qm, queueID).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}
	jobs := make([]models.QueueJob, 0, len(req.Items))
	for _, it := range req.Items {
		retries := it.MaxRetries
		if retries <= 0 {
			retries = qm.MaxRetries
		}
		jobs = append(jobs, models.QueueJob{
			QueueID:    qm.ID,
			Payload:    it.Payload,
			Status:     queue.JobPending,
			MaxRetries: retries,
		})
	}
	if err := config.DB.Create(&jobs).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	config.DB.Model(&models.Queue{}).Where("id = ?", qm.ID).
		UpdateColumn("total_jobs", config.DB.Raw("total_jobs + ?", len(jobs)))
	respondOK(c, len(jobs))
}

// KickJob 重试单个任务
// @Summary      重试单个任务
// @Description  将单个FAILED任务重新设为PENDING状态
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        jobId  path     int true "任务ID"
// @Success      200    {object} models.Response "成功"
// @Failure      400    {object} models.Response "操作失败"
// @Router       /queue/job/{jobId}/kick [post]
func (qc *QueueController) KickJob(c *gin.Context) {
	jobID, _ := strconv.Atoi(c.Param("jobId"))
	if err := queue.Default().Kick(uint(jobID)); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	respondOK(c, true)
}

// KickAll 重试队列所有失败任务
// @Summary      重试队列所有失败任务
// @Description  将指定队列内所有FAILED任务重新设为PENDING
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "队列ID"
// @Success      200 {object} models.Response "成功，data包含affected字段"
// @Failure      500 {object} models.Response "操作失败"
// @Router       /queue/{id}/kick [post]
func (qc *QueueController) KickAll(c *gin.Context) {
	queueID, _ := strconv.Atoi(c.Param("id"))
	n, err := queue.Default().KickAll(uint(queueID))
	if err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, gin.H{"affected": n})
}

// DeleteJob 删除单个任务
// @Summary      删除单个任务
// @Description  删除指定的队列任务
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        jobId  path     int true "任务ID"
// @Success      200    {object} models.Response "成功"
// @Failure      500    {object} models.Response "删除失败"
// @Router       /queue/job/{jobId} [delete]
func (qc *QueueController) DeleteJob(c *gin.Context) {
	jobID, _ := strconv.Atoi(c.Param("jobId"))
	if err := config.DB.Delete(&models.QueueJob{}, jobID).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// ClearJobs 清空队列任务
// @Summary      清空队列任务
// @Description  清空指定队列的任务，支持按状态和时间过滤
// @Tags         消息队列
// @Security     BearerAuth
// @Produce      json
// @Param        id      path     int    true  "队列ID"
// @Param        status  query    string false "状态过滤"
// @Param        before  query    string false "截止时间"
// @Success      200     {object} models.Response "成功"
// @Failure      500     {object} models.Response "清空失败"
// @Router       /queue/{id}/jobs [delete]
func (qc *QueueController) ClearJobs(c *gin.Context) {
	queueID, _ := strconv.Atoi(c.Param("id"))
	status := c.Query("status")
	before := c.Query("before")
	q := config.DB.Where("queue_id = ?", queueID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if before != "" {
		if t, err := parseTime(before); err == nil {
			q = q.Where("completed_at < ?", t)
		}
	}
	if err := q.Delete(&models.QueueJob{}).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// parseTime 兼容 RFC3339 / "2006-01-02 15:04:05" / unix 毫秒数字字符串
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.UnixMilli(ms), nil
	}
	return time.Time{}, fmt.Errorf("unrecognized time format: %s", s)
}
