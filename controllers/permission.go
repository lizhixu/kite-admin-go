package controllers

import (
	"backend/config"
	"backend/models"
	"log"

	"github.com/gin-gonic/gin"
)

type PermissionController struct{}

type createPermissionRequest struct {
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

type updatePermissionRequest struct {
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

// GetRolePermissionsTree 获取当前角色权限树
// @Summary      获取当前角色权限树
// @Description  根据当前登录用户的角色获取权限树（SUPER_ADMIN返回全部）
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.Permission} "成功"
// @Router       /role/permissions/tree [get]
func (pc *PermissionController) GetRolePermissionsTree(c *gin.Context) {
	roleCode := c.GetString("roleCode")

	// 超级管理员返回所有权限
	if roleCode == models.RoleSuperAdmin {
		var allPermissions []models.Permission
		if err := config.DB.Where("enable = ? OR enable IS NULL", true).Order("`order`").Find(&allPermissions).Error; err != nil {
			log.Printf("GetRolePermissionsTree find error: %v", err)
		}
		tree := pc.buildAllowedTree(allPermissions, nil, nil)
		respondOK(c, tree)
		return
	}

	// 其他角色根据权限过滤
	var role models.Role
	if err := config.DB.Preload("Permissions").Where("code = ?", roleCode).First(&role).Error; err != nil {
		respondOK(c, []models.Permission{})
		return
	}

	allowedPerms := make(map[uint]bool)
	for _, p := range role.Permissions {
		allowedPerms[p.ID] = true
	}

	var allPermissions []models.Permission
	if err := config.DB.Where("enable = ? OR enable IS NULL", true).Order("`order`").Find(&allPermissions).Error; err != nil {
		log.Printf("GetRolePermissionsTree find all error: %v", err)
	}

	tree := pc.buildAllowedTree(allPermissions, nil, allowedPerms)
	respondOK(c, tree)
}

// GetMenuTree 获取菜单树
// @Summary      获取菜单树
// @Description  获取所有MENU类型的权限树
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.Permission} "成功"
// @Router       /permission/menu/tree [get]
func (pc *PermissionController) GetMenuTree(c *gin.Context) {
	var allPermissions []models.Permission
	if err := config.DB.Where("type = ?", "MENU").Order("`order`").Find(&allPermissions).Error; err != nil {
		log.Printf("GetMenuTree find error: %v", err)
	}

	tree := pc.buildAllowedTree(allPermissions, nil, nil)
	respondOK(c, tree)
}

// ValidateMenuPath 校验菜单路径是否存在，用于前端区分 403/404
// @Summary      校验菜单路径
// @Description  判断系统中是否存在指定启用菜单路径
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Param        path query    string true "菜单路径"
// @Success      200  {object} models.Response{data=bool} "成功"
// @Router       /permission/menu/validate [get]
func (pc *PermissionController) ValidateMenuPath(c *gin.Context) {
	menuPath := c.Query("path")
	if menuPath == "" {
		respondOK(c, false)
		return
	}

	var count int64
	if err := config.DB.Model(&models.Permission{}).
		Where("type = ? AND path = ? AND (enable = ? OR enable IS NULL)", "MENU", menuPath, true).
		Count(&count).Error; err != nil {
		log.Printf("ValidateMenuPath count error: %v", err)
		respondOK(c, false)
		return
	}
	respondOK(c, count > 0)
}

// GetTree 获取完整权限树
// @Summary      获取完整权限树
// @Description  获取所有权限的完整树形结构
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.Permission} "成功"
// @Router       /permission/tree [get]
func (pc *PermissionController) GetTree(c *gin.Context) {
	var allPermissions []models.Permission
	if err := config.DB.Order("`order`").Find(&allPermissions).Error; err != nil {
		log.Printf("GetTree find error: %v", err)
	}

	tree := pc.buildAllowedTree(allPermissions, nil, nil)
	respondOK(c, tree)
}

// GetButtonsByParentID 获取子按钮权限
// @Summary      获取子按钮权限
// @Description  获取指定父级下的所有BUTTON类型权限
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Param        parentId path     int true "父级权限ID"
// @Success      200      {object} models.Response{data=[]models.Permission} "成功"
// @Router       /permission/button/{parentId} [get]
func (pc *PermissionController) GetButtonsByParentID(c *gin.Context) {
	parentID := c.Param("parentId")

	var buttons []models.Permission
	if err := config.DB.Where("parent_id = ? AND type = ?", parentID, "BUTTON").Order("`order`").Find(&buttons).Error; err != nil {
		log.Printf("GetButtonsByParentID find error: %v", err)
	}

	respondOK(c, buttons)
}

// Create 创建权限
// @Summary      创建权限
// @Description  创建新的权限节点（菜单、按钮等）
// @Tags         权限管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     createPermissionRequest true "权限信息"
// @Success      200  {object} models.Response{data=models.Permission} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /permission [post]
func (pc *PermissionController) Create(c *gin.Context) {
	var req createPermissionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
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
		respondInternal(c, "Failed to create permission")
		return
	}

	respondOK(c, permission)
}

// Update 更新权限
// @Summary      更新权限
// @Description  更新指定权限节点的信息
// @Tags         权限管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                    true "权限ID"
// @Param        body body     updatePermissionRequest true "权限信息"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /permission/{id} [patch]
func (pc *PermissionController) Update(c *gin.Context) {
	id := c.Param("id")
	var req updatePermissionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
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
		respondBadRequest(c, "No fields to update")
		return
	}

	if err := config.DB.Model(&models.Permission{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		respondInternal(c, "Failed to update permission")
		return
	}

	respondOK(c, true)
}

// Delete 删除权限
// @Summary      删除权限
// @Description  删除指定权限节点（需先删除子权限）
// @Tags         权限管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "权限ID"
// @Success      200 {object} models.Response "成功"
// @Failure      400 {object} models.Response "存在子权限，无法删除"
// @Router       /permission/{id} [delete]
func (pc *PermissionController) Delete(c *gin.Context) {
	id := c.Param("id")

	var childCount int64
	config.DB.Model(&models.Permission{}).Where("parent_id = ?", id).Count(&childCount)
	if childCount > 0 {
		respondBadRequest(c, "Please delete child permissions first")
		return
	}

	// Resolve many-to-many foreign key constraint before deleting the permission
	if err := config.DB.Exec("DELETE FROM role_permissions WHERE permission_id = ?", id).Error; err != nil {
		respondInternal(c, "Failed to clear permission associations")
		return
	}

	if err := config.DB.Delete(&models.Permission{}, id).Error; err != nil {
		respondInternal(c, "Failed to delete permission")
		return
	}

	respondOK(c, true)
}

func (pc *PermissionController) buildAllowedTree(all []models.Permission, parentId *uint, allowed map[uint]bool) []models.Permission {
	var res []models.Permission
	for _, p := range all {
		if p.Enable != nil && !*p.Enable {
			continue
		}
		if (parentId == nil && p.ParentID == nil) || (parentId != nil && p.ParentID != nil && *parentId == *p.ParentID) {
			children := pc.buildAllowedTree(all, &p.ID, allowed)
			if len(children) > 0 {
				p.Children = children
			}
			// 节点自身被允许，或有被允许的子节点，则包含该节点
			if allowed == nil || allowed[p.ID] || len(children) > 0 {
				res = append(res, p)
			}
		}
	}
	return res
}
