package middleware

import (
	"backend/config"
	"backend/models"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 角色权限缓存（避免每次请求查库）
var (
	permCache    = make(map[string]permCacheEntry)
	permCacheMu  sync.RWMutex
	permCacheTTL = 5 * time.Minute
)

type permCacheEntry struct {
	permissions []models.Permission
	expiresAt   time.Time
}

// GetRolePermissions 获取角色权限（带缓存），供其他包使用
func GetRolePermissions(roleCode string) ([]models.Permission, bool) {
	permCacheMu.RLock()
	entry, exists := permCache[roleCode]
	permCacheMu.RUnlock()

	if exists && time.Now().Before(entry.expiresAt) {
		return entry.permissions, true
	}

	// 缓存未命中，查询数据库
	var role models.Role
	if err := config.DB.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
		return nil, false
	}

	// 写入缓存
	permCacheMu.Lock()
	permCache[roleCode] = permCacheEntry{
		permissions: role.Permissions,
		expiresAt:   time.Now().Add(permCacheTTL),
	}
	permCacheMu.Unlock()

	return role.Permissions, true
}

// InvalidatePermCache 使指定角色的权限缓存失效（角色权限变更时调用）
func InvalidatePermCache(roleCode string) {
	permCacheMu.Lock()
	delete(permCache, roleCode)
	permCacheMu.Unlock()
}

// InvalidateAllPermCache 使所有权限缓存失效
func InvalidateAllPermCache() {
	permCacheMu.Lock()
	permCache = make(map[string]permCacheEntry)
	permCacheMu.Unlock()
}

// HasPermission 检查当前请求角色是否拥有指定权限
func HasPermission(c *gin.Context, permissionCode string) bool {
	roleCode := c.GetString("roleCode")
	if roleCode == models.RoleSuperAdmin {
		return true
	}
	permissions, ok := GetRolePermissions(roleCode)
	if !ok {
		return false
	}
	for _, perm := range permissions {
		if perm.Code == permissionCode && perm.Enable != nil && *perm.Enable {
			return true
		}
	}
	return false
}

// PermissionMiddleware 权限校验中间件（带缓存）
func PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		roleCode := c.GetString("roleCode")

		// 超级管理员拥有所有权限
		if roleCode == models.RoleSuperAdmin {
			c.Next()
			return
		}

		// 查询角色的权限（带缓存）
		permissions, ok := GetRolePermissions(roleCode)
		if !ok {
			c.JSON(http.StatusForbidden, models.Response{
				Code:    403,
				Message: "Permission denied",
			})
			c.Abort()
			return
		}

		// 检查是否有所需权限
		hasPermission := false
		for _, perm := range permissions {
			if perm.Code == requiredPermission && perm.Enable != nil && *perm.Enable {
				hasPermission = true
				break
			}
		}

		if !hasPermission {
			c.JSON(http.StatusForbidden, models.Response{
				Code:    403,
				Message: "Permission denied",
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
