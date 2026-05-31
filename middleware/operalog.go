package middleware

import (
	"backend/config"
	"backend/models"
	"bytes"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w responseBodyWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func OperationLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		startTime := time.Now()

		// Skip logging for GET syslog/list to avoid log spam when viewing logs
		if c.Request.URL.Path == "/syslog/list" {
			c.Next()
			return
		}

		var paramsStr string
		if c.Request.URL.RawQuery != "" {
			paramsStr = c.Request.URL.RawQuery
		}

		if c.Request.Method != "GET" && c.Request.Body != nil {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			bodyStr := string(bodyBytes)
			// 敏感路径脱敏：掩码 password、captcha 等字段
			if isSensitivePath(c.Request.URL.Path) {
				bodyStr = maskSensitiveFields(bodyStr)
			}
			if paramsStr != "" {
				paramsStr = paramsStr + " | Body: " + bodyStr
			} else {
				paramsStr = bodyStr
			}
		}

		// Intercept response
		w := &responseBodyWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = w

		// Process request
		c.Next()

		// Stop timer
		endTime := time.Now()
		latency := endTime.Sub(startTime).Milliseconds()

		responseStr := w.body.String()

		// Extract user from context
		userIdVal, exists := c.Get("userID")
		userId := uint(0)
		if exists {
			if id, ok := userIdVal.(uint); ok {
				userId = id
			} else if idFloat, ok := userIdVal.(float64); ok {
				userId = uint(idFloat)
			}
		}

		usernameVal, exists := c.Get("username")
		username := ""
		if exists && usernameVal != nil {
			if s, ok := usernameVal.(string); ok {
				username = s
			}
		}

		userAgent := strings.Join(c.Request.Header["User-Agent"], " ")
		if len(userAgent) > 250 {
			userAgent = userAgent[:250]
		}

		clientIP := c.ClientIP()
		if clientIP == "::1" {
			clientIP = "127.0.0.1"
		}

		logEntry := models.SysLog{
			UserID:     userId,
			Username:   username,
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			Params:     paramsStr,
			Response:   responseStr,
			IP:         clientIP,
			StatusCode: c.Writer.Status(),
			Latency:    latency,
			UserAgent:  userAgent,
		}

		// Fast async insert
		go func(l models.SysLog) {
			if err := config.DB.Create(&l).Error; err != nil {
				log.Printf("failed to create syslog: %v", err)
			}
		}(logEntry)
	}
}

// isSensitivePath 判断路径是否包含敏感数据（密码、验证码等）
func isSensitivePath(path string) bool {
	sensitivePaths := []string{
		"/auth/login",
		"/user/password/reset",
	}
	for _, p := range sensitivePaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// maskSensitiveFields 将请求体中的敏感字段值替换为 ***
var sensitiveFieldRe = regexp.MustCompile(`("(?i:password|captcha|captchaId)"\s*:\s*")([^"]*?)"`)

func maskSensitiveFields(body string) string {
	return sensitiveFieldRe.ReplaceAllString(body, `${1}***"`)
}
