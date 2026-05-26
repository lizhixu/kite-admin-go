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

// getClientIP 获取客户端真实 IP
func getClientIP(c *gin.Context) string {
	ip := c.ClientIP()
	if ip == "::1" {
		ip = "127.0.0.1"
	}
	return ip
}

// recordLoginLog 异步记录登录日志
func recordLoginLog(userID uint, username, ip, userAgent string, success bool, message string) {
	// 截断 UserAgent 防止过长
	if len(userAgent) > 250 {
		userAgent = userAgent[:250]
	}
	log := models.LoginLog{
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		Message:   message,
	}
	go func() {
		config.DB.Create(&log)
	}()
}

type AuthController struct{}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Captcha  string `json:"captcha" binding:"required"`
}

func (ac *AuthController) Login(c *gin.Context) {
	ip := getClientIP(c)
	userAgent := c.Request.UserAgent()

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		recordLoginLog(0, req.Username, ip, userAgent, false, "参数错误: "+err.Error())
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 校验验证码
	captchaID, err := c.Cookie("captcha_id")
	if err != nil || captchaID == "" {
		recordLoginLog(0, req.Username, ip, userAgent, false, "验证码已过期")
		c.JSON(http.StatusOK, models.Response{
			Code:      10003,
			Message:   "验证码已过期，请刷新",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}
	if !utils.VerifyCaptcha(captchaID, strings.TrimSpace(req.Captcha)) {
		recordLoginLog(0, req.Username, ip, userAgent, false, "验证码错误")
		c.JSON(http.StatusOK, models.Response{
			Code:      10003,
			Message:   "验证码错误",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	var user models.User
	if err := config.DB.Preload("Roles").Preload("Profile").Where("username = ?", req.Username).First(&user).Error; err != nil {
		recordLoginLog(0, req.Username, ip, userAgent, false, "用户不存在")
		c.JSON(http.StatusOK, models.Response{
			Code:      10004,
			Message:   "账号或密码错误",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if !utils.CheckPassword(req.Password, user.Password) {
		recordLoginLog(user.ID, user.Username, ip, userAgent, false, "密码错误")
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
		recordLoginLog(user.ID, user.Username, ip, userAgent, false, "生成token失败")
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to generate token",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	recordLoginLog(user.ID, user.Username, ip, userAgent, true, "登录成功")
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
