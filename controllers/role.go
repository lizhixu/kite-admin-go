package controllers

import (
	"backend/config"
	"backend/middleware"
	"backend/models"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RoleController struct{}

type createRoleRequest struct {
	Name          string `json:"name" binding:"required"`
	Code          string `json:"code" binding:"required"`
	Enable        *bool  `json:"enable"`
	PermissionIds []uint `json:"permissionIds"`
}

type updateRoleRequest struct {
	Name          string `json:"name"`
	Code          string `json:"code"`
	Enable        *bool  `json:"enable"`
	PermissionIds []uint `json:"permissionIds"`
}

type roleUsersRequest struct {
	UserIds []uint `json:"userIds" binding:"required"`
}

// GetPage 分页查询角色
// @Summary      分页查询角色
// @Description  分页查询角色列表，支持按名称搜索
// @Tags         角色管理
// @Security     BearerAuth
// @Produce      json
// @Param        name     query    string false "角色名称（模糊搜索）"
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Success      200      {object} models.Response{data=models.PageData} "成功"
// @Router       /role/page [get]
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
	if pageSize > 100 {
		pageSize = 100
	}

	query := config.DB.Model(&models.Role{}).Preload("Permissions")
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Printf("GetPage count error: %v", err)
	}

	var roles []models.Role
	offset := (pageNo - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&roles).Error; err != nil {
		log.Printf("GetPage find error: %v", err)
	}

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

	respondOK(c, models.PageData{PageData: pageData, Total: total})
}

// GetAll 获取所有启用角色
// @Summary      获取所有启用角色
// @Description  获取所有启用状态的角色列表
// @Tags         角色管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response{data=[]models.Role} "成功"
// @Router       /role [get]
func (rc *RoleController) GetAll(c *gin.Context) {
	var roles []models.Role
	if err := config.DB.Where("enable = ?", true).Find(&roles).Error; err != nil {
		log.Printf("GetAll find error: %v", err)
	}

	respondOK(c, roles)
}

// Create 创建角色
// @Summary      创建角色
// @Description  创建新角色，可同时分配权限
// @Tags         角色管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     createRoleRequest true "角色信息"
// @Success      200  {object} models.Response{data=models.Role} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /role [post]
func (rc *RoleController) Create(c *gin.Context) {
	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	role := models.Role{Name: req.Name, Code: req.Code}
	if req.Enable != nil {
		role.Enable = *req.Enable
	} else {
		role.Enable = true
	}

	if err := config.DB.Create(&role).Error; err != nil {
		respondInternal(c, "Failed to create role")
		return
	}

	// 分配权限
	if len(req.PermissionIds) > 0 {
		var permissions []models.Permission
		if err := config.DB.Where("id IN ?", req.PermissionIds).Find(&permissions).Error; err == nil {
			if err := config.DB.Model(&role).Association("Permissions").Append(&permissions); err != nil {
				log.Printf("Create role assign permissions error: %v", err)
			}
		} else {
			log.Printf("Create role query permissions error: %v", err)
		}
	}
	middleware.InvalidatePermCache(role.Code)

	respondOK(c, role)
}

// Update 更新角色
// @Summary      更新角色
// @Description  更新角色信息和权限关联
// @Tags         角色管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int               true "角色ID"
// @Param        body body     updateRoleRequest true "角色信息"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "角色不存在"
// @Router       /role/{id} [patch]
func (rc *RoleController) Update(c *gin.Context) {
	id := c.Param("id")
	var req updateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	// 查询角色
	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		respondNotFound(c, "Role not found")
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
			respondInternal(c, "Failed to update role")
			return
		}
	}

	// 更新权限关联
	if req.PermissionIds != nil {
		var permissions []models.Permission
		if err := config.DB.Where("id IN ?", req.PermissionIds).Find(&permissions).Error; err != nil {
			respondInternal(c, "Failed to query permissions")
			return
		}
		if err := config.DB.Model(&role).Association("Permissions").Replace(&permissions); err != nil {
			respondInternal(c, "Failed to update role permissions")
			return
		}
	}
	middleware.InvalidatePermCache(role.Code)

	respondOK(c, true)
}

// Delete 删除角色
// @Summary      删除角色
// @Description  删除指定角色（SUPER_ADMIN角色不可删除）
// @Tags         角色管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "角色ID"
// @Success      200 {object} models.Response "成功"
// @Failure      400 {object} models.Response "不可删除超级管理员角色"
// @Failure      404 {object} models.Response "角色不存在"
// @Router       /role/{id} [delete]
func (rc *RoleController) Delete(c *gin.Context) {
	id := c.Param("id")

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		respondNotFound(c, "Role not found")
		return
	}

	if role.Code == models.RoleSuperAdmin {
		respondBadRequest(c, "Super Admin role cannot be deleted")
		return
	}

	// 1. Clear many-to-many associations first to resolve foreign key constraints
	if err := config.DB.Model(&role).Association("Permissions").Clear(); err != nil {
		log.Printf("Delete role clear permissions error: %v", err)
	}
	if err := config.DB.Model(&role).Association("Users").Clear(); err != nil {
		log.Printf("Delete role clear users error: %v", err)
	}

	// 2. Delete the role
	if err := config.DB.Delete(&role).Error; err != nil {
		respondInternal(c, "Failed to delete role")
		return
	}
	middleware.InvalidatePermCache(role.Code)

	respondOK(c, true)
}

// AddUsers 添加角色用户
// @Summary      添加角色用户
// @Description  向指定角色添加用户
// @Tags         角色管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int              true "角色ID"
// @Param        body body     roleUsersRequest true "用户ID列表"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "角色不存在"
// @Router       /role/users/add/{id} [patch]
func (rc *RoleController) AddUsers(c *gin.Context) {
	id := c.Param("id")
	var req roleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		respondNotFound(c, "Role not found")
		return
	}

	var users []models.User
	if err := config.DB.Where("id IN ?", req.UserIds).Find(&users).Error; err != nil {
		respondInternal(c, "Failed to query users")
		return
	}

	if err := config.DB.Model(&role).Association("Users").Append(&users); err != nil {
		respondInternal(c, "Failed to add users to role")
		return
	}

	respondOK(c, true)
}

// RemoveUsers 移除角色用户
// @Summary      移除角色用户
// @Description  从指定角色移除用户
// @Tags         角色管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int              true "角色ID"
// @Param        body body     roleUsersRequest true "用户ID列表"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "角色不存在"
// @Router       /role/users/remove/{id} [patch]
func (rc *RoleController) RemoveUsers(c *gin.Context) {
	id := c.Param("id")
	var req roleUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var role models.Role
	if err := config.DB.First(&role, id).Error; err != nil {
		respondNotFound(c, "Role not found")
		return
	}

	var users []models.User
	if err := config.DB.Where("id IN ?", req.UserIds).Find(&users).Error; err != nil {
		respondInternal(c, "Failed to query users")
		return
	}

	if err := config.DB.Model(&role).Association("Users").Delete(&users); err != nil {
		respondInternal(c, "Failed to remove users from role")
		return
	}

	respondOK(c, true)
}
