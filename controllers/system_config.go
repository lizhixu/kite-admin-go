package controllers

import (
	"backend/config"
	"backend/models"

	"github.com/gin-gonic/gin"
)

type SystemConfigController struct{}

// systemConfigRequest 系统配置请求
type systemConfigRequest struct {
	SiteName  string `json:"siteName" binding:"required"`
	Logo      string `json:"logo"`
	Copyright string `json:"copyright"`
	Favicon   string `json:"favicon"`
}

// Get 获取系统配置
// @Summary      获取系统配置
// @Description  获取当前系统配置信息
// @Tags         系统配置
// @Produce      json
// @Success      200 {object} models.Response{data=models.SystemConfig} "成功"
// @Router       /system/config [get]
func (sc *SystemConfigController) Get(c *gin.Context) {
	var cfg models.SystemConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondOK(c, nil)
		return
	}
	respondOK(c, cfg)
}

// Save 保存系统配置
// @Summary      保存系统配置
// @Description  创建或更新系统配置
// @Tags         系统配置
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     systemConfigRequest true "配置信息"
// @Success      200  {object} models.Response{data=models.SystemConfig} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /system/config [put]
func (sc *SystemConfigController) Save(c *gin.Context) {
	var req systemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, 400, err.Error())
		return
	}

	var cfg models.SystemConfig
	err := config.DB.First(&cfg).Error

	cfg.SiteName = req.SiteName
	cfg.Logo = req.Logo
	cfg.Copyright = req.Copyright
	cfg.Favicon = req.Favicon

	if err != nil {
		// Create
		if err := config.DB.Create(&cfg).Error; err != nil {
			respondErr(c, 500, "Failed to save system config")
			return
		}
	} else {
		// Update
		if err := config.DB.Save(&cfg).Error; err != nil {
			respondErr(c, 500, "Failed to save system config")
			return
		}
	}

	respondOK(c, cfg)
}