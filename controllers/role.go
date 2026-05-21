package controllers

import (
	"backend/config"
	"backend/models"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RoleController struct{}

func (rc *RoleController) GetPage(c *gin.Context) {
	name := c.Query("name")
	pageNo, _ := strconv.Atoi(c.Query("pageNo"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))

	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	query := config.DB.Model(&models.Role{}).Preload("Permissions")
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	var total int64
	query.Count(&total)

	var roles []models.Role
	offset := (pageNo - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Find(&roles)

	var pageData []gin.H
	for _, role := range roles {
		var permissionIds []uint
		for _, perm := range role.Permissions {
			permissionIds = append(permissionIds, perm.ID)
		}
		pageData = append(pageData, gin.H{
			"id":            role.ID,
			"code":          role.Code,
			"name":          role.Name,
			"enable":        role.Enable,
			"permissionIds": permissionIds,
		})
	}

	c.JSON(http.StatusOK, models.Response{
		Code:    0,
		Message: "OK",
		Data: models.PageData{
			PageData: pageData,
			Total:    total,
		},
		OriginUrl: c.Request.URL.String(),
	})
}

func (rc *RoleController) GetAll(c *gin.Context) {
	var roles []models.Role
	config.DB.Where("enable = ?", true).Find(&roles)

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      roles,
		OriginUrl: c.Request.URL.String(),
	})
}

func (rc *RoleController) Create(c *gin.Context) {
	var req struct {
		Name          string `json:"name" binding:"required"`
		Code          string `json:"code" binding:"required"`
		Enable        *bool  `json:"enable"`
		PermissionIds []uint `json:"permissionIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	role := models.Role{Name: req.Name, Code: req.Code}
	if req.Enable != nil {
		role.Enable = *req.Enable
	} else {
		role.Enable = true
	}

	if err := config.DB.Create(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to create role",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 分配权限
	if len(req.PermissionIds) > 0 {
		var permissions []models.Permission
		if err := config.DB.Where("id IN ?", req.PermissionIds).Find(&permissions).Error; err == nil {
			config.DB.Model(&role).Association("Permissions").Append(&permissions)
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      role,
		OriginUrl: c.Request.URL.Path,
	})
}

func (rc *RoleController) Update(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name          string `json:"name"`
		Code          string `json:"code"`
		Enable        *bool  `json:"enable"`
		PermissionIds []uint `json:"permissionIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 查询角色
	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "Role not found",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 更新角色基本信息
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Code != "" {
		updates["code"] = req.Code
	}
	if req.Enable != nil {
		updates["enable"] = *req.Enable
	}

	if len(updates) > 0 {
		if err := config.DB.Model(&role).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to update role",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
	}

	// 更新权限关联
	if req.PermissionIds != nil {
		var permissions []models.Permission
		if err := config.DB.Where("id IN ?", req.PermissionIds).Find(&permissions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to query permissions",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
		if err := config.DB.Model(&role).Association("Permissions").Replace(&permissions); err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to update role permissions",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      true,
		OriginUrl: c.Request.URL.Path,
	})
}

func (rc *RoleController) Delete(c *gin.Context) {
	id := c.Param("id")

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "Role not found",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if role.Code == "SUPER_ADMIN" {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   "Super Admin role cannot be deleted",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 1. Clear many-to-many associations first to resolve foreign key constraints
	config.DB.Model(&role).Association("Permissions").Clear()
	config.DB.Model(&role).Association("Users").Clear()

	// 2. Delete the role
	if err := config.DB.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to delete role",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      true,
		OriginUrl: c.Request.URL.Path,
	})
}

func (rc *RoleController) AddUsers(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		UserIds []uint `json:"userIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "Role not found",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	var users []models.User
	if err := config.DB.Where("id IN ?", req.UserIds).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to query users",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if err := config.DB.Model(&role).Association("Users").Append(&users); err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to add users to role",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      true,
		OriginUrl: c.Request.URL.Path,
	})
}

func (rc *RoleController) RemoveUsers(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		UserIds []uint `json:"userIds" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "Role not found",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	var users []models.User
	if err := config.DB.Where("id IN ?", req.UserIds).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to query users",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if err := config.DB.Model(&role).Association("Users").Delete(&users); err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to remove users from role",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      true,
		OriginUrl: c.Request.URL.Path,
	})
}
