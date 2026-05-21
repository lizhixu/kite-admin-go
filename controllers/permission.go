package controllers

import (
	"backend/config"
	"backend/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PermissionController struct{}

func (pc *PermissionController) GetRolePermissionsTree(c *gin.Context) {
	roleCode := c.GetString("roleCode")

	// 超级管理员返回所有权限
	if roleCode == "SUPER_ADMIN" {
		var allPermissions []models.Permission
		config.DB.Where("enable = ? OR enable IS NULL", true).Order("`order`").Find(&allPermissions)
		tree := pc.buildAllowedTree(allPermissions, nil, nil)
		c.JSON(http.StatusOK, models.Response{
			Code:      0,
			Message:   "OK",
			Data:      tree,
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 其他角色根据权限过滤
	var role models.Role
	if err := config.DB.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
		c.JSON(http.StatusOK, models.Response{
			Code:      0,
			Message:   "OK",
			Data:      []models.Permission{},
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	allowedPerms := make(map[uint]bool)
	for _, p := range role.Permissions {
		allowedPerms[p.ID] = true
	}

	var allPermissions []models.Permission
	config.DB.Where("enable = ? OR enable IS NULL", true).Order("`order`").Find(&allPermissions)

	tree := pc.buildAllowedTree(allPermissions, nil, allowedPerms)

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      tree,
		OriginUrl: c.Request.URL.Path,
	})
}

func (pc *PermissionController) GetMenuTree(c *gin.Context) {
	var allPermissions []models.Permission
	config.DB.Where("type = ?", "MENU").Order("`order`").Find(&allPermissions)

	tree := pc.buildAllowedTree(allPermissions, nil, nil)

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      tree,
		OriginUrl: c.Request.URL.Path,
	})
}

func (pc *PermissionController) GetTree(c *gin.Context) {
	var allPermissions []models.Permission
	config.DB.Order("`order`").Find(&allPermissions)

	tree := pc.buildAllowedTree(allPermissions, nil, nil)

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      tree,
		OriginUrl: c.Request.URL.Path,
	})
}

func (pc *PermissionController) GetButtonsByParentID(c *gin.Context) {
	parentID := c.Param("parentId")

	var buttons []models.Permission
	config.DB.Where("parent_id = ? AND type = ?", parentID, "BUTTON").Order("`order`").Find(&buttons)

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      buttons,
		OriginUrl: c.Request.URL.Path,
	})
}

func (pc *PermissionController) Create(c *gin.Context) {
	var req struct {
		Name        string  `json:"name" binding:"required"`
		Code        string  `json:"code" binding:"required"`
		Type        string  `json:"type" binding:"required"`
		ParentID    *uint   `json:"parentId"`
		Path        *string `json:"path"`
		Redirect    *string `json:"redirect"`
		Icon        *string `json:"icon"`
		Component   *string `json:"component"`
		Layout      *string `json:"layout"`
		KeepAlive   *bool   `json:"keepAlive"`
		Method      *string `json:"method"`
		Description *string `json:"description"`
		Show        *bool   `json:"show"`
		Enable      *bool   `json:"enable"`
		Order       int     `json:"order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	permission := models.Permission{
		Name:        req.Name,
		Code:        req.Code,
		Type:        req.Type,
		ParentID:    req.ParentID,
		Path:        req.Path,
		Redirect:    req.Redirect,
		Icon:        req.Icon,
		Component:   req.Component,
		Layout:      req.Layout,
		KeepAlive:   req.KeepAlive,
		Method:      req.Method,
		Description: req.Description,
		Order:       req.Order,
	}

	if req.Show != nil {
		permission.Show = req.Show
	} else {
		trueVal := true
		permission.Show = &trueVal
	}
	if req.Enable != nil {
		permission.Enable = req.Enable
	} else {
		trueVal := true
		permission.Enable = &trueVal
	}

	if err := config.DB.Create(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to create permission",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      permission,
		OriginUrl: c.Request.URL.Path,
	})
}

func (pc *PermissionController) Update(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name        *string `json:"name"`
		Code        *string `json:"code"`
		Type        *string `json:"type"`
		ParentID    *uint   `json:"parentId"`
		Path        *string `json:"path"`
		Redirect    *string `json:"redirect"`
		Icon        *string `json:"icon"`
		Component   *string `json:"component"`
		Layout      *string `json:"layout"`
		KeepAlive   *bool   `json:"keepAlive"`
		Method      *string `json:"method"`
		Description *string `json:"description"`
		Show        *bool   `json:"show"`
		Enable      *bool   `json:"enable"`
		Order       *int    `json:"order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Code != nil {
		updates["code"] = *req.Code
	}
	if req.Type != nil {
		updates["type"] = *req.Type
	}
	if req.ParentID != nil {
		updates["parent_id"] = *req.ParentID
	}
	if req.Path != nil {
		updates["path"] = *req.Path
	}
	if req.Redirect != nil {
		updates["redirect"] = *req.Redirect
	}
	if req.Icon != nil {
		updates["icon"] = *req.Icon
	}
	if req.Component != nil {
		updates["component"] = *req.Component
	}
	if req.Layout != nil {
		updates["layout"] = *req.Layout
	}
	if req.KeepAlive != nil {
		updates["keep_alive"] = *req.KeepAlive
	}
	if req.Method != nil {
		updates["method"] = *req.Method
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Show != nil {
		updates["show"] = *req.Show
	}
	if req.Enable != nil {
		updates["enable"] = *req.Enable
	}
	if req.Order != nil {
		updates["order"] = *req.Order
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   "No fields to update",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if err := config.DB.Model(&models.Permission{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to update permission",
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

func (pc *PermissionController) Delete(c *gin.Context) {
	id := c.Param("id")

	var childCount int64
	config.DB.Model(&models.Permission{}).Where("parent_id = ?", id).Count(&childCount)
	if childCount > 0 {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   "Please delete child permissions first",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// Resolve many-to-many foreign key constraint before deleting the permission
	if err := config.DB.Exec("DELETE FROM role_permissions WHERE permission_id = ?", id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to clear permission associations",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if err := config.DB.Delete(&models.Permission{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to delete permission",
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

func (pc *PermissionController) buildAllowedTree(all []models.Permission, parentId *uint, allowed map[uint]bool) []models.Permission {
	var res []models.Permission
	for _, p := range all {
		// 检查是否启用
		if p.Enable != nil && !*p.Enable {
			continue
		}
		// 检查权限
		if allowed != nil && !allowed[p.ID] {
			continue
		}
		// 检查父级匹配
		if (parentId == nil && p.ParentID == nil) || (parentId != nil && p.ParentID != nil && *parentId == *p.ParentID) {
			children := pc.buildAllowedTree(all, &p.ID, allowed)
			if len(children) > 0 {
				p.Children = children
			}
			res = append(res, p)
		}
	}
	return res
}
