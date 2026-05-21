package controllers

import (
	"backend/config"
	"backend/models"
	"backend/utils"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthController struct{}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Captcha  string `json:"captcha"`
	IsQuick  bool   `json:"isQuick"`
}

func (ac *AuthController) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 非一键体验模式需校验验证码
	if !req.IsQuick {
		captchaID, err := c.Cookie("captcha_id")
		if err != nil || captchaID == "" {
			c.JSON(http.StatusOK, models.Response{
				Code:      10003,
				Message:   "验证码已过期，请刷新",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
		if !utils.VerifyCaptcha(captchaID, strings.TrimSpace(req.Captcha)) {
			c.JSON(http.StatusOK, models.Response{
				Code:      10003,
				Message:   "验证码错误",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
	}

	var user models.User
	if err := config.DB.Preload("Roles").Preload("Profile").Where("username = ?", req.Username).First(&user).Error; err != nil {
		c.JSON(http.StatusOK, models.Response{
			Code:      10004,
			Message:   "账号或密码错误",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if !utils.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusOK, models.Response{
			Code:      10004,
			Message:   "账号或密码错误",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	roleCode := ""
	if len(user.Roles) > 0 {
		roleCode = user.Roles[0].Code
	}

	cfg := config.LoadConfig()
	token, err := utils.GenerateToken(user.ID, user.Username, roleCode, cfg.JWT.Secret, cfg.JWT.ExpireTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to generate token",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:    0,
		Message: "OK",
		Data: gin.H{
			"accessToken": token,
		},
		OriginUrl: c.Request.URL.Path,
	})
}

func (ac *AuthController) GetCaptcha(c *gin.Context) {
	id, imgBytes, err := utils.GenerateCaptcha()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to generate captcha",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 通过 Cookie 传递验证码 ID
	c.SetCookie("captcha_id", id, 300, "/", "", false, false)

	// 直接返回 PNG 图片
	c.Data(http.StatusOK, "image/png", imgBytes)
}

func (ac *AuthController) SwitchRole(c *gin.Context) {
	roleCode := c.Param("roleCode")
	userID := c.GetUint("userID")
	username := c.GetString("username")

	cfg := config.LoadConfig()
	token, err := utils.GenerateToken(userID, username, roleCode, cfg.JWT.Secret, cfg.JWT.ExpireTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to generate token",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:    0,
		Message: "OK",
		Data: gin.H{
			"accessToken": token,
		},
		OriginUrl: fmt.Sprintf("/auth/current-role/switch/%s", roleCode),
	})
}

func (ac *AuthController) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      true,
		OriginUrl: c.Request.URL.Path,
	})
}
