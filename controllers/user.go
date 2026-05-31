package controllers

import (
	"backend/config"
	"backend/models"
	"backend/utils"
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserController struct{}

type createUserRequest struct {
	Username string  `json:"username" binding:"required"`
	Password string  `json:"password" binding:"required"`
	Enable   *bool   `json:"enable"`
	RoleIds  []uint  `json:"roleIds"`
	NickName *string `json:"nickName"`
	Gender   *int    `json:"gender"`
	Avatar   *string `json:"avatar"`
	Address  *string `json:"address"`
	Email    *string `json:"email"`
}

type updateUserRequest struct {
	Username string  `json:"username"`
	Enable   *bool   `json:"enable"`
	RoleIds  []uint  `json:"roleIds"`
	NickName *string `json:"nickName"`
	Gender   *int    `json:"gender"`
	Avatar   *string `json:"avatar"`
	Address  *string `json:"address"`
	Email    *string `json:"email"`
}

type updateProfileRequest struct {
	NickName *string `json:"nickName"`
	Gender   *int    `json:"gender"`
	Avatar   *string `json:"avatar"`
	Address  *string `json:"address"`
	Email    *string `json:"email"`
}

type resetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

// GetDetail 获取当前用户详情
// @Summary      获取当前用户详情
// @Description  获取当前登录用户的详细信息，包括资料和角色
// @Tags         用户管理
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} models.Response "成功"
// @Failure      404 {object} models.Response "用户不存在"
// @Router       /user/detail [get]
func (uc *UserController) GetDetail(c *gin.Context) {
	userID := c.GetUint("userID")

	var user models.User
	if err := config.DB.Preload("Profile").Preload("Roles").First(&user, userID).Error; err != nil {
		respondNotFound(c, "User not found")
		return
	}

	roleCode := c.GetString("roleCode")
	var currentRole *models.Role
	for _, role := range user.Roles {
		if role.Code == roleCode {
			currentRole = &role
			break
		}
	}

	respondOK(c, gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"enable":      user.Enable,
		"createTime":  user.CreateTime,
		"updateTime":  user.UpdateTime,
		"profile":     user.Profile,
		"roles":       user.Roles,
		"currentRole": currentRole,
	})
}

// GetList 获取用户列表
// @Summary      获取用户列表
// @Description  分页查询用户列表，支持按用户名搜索
// @Tags         用户管理
// @Security     BearerAuth
// @Produce      json
// @Param        username query    string false "用户名（模糊搜索）"
// @Param        pageNo   query    int    false "页码"     default(1)
// @Param        pageSize query    int    false "每页数量" default(10)
// @Success      200      {object} models.Response{data=models.PageData} "成功"
// @Router       /user [get]
func (uc *UserController) GetList(c *gin.Context) {
	username := c.Query("username")
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

	query := config.DB.Model(&models.User{}).Preload("Roles").Preload("Profile")
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Printf("GetList count error: %v", err)
	}

	var users []models.User
	offset := (pageNo - 1) * pageSize
	if err := query.Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		log.Printf("GetList find error: %v", err)
	}

	var pageData []gin.H
	for _, user := range users {
		userData := gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"enable":     user.Enable,
			"createTime": user.CreateTime,
			"updateTime": user.UpdateTime,
			"roles":      user.Roles,
		}
		if user.Profile != nil {
			userData["gender"] = user.Profile.Gender
			userData["avatar"] = user.Profile.Avatar
			userData["address"] = user.Profile.Address
			userData["email"] = user.Profile.Email
		}
		pageData = append(pageData, userData)
	}

	respondOK(c, models.PageData{PageData: pageData, Total: total})
}

// Create 创建用户
// @Summary      创建用户
// @Description  创建新用户，可同时设置资料和分配角色
// @Tags         用户管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body body     createUserRequest true "用户信息"
// @Success      200  {object} models.Response{data=models.User} "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /user [post]
func (uc *UserController) Create(c *gin.Context) {
	var req createUserRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		respondInternal(c, "Failed to hash password")
		return
	}

	user := models.User{
		Username: req.Username,
		Password: hashedPassword,
	}
	if req.Enable != nil {
		user.Enable = *req.Enable
	} else {
		user.Enable = true
	}

	if err := config.DB.Create(&user).Error; err != nil {
		respondInternal(c, "Failed to create user")
		return
	}

	// 创建用户资料
	if hasProfileFields(req.NickName, req.Gender, req.Avatar, req.Address, req.Email) {
		profile := buildProfile(user.ID, req.NickName, req.Gender, req.Avatar, req.Address, req.Email)
		if err := config.DB.Create(&profile).Error; err != nil {
			log.Printf("Create user profile error: %v", err)
		}
	}

	// 分配角色
	if len(req.RoleIds) > 0 {
		var roles []models.Role
		if err := config.DB.Where("id IN ?", req.RoleIds).Find(&roles).Error; err == nil {
			if err := config.DB.Model(&user).Association("Roles").Append(&roles); err != nil {
				log.Printf("Create user assign roles error: %v", err)
			}
		} else {
			log.Printf("Create user query roles error: %v", err)
		}
	}

	respondOK(c, user)
}

