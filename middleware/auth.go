package middleware

import (
	"backend/config"
	"backend/models"
	"backend/utils"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, models.Response{
				Code:    401,
				Message: "Authorization header required",
			})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, models.Response{
				Code:    401,
				Message: "Invalid authorization format",
			})
			c.Abort()
			return
		}

		cfg := config.LoadConfig()
		claims, err := utils.ParseToken(parts[1], cfg.JWT.Secret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, models.Response{
				Code:    401,
				Message: "Invalid token",
			})
			c.Abort()
			return
		}

		var user models.User
		if err := config.DB.Preload("Roles").First(&user, claims.UserID).Error; err != nil || !user.Enable {
			c.JSON(http.StatusUnauthorized, models.Response{
				Code:    401,
				Message: "Invalid token",
			})
			c.Abort()
			return
		}

		if claims.RoleCode != "" {
			hasRole := false
			for _, role := range user.Roles {
				if role.Code == claims.RoleCode && role.Enable {
					hasRole = true
					break
				}
			}
			if !hasRole {
				c.JSON(http.StatusForbidden, models.Response{
					Code:    403,
					Message: "Permission denied",
				})
				c.Abort()
				return
			}
		}

		c.Set("userID", user.ID)
		c.Set("username", user.Username)
		c.Set("roleCode", claims.RoleCode)
		c.Next()
	}
}
