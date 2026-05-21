package middleware

import (
	"backend/config"
	"backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// PermissionMiddleware 权限校验中间件（查询数据库）
func PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleCode := c.GetString("roleCode")

		// 超级管理员拥有所有权限
		if roleCode == "SUPER_ADMIN" {
			c.Next()
			return
		}

		// 查询角色的权限
		var role models.Role
		if err := config.DB.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "Permission denied",
			})
			c.Abort()
			return
		}

		// 检查是否有所需权限
		hasPermission := false
		for _, perm := range role.Permissions {
			if perm.Code == requiredPermission && perm.Enable != nil && *perm.Enable {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "Permission denied",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequirePermission 便捷函数，用于路由中快速添加权限校验
func RequirePermission(permission string) gin.HandlerFunc {
	return PermissionMiddleware(permission)
}
