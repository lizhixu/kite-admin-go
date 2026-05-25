package config

import (
	"backend/models"
	"backend/utils"
	"fmt"
	"log"

	"gorm.io/gorm"
)

func ptrStr(s string) *string { return &s }
func ptrUint(i uint) *uint    { return &i }
func ptrBool(b bool) *bool    { return &b }

// Seed 启动时同步数据：权限每次启动都会同步（新增的自动插入，已存在的更新），
// 用户和角色仅在数据库为空时创建一次。
func Seed() error {
	// 权限：每次启动都同步，确保新增或修改的权限配置能自动生效
	if err := syncPermissions(); err != nil {
		return fmt.Errorf("sync permissions: %w", err)
	}

	// 用户/角色：仅首次初始化
	if err := seedInitialIfEmpty(); err != nil {
		return fmt.Errorf("seed initial: %w", err)
	}

	// 存储配置：仅首次初始化（默认本地存储）
	if err := seedDefaultStorageIfEmpty(); err != nil {
		return fmt.Errorf("seed default storage: %w", err)
	}

	return nil
}

// seedInitialIfEmpty 仅在数据库无用户时创建管理员、角色等初始数据
func seedInitialIfEmpty() error {
	var count int64
	if err := DB.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := seedInitialRoles(tx); err != nil {
			return err
		}
		return seedAdminUser(tx)
	})
	if err != nil {
		return err
	}

	log.Println("Initial users and roles created successfully")
	return nil
}

// syncPermissions 按 Code 为唯一键同步权限：不存在则插入，存在则按配置更新关键字段，
// 同时清理代码中已删除的孤儿权限
func syncPermissions() error {
	perms := defaultPermissions()

	codeSet := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		codeSet[p.Code] = struct{}{}
	}

	for _, p := range perms {
		var existing models.Permission
		err := DB.Where("code = ?", p.Code).First(&existing).Error
		if err == nil {
			// 已存在：更新可能变化的字段
			existing.Name = p.Name
			existing.Type = p.Type
			existing.ParentID = p.ParentID
			existing.Path = p.Path
			existing.Icon = p.Icon
			existing.Component = p.Component
			existing.Show = p.Show
			existing.Enable = p.Enable
			existing.Order = p.Order
			existing.KeepAlive = p.KeepAlive
			if err := DB.Save(&existing).Error; err != nil {
				return fmt.Errorf("update permission %s: %w", p.Code, err)
			}
		} else if err == gorm.ErrRecordNotFound {
			// 不存在：直接插入（使用预定义 ID）
			if err := DB.Create(&p).Error; err != nil {
				return fmt.Errorf("create permission %s: %w", p.Code, err)
			}
		} else {
			return fmt.Errorf("query permission %s: %w", p.Code, err)
		}
	}

	// 清理代码里已删除的"孤儿"权限（避免菜单/按钮残留）
	var dbPerms []models.Permission
	if err := DB.Find(&dbPerms).Error; err != nil {
		return fmt.Errorf("scan permissions: %w", err)
	}
	var orphanIDs []uint
	for _, p := range dbPerms {
		if _, ok := codeSet[p.Code]; !ok {
			orphanIDs = append(orphanIDs, p.ID)
		}
	}
	if len(orphanIDs) > 0 {
		if err := DB.Exec("DELETE FROM role_permissions WHERE permission_id IN ?", orphanIDs).Error; err != nil {
			return fmt.Errorf("cleanup role_permissions: %w", err)
		}
		if err := DB.Where("id IN ?", orphanIDs).Delete(&models.Permission{}).Error; err != nil {
			return fmt.Errorf("delete orphan permissions: %w", err)
		}
		log.Printf("Removed %d orphan permission(s): %v", len(orphanIDs), orphanIDs)
	}

	// 将所有权限重新授权给超级管理员
	var superAdmin models.Role
	var allPerms []models.Permission
	if err := DB.Where("code = ?", "SUPER_ADMIN").First(&superAdmin).Error; err == nil {
		DB.Find(&allPerms)
		DB.Model(&superAdmin).Association("Permissions").Replace(&allPerms)
	}

	log.Println("Permissions synced successfully")
	return nil
}

