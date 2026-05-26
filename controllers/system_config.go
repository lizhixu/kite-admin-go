package controllers

import (
	"backend/config"
	"backend/models"

	"github.com/gin-gonic/gin"
)

type SystemConfigController struct{}

type systemConfigRequest struct {
	SiteName  string `json:"siteName" binding:"required"`
	Logo      string `json:"logo"`
	Copyright string `json:"copyright"`
	Favicon   string `json:"favicon"`
}

func (sc *SystemConfigController) Get(c *gin.Context) {
	var cfg models.SystemConfig
	if err := config.DB.First(&cfg).Error; err != nil {
		respondOK(c, nil)
		return
	}
	respondOK(c, cfg)
}

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