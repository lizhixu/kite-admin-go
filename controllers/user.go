package controllers

import (
	"backend/config"
	"backend/models"
	"backend/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserController struct{}

func (uc *UserController) GetDetail(c *gin.Context) {
	userID := c.GetUint("userID")

	var user models.User
	if err := config.DB.Preload("Profile").Preload("Roles").First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "User not found",
			OriginUrl: c.Request.URL.Path,
		})
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

	c.JSON(http.StatusOK, models.Response{
		Code:    0,
		Message: "OK",
		Data: gin.H{
			"id":          user.ID,
			"username":    user.Username,
			"enable":      user.Enable,
			"createTime":  user.CreateTime,
			"updateTime":  user.UpdateTime,
			"profile":     user.Profile,
			"roles":       user.Roles,
			"currentRole": currentRole,
		},
		OriginUrl: c.Request.URL.Path,
	})
}

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

	query := config.DB.Model(&models.User{}).Preload("Roles").Preload("Profile")
	if username != "" {
		query = query.Where("username LIKE ?", "%"+username+"%")
	}

	var total int64
	query.Count(&total)

	var users []models.User
	offset := (pageNo - 1) * pageSize
	query.Offset(offset).Limit(pageSize).Find(&users)

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

func (uc *UserController) Create(c *gin.Context) {
	var req struct {
		Username string  `json:"username" binding:"required"`
		Password string  `json:"password" binding:"required"`
		Enable   *bool   `json:"enable"`
		RoleIds  []uint  `json:"roleIds"`
		NickName *string `json:"nickName"`
		Gender   *string `json:"gender"`
		Avatar   *string `json:"avatar"`
		Address  *string `json:"address"`
		Email    *string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to hash password",
			OriginUrl: c.Request.URL.Path,
		})
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
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to create user",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 创建用户资料
	if req.NickName != nil || req.Gender != nil || req.Avatar != nil || req.Address != nil || req.Email != nil {
		profile := models.Profile{UserID: user.ID}
		if req.NickName != nil {
			profile.NickName = *req.NickName
		}
		if req.Gender != nil {
			profile.Gender = req.Gender
		}
		if req.Avatar != nil {
			profile.Avatar = *req.Avatar
		}
		if req.Address != nil {
			profile.Address = req.Address
		}
		if req.Email != nil {
			profile.Email = req.Email
		}
		config.DB.Create(&profile)
	}

	// 分配角色
	if len(req.RoleIds) > 0 {
		var roles []models.Role
		if err := config.DB.Where("id IN ?", req.RoleIds).Find(&roles).Error; err == nil {
			config.DB.Model(&user).Association("Roles").Append(&roles)
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Code:      0,
		Message:   "OK",
		Data:      user,
		OriginUrl: c.Request.URL.Path,
	})
}

func (uc *UserController) Update(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Username string  `json:"username"`
		Enable   *bool   `json:"enable"`
		RoleIds  []uint  `json:"roleIds"`
		NickName *string `json:"nickName"`
		Gender   *string `json:"gender"`
		Avatar   *string `json:"avatar"`
		Address  *string `json:"address"`
		Email    *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	// 查询用户
	var user models.User
	if err := config.DB.Preload("Profile").First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, models.Response{
			Code:      404,
			Message:   "User not found",
			OriginUrl: c.Request.URL.Path,
		})
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
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to update user",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
	}

	// 更新用户资料
	if req.NickName != nil || req.Gender != nil || req.Avatar != nil || req.Address != nil || req.Email != nil {
		profileUpdates := make(map[string]interface{})
		if req.NickName != nil {
			profileUpdates["nick_name"] = *req.NickName
		}
		if req.Gender != nil {
			profileUpdates["gender"] = *req.Gender
		}
		if req.Avatar != nil {
			profileUpdates["avatar"] = *req.Avatar
		}
		if req.Address != nil {
			profileUpdates["address"] = *req.Address
		}
		if req.Email != nil {
			profileUpdates["email"] = *req.Email
		}

		if user.Profile != nil {
			// 更新现有资料
			config.DB.Model(&user.Profile).Updates(profileUpdates)
		} else {
			// 创建新资料
			profile := models.Profile{UserID: user.ID}
			if req.NickName != nil {
				profile.NickName = *req.NickName
			}
			if req.Gender != nil {
				profile.Gender = req.Gender
			}
			if req.Avatar != nil {
				profile.Avatar = *req.Avatar
			}
			if req.Address != nil {
				profile.Address = req.Address
			}
			if req.Email != nil {
				profile.Email = req.Email
			}
			config.DB.Create(&profile)
		}
	}

	// 更新角色关联
	if req.RoleIds != nil {
		var roles []models.Role
		if err := config.DB.Where("id IN ?", req.RoleIds).Find(&roles).Error; err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to query roles",
				OriginUrl: c.Request.URL.Path,
			})
			return
		}
		if err := config.DB.Model(&user).Association("Roles").Replace(&roles); err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Code:      500,
				Message:   "Failed to update user roles",
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

func (uc *UserController) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := config.DB.Delete(&models.User{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to delete user",
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

func (uc *UserController) ResetPassword(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Code:      400,
			Message:   err.Error(),
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to hash password",
			OriginUrl: c.Request.URL.Path,
		})
		return
	}

	if err := config.DB.Model(&models.User{}).Where("id = ?", id).Update("password", hashedPassword).Error; err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Code:      500,
			Message:   "Failed to reset password",
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
