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

// queueUpdateRequest 仅允许编辑运维参数（name、handler 等代码侧字段不可改）
type queueUpdateRequest struct {
	Description string `json:"description"`
	Concurrency int    `json:"concurrency"`
	Timeout     int    `json:"timeout"`
	MaxRetries  int    `json:"maxRetries"`
	Status      string `json:"status"`
}

func (qc *QueueController) GetOne(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var q models.Queue
	if err := config.DB.First(&q, id).Error; err != nil {
		respondErr(c, 404, "queue not found")
		return
	}
	respondOK(c, q)
}

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

// Toggle 切换运行/暂停
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

// Stats 队列与任务汇总
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

// GetHandlers 列出代码侧已注册的 tube 名称（前端「已注册」徽标用）
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

// GetJobs 分页查询某队列任务，支持状态 + 日期范围过滤
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

// AddJob 投递一个任务到队列（debug / UI 用）
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

// BulkAddJobs 批量投递
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

// KickJob 复活单个 FAILED 任务为 PENDING
func (qc *QueueController) KickJob(c *gin.Context) {
	jobID, _ := strconv.Atoi(c.Param("jobId"))
	if err := queue.Default().Kick(uint(jobID)); err != nil {
		respondErr(c, 400, err.Error())
		return
	}
	respondOK(c, true)
}

// KickAll 复活某队列内所有 FAILED 任务为 PENDING
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
func (qc *QueueController) DeleteJob(c *gin.Context) {
	jobID, _ := strconv.Atoi(c.Param("jobId"))
	if err := config.DB.Delete(&models.QueueJob{}, jobID).Error; err != nil {
		respondErr(c, 500, err.Error())
		return
	}
	respondOK(c, true)
}

// ClearJobs 清空任务；支持 status / before 过滤
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