// Update 更新用户
// @Summary      更新用户
// @Description  更新用户基本信息、资料和角色
// @Tags         用户管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int              true "用户ID"
// @Param        body body     updateUserRequest true "用户信息"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      404  {object} models.Response "用户不存在"
// @Router       /user/{id} [patch]
func (uc *UserController) Update(c *gin.Context) {
	id := c.Param("id")
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	// 查询用户
	var user models.User
	if err := config.DB.Preload("Profile").First(&user, id).Error; err != nil {
		respondNotFound(c, "User not found")
		return
	}

	// 更新用户基本信息
	updates := make(map[string]interface{})
	if req.Username != "" {
		updates["username"] = req.Username
	}
	if req.Enable != nil {
		updates["enable"] = *req.Enable
	}

	if len(updates) > 0 {
		if err := config.DB.Model(&user).Updates(updates).Error; err != nil {
			respondInternal(c, "Failed to update user")
			return
		}
	}

	// 更新用户资料
	if hasProfileFields(req.NickName, req.Gender, req.Avatar, req.Address, req.Email) {
		if err := saveOrCreateProfile(&user, req.NickName, req.Gender, req.Avatar, req.Address, req.Email); err != nil {
			log.Printf("Update user profile error: %v", err)
		}
	}

	// 更新角色关联
	if req.RoleIds != nil {
		var roles []models.Role
		if err := config.DB.Where("id IN ?", req.RoleIds).Find(&roles).Error; err != nil {
			respondInternal(c, "Failed to query roles")
			return
		}
		if err := config.DB.Model(&user).Association("Roles").Replace(&roles); err != nil {
			respondInternal(c, "Failed to update user roles")
			return
		}
	}

	respondOK(c, true)
}

// UpdateProfile 更新个人资料
// @Summary      更新个人资料
// @Description  用户更新自己的个人资料（昵称、头像、性别、地址、邮箱），仅允许更新本人
// @Tags         用户管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                 true "用户ID"
// @Param        body body     updateProfileRequest true "个人资料"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Failure      403  {object} models.Response "无权修改他人资料"
// @Router       /user/profile/{id} [patch]
func (uc *UserController) UpdateProfile(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	currentUserID := c.GetUint("userID")
	if uint(id) != currentUserID {
		respondForbidden(c, "无权修改他人资料")
		return
	}

	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	var user models.User
	if err := config.DB.Preload("Profile").First(&user, currentUserID).Error; err != nil {
		respondNotFound(c, "User not found")
		return
	}

	if err := saveOrCreateProfile(&user, req.NickName, req.Gender, req.Avatar, req.Address, req.Email); err != nil {
		respondInternal(c, "Failed to save profile")
		return
	}

	respondOK(c, true)
}

// Delete 删除用户
// @Summary      删除用户
// @Description  根据ID删除用户
// @Tags         用户管理
// @Security     BearerAuth
// @Produce      json
// @Param        id  path     int true "用户ID"
// @Success      200 {object} models.Response "成功"
// @Failure      500 {object} models.Response "删除失败"
// @Router       /user/{id} [delete]
func (uc *UserController) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := config.DB.Delete(&models.User{}, id).Error; err != nil {
		respondInternal(c, "Failed to delete user")
		return
	}

	respondOK(c, true)
}

// ResetPassword 重置密码
// @Summary      重置密码
// @Description  管理员重置指定用户的密码
// @Tags         用户管理
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id   path     int                 true "用户ID"
// @Param        body body     resetPasswordRequest true "新密码"
// @Success      200  {object} models.Response "成功"
// @Failure      400  {object} models.Response "请求参数错误"
// @Router       /user/password/reset/{id} [patch]
func (uc *UserController) ResetPassword(c *gin.Context) {
	id := c.Param("id")
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBadRequest(c, err.Error())
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		respondInternal(c, "Failed to hash password")
		return
	}

	if err := config.DB.Model(&models.User{}).Where("id = ?", id).Update("password", hashedPassword).Error; err != nil {
		respondInternal(c, "Failed to reset password")
		return
	}

	respondOK(c, true)
}

// buildProfileUpdates 从可选字段构建 profile 更新 map
func buildProfileUpdates(nickName *string, gender *int, avatar *string, address *string, email *string) map[string]interface{} {
	updates := make(map[string]interface{})
	if nickName != nil {
		updates["nick_name"] = *nickName
	}
	if gender != nil {
		updates["gender"] = *gender
	}
	if avatar != nil {
		updates["avatar"] = *avatar
	}
	if address != nil {
		updates["address"] = *address
	}
	if email != nil {
		updates["email"] = *email
	}
	return updates
}

// buildProfile 从可选字段构建 Profile 结构体
func buildProfile(userID uint, nickName *string, gender *int, avatar *string, address *string, email *string) models.Profile {
	p := models.Profile{UserID: userID}
	if nickName != nil {
		p.NickName = *nickName
	}
	if gender != nil {
		p.Gender = gender
	}
	if avatar != nil {
		p.Avatar = *avatar
	}
	if address != nil {
		p.Address = address
	}
	if email != nil {
		p.Email = email
	}
	return p
}

// hasProfileFields 检查是否有任何 profile 字段被设置
func hasProfileFields(nickName *string, gender *int, avatar *string, address *string, email *string) bool {
	return nickName != nil || gender != nil || avatar != nil || address != nil || email != nil
}

// saveOrCreateProfile 保存或创建用户资料（通用逻辑）
func saveOrCreateProfile(user *models.User, nickName *string, gender *int, avatar *string, address *string, email *string) error {
	if user.Profile != nil {
		updates := buildProfileUpdates(nickName, gender, avatar, address, email)
		if len(updates) > 0 {
			return config.DB.Model(&user.Profile).Updates(updates).Error
		}
		return nil
	}
	profile := buildProfile(user.ID, nickName, gender, avatar, address, email)
	return config.DB.Create(&profile).Error
}