func defaultPermissions() []models.Permission {
	return []models.Permission{
		// 系统管理
		{ID: 2, Name: "系统管理", Code: "SysMgt", Type: "MENU", Icon: ptrStr("i-fe:grid"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},

		// 日志管理
		{ID: 6, Name: "日志管理", Code: "LogMgt", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/log/list"), Icon: ptrStr("i-fe:file-text"), Component: ptrStr("/src/views/log/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},

		// 资源管理
		{ID: 1, Name: "资源管理", Code: "Resource_Mgt", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/pms/resource"), Icon: ptrStr("i-fe:list"), Component: ptrStr("/src/views/pms/resource/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 11, Name: "新增资源", Code: "AddResource", Type: "BUTTON", ParentID: ptrUint(1), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 12, Name: "编辑资源", Code: "EditResource", Type: "BUTTON", ParentID: ptrUint(1), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 13, Name: "删除资源", Code: "DeleteResource", Type: "BUTTON", ParentID: ptrUint(1), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},

		// 角色管理
		{ID: 3, Name: "角色管理", Code: "RoleMgt", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/pms/role"), Icon: ptrStr("i-fe:user-check"), Component: ptrStr("/src/views/pms/role/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 5, Name: "分配用户", Code: "RoleUser", Type: "MENU", ParentID: ptrUint(3), Path: ptrStr("/pms/role/user/:roleId"), Icon: ptrStr("i-fe:user-plus"), Component: ptrStr("/src/views/pms/role/role-user.vue"), Show: ptrBool(false), Enable: ptrBool(true), Order: 1},
		{ID: 14, Name: "新增角色", Code: "AddRole", Type: "BUTTON", ParentID: ptrUint(3), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 15, Name: "编辑角色", Code: "EditRole", Type: "BUTTON", ParentID: ptrUint(3), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 16, Name: "删除角色", Code: "DeleteRole", Type: "BUTTON", ParentID: ptrUint(3), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 17, Name: "分配权限", Code: "AssignPermission", Type: "BUTTON", ParentID: ptrUint(3), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},

		// 用户管理
		{ID: 4, Name: "用户管理", Code: "UserMgt", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/pms/user"), Icon: ptrStr("i-fe:user"), Component: ptrStr("/src/views/pms/user/index.vue"), KeepAlive: ptrBool(true), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 18, Name: "新增用户", Code: "AddUser", Type: "BUTTON", ParentID: ptrUint(4), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 19, Name: "编辑用户", Code: "EditUser", Type: "BUTTON", ParentID: ptrUint(4), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 20, Name: "删除用户", Code: "DeleteUser", Type: "BUTTON", ParentID: ptrUint(4), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 21, Name: "重置密码", Code: "ResetPassword", Type: "BUTTON", ParentID: ptrUint(4), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},

		// 个人资料
		{ID: 8, Name: "个人资料", Code: "UserProfile", Type: "MENU", Path: ptrStr("/profile"), Icon: ptrStr("i-fe:user"), Component: ptrStr("/src/views/profile/index.vue"), Show: ptrBool(false), Enable: ptrBool(true), Order: 99},

		// 任务管理（父容器：定时任务 + 队列管理）
		{ID: 7, Name: "任务管理", Code: "TaskMgt", Type: "MENU", Icon: ptrStr("i-fe:clock"), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		// 子菜单：定时任务
		{ID: 40, Name: "定时任务", Code: "ScheduleTask", Type: "MENU", ParentID: ptrUint(7), Path: ptrStr("/task/schedule"), Icon: ptrStr("i-fe:calendar"), Component: ptrStr("/src/views/task/schedule/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 22, Name: "新增任务", Code: "AddTask", Type: "BUTTON", ParentID: ptrUint(40), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 23, Name: "编辑任务", Code: "EditTask", Type: "BUTTON", ParentID: ptrUint(40), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 24, Name: "删除任务", Code: "DeleteTask", Type: "BUTTON", ParentID: ptrUint(40), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 25, Name: "执行任务", Code: "RunTask", Type: "BUTTON", ParentID: ptrUint(40), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},
		// 子菜单：队列管理
		{ID: 41, Name: "队列管理", Code: "QueueMgt", Type: "MENU", ParentID: ptrUint(7), Path: ptrStr("/task/queue"), Icon: ptrStr("i-fe:layers"), Component: ptrStr("/src/views/task/queue/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 43, Name: "编辑队列", Code: "EditQueue", Type: "BUTTON", ParentID: ptrUint(41), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 44, Name: "删除队列", Code: "DeleteQueue", Type: "BUTTON", ParentID: ptrUint(41), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 45, Name: "投递任务", Code: "AddQueueJob", Type: "BUTTON", ParentID: ptrUint(41), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 46, Name: "Kick任务", Code: "KickQueueJob", Type: "BUTTON", ParentID: ptrUint(41), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},
		{ID: 47, Name: "队列详情", Code: "QueueDetail", Type: "MENU", ParentID: ptrUint(41), Path: ptrStr("/task/queue/:id"), Icon: ptrStr("i-fe:info"), Component: ptrStr("/src/views/task/queue/detail.vue"), Show: ptrBool(false), Enable: ptrBool(true), Order: 5},

		// 媒体库（父容器）
		{ID: 30, Name: "媒体库", Code: "MediaMgt", Type: "MENU", Icon: ptrStr("i-fe:image"), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		// 子菜单：媒体列表
		{ID: 34, Name: "媒体列表", Code: "MediaList", Type: "MENU", ParentID: ptrUint(30), Path: ptrStr("/media/library"), Icon: ptrStr("i-fe:folder"), Component: ptrStr("/src/views/media/library/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 31, Name: "上传文件", Code: "UploadMedia", Type: "BUTTON", ParentID: ptrUint(34), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 32, Name: "删除文件", Code: "DeleteMedia", Type: "BUTTON", ParentID: ptrUint(34), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 36, Name: "管理文件夹", Code: "ManageFolder", Type: "BUTTON", ParentID: ptrUint(34), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 37, Name: "查看全部媒体", Code: "ViewAllMedia", Type: "BUTTON", ParentID: ptrUint(34), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},
		// 子菜单：存储配置
		{ID: 35, Name: "存储配置", Code: "StorageMgt", Type: "MENU", ParentID: ptrUint(30), Path: ptrStr("/media/storage"), Icon: ptrStr("i-fe:hard-drive"), Component: ptrStr("/src/views/media/storage/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 33, Name: "管理存储", Code: "ManageStorage", Type: "BUTTON", ParentID: ptrUint(35), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
	}
}

func seedInitialRoles(tx *gorm.DB) error {
	superAdmin := &models.Role{Code: "SUPER_ADMIN", Name: "超级管理员", Enable: true}
	if err := tx.Create(superAdmin).Error; err != nil {
		return err
	}
	var allPerms []models.Permission
	tx.Find(&allPerms)
	if err := tx.Model(superAdmin).Association("Permissions").Append(allPerms); err != nil {
		return err
	}

	qa := &models.Role{Code: "ROLE_QA", Name: "质检员", Enable: true}
	if err := tx.Create(qa).Error; err != nil {
		return err
	}
	return nil
}

func seedAdminUser(tx *gorm.DB) error {
	hashed, err := utils.HashPassword("123456")
	if err != nil {
		return err
	}

	admin := &models.User{Username: "admin", Password: hashed, Enable: true}
	if err := tx.Create(admin).Error; err != nil {
		return err
	}

	profile := &models.Profile{
		UserID:   admin.ID,
		NickName: "Admin",
		Avatar:   "https://wpimg.wallstcn.com/f778738c-e4f8-4870-b634-56703b4acafe.gif?imageView2/1/w/80/h/80",
	}
	if err := tx.Create(profile).Error; err != nil {
		return err
	}

	// 赋予所有角色
	var allRoles []models.Role
	tx.Find(&allRoles)
	return tx.Model(admin).Association("Roles").Append(allRoles)
}

// seedDefaultStorageIfEmpty 若无任何存储配置，插入一条 LOCAL 默认配置
func seedDefaultStorageIfEmpty() error {
	var count int64
	if err := DB.Model(&models.StorageConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cfg := models.StorageConfig{
		Name:         "本地存储",
		Type:         "LOCAL",
		LocalDir:     "./uploads",
		PublicPrefix: "/uploads",
		MaxSizeMB:    50,
		Enabled:      true,
		IsDefault:    true,
	}
	if err := DB.Create(&cfg).Error; err != nil {
		return err
	}
	log.Println("Default LOCAL storage config seeded")
	return nil
}
