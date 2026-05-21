package middleware

import (
	"backend/config"
	"backend/models"
	"bytes"
	"io"
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
			if len(bodyStr) > 5000 {
				bodyStr = bodyStr[:5000] + "... (truncated)"
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
		if len(responseStr) > 5000 {
			responseStr = responseStr[:5000] + "... (truncated)"
		}

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
			username = usernameVal.(string)
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
			config.DB.Create(&l)
		}(logEntry)
	}
}
