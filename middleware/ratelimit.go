package middleware

import (
	"backend/models"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimit 基于 IP 的速率限制中间件
// maxRequests: 窗口期内最大请求数
// window: 时间窗口
func RateLimit(maxRequests int, window time.Duration) gin.HandlerFunc {
	type client struct {
		count   int
		resetAt time.Time
	}

	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// 定期清理过期条目
	go func() {
		ticker := time.NewTicker(window * 2)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for ip, cl := range clients {
				if now.After(cl.resetAt) {
					delete(clients, ip)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		ip := c.ClientIP()

		mu.Lock()
		cl, exists := clients[ip]
		now := time.Now()

		if !exists || now.After(cl.resetAt) {
			clients[ip] = &client{count: 1, resetAt: now.Add(window)}
			mu.Unlock()
			c.Next()
			return
		}

		cl.count++
		if cl.count > maxRequests {
			mu.Unlock()
			c.JSON(http.StatusTooManyRequests, models.Response{
				Code:    429,
				Message: "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}
		mu.Unlock()

		c.Next()
	}
}
