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

// recordLoginLog 异步记录登录成功日志
func recordLoginLog(userID uint, username, ip, userAgent string) {
	// 截断 UserAgent 防止过长
	if len(userAgent) > 250 {
		userAgent = userAgent[:250]
	}
	log := models.LoginLog{
		UserID:    userID,
		Username:  username,
		IP:        ip,
		UserAgent: userAgent,
		Success:   true,
		Message:   "登录成功",
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

// Login 用户登录
// @Summary      用户登录
// @Description  通过用户名、密码和验证码登录系统
// @Tags         认证管理
// @Accept       json
// @Produce      json
// @Param        body body     LoginRequest true "登录信息"
// @Success      200  {object} models.Response "成功，data包含accessToken"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /auth/login [post]
func (ac *AuthController) Login(c *gin.Context) {
	ip := getClientIP(c)
	userAgent := c.Request.UserAgent()

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	// 校验验证码
	captchaID, err := c.Cookie("captcha_id")
	if err != nil || captchaID == "" {
		respondErr(c, http.StatusUnauthorized, 10003, "验证码已过期，请刷新")
		return
	}
	if !utils.VerifyCaptcha(captchaID, strings.TrimSpace(req.Captcha)) {
		respondErr(c, http.StatusUnauthorized, 10003, "验证码错误")
		return
	}

	var user models.User
	if err := config.DB.Preload("Roles").Preload("Profile").Where("username = ?", req.Username).First(&user).Error; err != nil {
		respondErr(c, http.StatusUnauthorized, 10004, "账号或密码错误")
		return
	}

	if !utils.CheckPassword(req.Password, user.Password) {
		respondErr(c, http.StatusUnauthorized, 10004, "账号或密码错误")
		return
	}

	roleCode := ""
	if len(user.Roles) > 0 {
		roleCode = user.Roles[0].Code
	}

	cfg := config.LoadConfig()
	token, err := utils.GenerateToken(user.ID, user.Username, roleCode, cfg.JWT.Secret, cfg.JWT.ExpireTime)
	if err != nil {
		respondInternal(c, "Failed to generate token")
		return
	}

	recordLoginLog(user.ID, user.Username, ip, userAgent)
	respondOK(c, gin.H{"accessToken": token})
}

// GetCaptcha 获取验证码
// @Summary      获取验证码
// @Description  获取图形验证码，验证码ID通过Cookie返回
// @Tags         认证管理
// @Produce      png
// @Success      200 {string} string "验证码图片"
// @Router       /auth/captcha [get]
func (ac *AuthController) GetCaptcha(c *gin.Context) {
	id, imgBytes, err := utils.GenerateCaptcha()
	if err != nil {
		respondInternal(c, "Failed to generate captcha")
		return
	}

	// 通过 Cookie 传递验证码 ID
	c.SetCookie("captcha_id", id, 300, "/", "", false, false)

	// 直接返回 PNG 图片
	c.Data(http.StatusOK, "image/png", imgBytes)
}

// SwitchRole 切换当前角色
// @Summary      切换当前角色
// @Description  切换当前用户的角色，返回新的Token
// @Tags         认证管理
// @Security     BearerAuth
// @Produce      json
// @Param        roleCode path     string true "角色编码"
// @Success      200      {object} models.Response "成功，data包含accessToken"
// @Failure      500      {object} models.Response "生成Token失败"
// @Router       /auth/current-role/switch/{roleCode} [post]
func (ac *AuthController) SwitchRole(c *gin.Context) {
	roleCode := c.Param("roleCode")
	userID := c.GetUint("userID")
	username := c.GetString("username")

	// 校验用户是否拥有目标角色
	var user models.User
	if err := config.DB.Preload("Roles").First(&user, userID).Error; err != nil {
		respondForbidden(c, "用户不存在")
		return
	}
	allowed := false
	for _, r := range user.Roles {
		if r.Code == roleCode {
			allowed = true
			break
		}
	}
	if !allowed {
		respondForbidden(c, "无权切换到该角色")
		return
	}

	cfg := config.LoadConfig()
	token, err := utils.GenerateToken(userID, username, roleCode, cfg.JWT.Secret, cfg.JWT.ExpireTime)
	if err != nil {
		respondInternal(c, "Failed to generate token")
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

// Logout 用户登出
// @Summary      用户登出
// @Description  退出登录（客户端应清除Token）
// @Tags         认证管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Router       /auth/logout [post]
func (ac *AuthController) Logout(c *gin.Context) {
	respondOK(c, true)
}
