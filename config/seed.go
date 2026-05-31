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

	// 邮件模板：仅首次初始化
	if err := seedEmailTemplatesIfEmpty(); err != nil {
		return fmt.Errorf("seed email templates: %w", err)
	}

	// 系统配置：仅首次初始化
	if err := seedSystemConfigIfEmpty(); err != nil {
		return fmt.Errorf("seed system config: %w", err)
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
	if err := DB.Where("code = ?", models.RoleSuperAdmin).First(&superAdmin).Error; err == nil {
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

		// 系统配置
		{ID: 61, Name: "系统配置", Code: "SystemConfig", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/system/config"), Icon: ptrStr("i-fe:settings"), Component: ptrStr("/src/views/system/config.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 0},
		{ID: 62, Name: "保存系统配置", Code: "SaveSystemConfig", Type: "BUTTON", ParentID: ptrUint(61), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},

		// 日志管理
		{ID: 6, Name: "日志管理", Code: "LogMgt", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/log/list"), Icon: ptrStr("i-fe:file-text"), Component: ptrStr("/src/views/log/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},
		{ID: 60, Name: "登录日志", Code: "LoginLog", Type: "MENU", ParentID: ptrUint(2), Path: ptrStr("/log/login"), Icon: ptrStr("i-fe:log-in"), Component: ptrStr("/src/views/log/login.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 5},

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

		// 消息管理
		{ID: 50, Name: "消息管理", Code: "MsgMgt", Type: "MENU", Icon: ptrStr("i-fe:bell"), Show: ptrBool(true), Enable: ptrBool(true), Order: 4},
		{ID: 51, Name: "消息列表", Code: "MessageList", Type: "MENU", ParentID: ptrUint(50), Path: ptrStr("/message/list"), Icon: ptrStr("i-fe:message-square"), Component: ptrStr("/src/views/message/index.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 52, Name: "发送消息", Code: "SendMessage", Type: "BUTTON", ParentID: ptrUint(51), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 53, Name: "删除消息", Code: "DeleteMessage", Type: "BUTTON", ParentID: ptrUint(51), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 54, Name: "邮件配置", Code: "EmailConfigMgt", Type: "MENU", ParentID: ptrUint(50), Path: ptrStr("/message/email-config"), Icon: ptrStr("i-fe:mail"), Component: ptrStr("/src/views/message/email-config.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 56, Name: "保存配置", Code: "SaveEmailConfig", Type: "BUTTON", ParentID: ptrUint(54), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
		{ID: 57, Name: "发送测试邮件", Code: "TestEmailConfig", Type: "BUTTON", ParentID: ptrUint(54), Show: ptrBool(true), Enable: ptrBool(true), Order: 2},
		{ID: 55, Name: "邮件模板", Code: "EmailTemplateMgt", Type: "MENU", ParentID: ptrUint(50), Path: ptrStr("/message/email-template"), Icon: ptrStr("i-fe:layout"), Component: ptrStr("/src/views/message/email-template.vue"), Show: ptrBool(true), Enable: ptrBool(true), Order: 3},
		{ID: 58, Name: "保存模板", Code: "SaveEmailTemplate", Type: "BUTTON", ParentID: ptrUint(55), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},
	}
}

func seedInitialRoles(tx *gorm.DB) error {
	superAdmin := &models.Role{Code: models.RoleSuperAdmin, Name: "超级管理员", Enable: true}
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

func seedEmailTemplatesIfEmpty() error {
	var count int64
	if err := DB.Model(&models.EmailTemplate{}).Count(&count).Error; err != nil {
		return err
	}

	templates := defaultEmailTemplates()

	if count == 0 {
		// 首次初始化，创建所有模板
		for _, tmpl := range templates {
			if err := DB.Create(&tmpl).Error; err != nil {
				return err
			}
		}
		log.Println("Email templates seeded")
		return nil
	}

	// 已存在模板，同步更新内置模板
	for _, tmpl := range templates {
		var existing models.EmailTemplate
		err := DB.Where("scene = ? AND is_builtin = ?", tmpl.Scene, true).First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			// 不存在则创建
			if err := DB.Create(&tmpl).Error; err != nil {
				return err
			}
			log.Printf("Created builtin email template: %s", tmpl.Scene)
		} else if err == nil {
			// 已存在则更新
			existing.Name = tmpl.Name
			existing.Subject = tmpl.Subject
			existing.Content = tmpl.Content
			if err := DB.Save(&existing).Error; err != nil {
				return err
			}
			log.Printf("Updated builtin email template: %s", tmpl.Scene)
		} else {
			return err
		}
	}

	return nil
}

func defaultEmailTemplates() []models.EmailTemplate {
	return []models.EmailTemplate{
		{
			Scene:     "SYSTEM",
			Name:      "系统消息",
			Subject:   "【系统通知】{{title}}",
			IsBuiltin: true,
			Content: `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>{{title}}</title>
</head>

<body style="margin:0; padding:0; background:#f7f5f2; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Hiragino Sans GB','Microsoft YaHei',Arial,sans-serif; color:#1f2329;">
  <table width="100%" cellpadding="0" cellspacing="0" role="presentation" style="width:100%; background:#f7f5f2; padding:40px 16px;">
    <tr>
      <td align="center">

        <!-- Container -->
        <table width="600" cellpadding="0" cellspacing="0" role="presentation" style="width:600px; max-width:100%; background:#ffffff; border:1px solid #e8e3dc; border-radius:10px; overflow:hidden;">

          <!-- Header -->
          <tr>
            <td style="padding:18px 28px; border-bottom:1px solid #eee8df; background:#ffffff;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation">
                <tr>
                  <td style="vertical-align:middle;">
                    <table cellpadding="0" cellspacing="0" role="presentation">
                      <tr>
                        <td style="vertical-align:middle; padding-right:10px;">
                          <!-- SVG Icon -->
                          <span style="display:inline-block; width:30px; height:30px; border-radius:8px; background:#fff3e0; text-align:center; line-height:30px;">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" style="vertical-align:-3px;" xmlns="http://www.w3.org/2000/svg">
                              <path d="M12 2.75C7.168 2.75 3.25 6.668 3.25 11.5C3.25 14.013 4.312 16.278 6.012 17.875C6.286 18.132 6.45 18.488 6.45 18.864V19.25C6.45 20.355 7.345 21.25 8.45 21.25H15.55C16.655 21.25 17.55 20.355 17.55 19.25V18.864C17.55 18.488 17.714 18.132 17.988 17.875C19.688 16.278 20.75 14.013 20.75 11.5C20.75 6.668 16.832 2.75 12 2.75Z" fill="#F59E0B"/>
                              <path d="M9.25 11.75L11.1 13.6L15.1 9.6" stroke="#FFFFFF" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
                            </svg>
                          </span>
                        </td>
                        <td style="vertical-align:middle;">
                          <div style="font-size:14px; font-weight:600; color:#1f2329; line-height:1.3;">
                            系统通知
                          </div>
                          <div style="font-size:12px; color:#8a8178; line-height:1.5; margin-top:1px;">
                            System Message
                          </div>
                        </td>
                      </tr>
                    </table>
                  </td>

                  <td align="right" style="vertical-align:middle; font-size:12px; color:#9a938b; white-space:nowrap;">
                    {{currentTime}}
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Main -->
          <tr>
            <td style="padding:30px 32px 8px;">
              <div style="display:inline-block; padding:4px 10px; border-radius:999px; background:#fff7ed; color:#d97706; font-size:12px; line-height:1.5; margin-bottom:14px;">
                系统消息
              </div>

              <h1 style="margin:0; padding:0; font-size:20px; font-weight:600; line-height:1.45; color:#1f2329;">
                {{title}}
              </h1>
            </td>
          </tr>

          <!-- Content -->
          <tr>
            <td style="padding:14px 32px 32px;">
              <div style="font-size:14px; line-height:1.85; color:#4e5969;">
                {{content}}
              </div>
            </td>
          </tr>

          <!-- Notice Box -->
          <tr>
            <td style="padding:0 32px 30px;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation" style="background:#fbfaf8; border:1px solid #eee8df; border-radius:8px;">
                <tr>
                  <td style="padding:14px 16px;">
                    <table cellpadding="0" cellspacing="0" role="presentation">
                      <tr>
                        <td style="vertical-align:top; padding-right:10px;">
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" style="margin-top:2px;" xmlns="http://www.w3.org/2000/svg">
                            <path d="M12 21.5C17.247 21.5 21.5 17.247 21.5 12C21.5 6.75329 17.247 2.5 12 2.5C6.75329 2.5 2.5 6.75329 2.5 12C2.5 17.247 6.75329 21.5 12 21.5Z" fill="#D6CEC4"/>
                            <path d="M12 10.75C12.4142 10.75 12.75 11.0858 12.75 11.5V16.25C12.75 16.6642 12.4142 17 12 17C11.5858 17 11.25 16.6642 11.25 16.25V11.5C11.25 11.0858 11.5858 10.75 12 10.75Z" fill="#FFFFFF"/>
                            <path d="M12 7.25C12.5523 7.25 13 7.69772 13 8.25C13 8.80228 12.5523 9.25 12 9.25C11.4477 9.25 11 8.80228 11 8.25C11 7.69772 11.4477 7.25 12 7.25Z" fill="#FFFFFF"/>
                          </svg>
                        </td>
                        <td style="font-size:12px; line-height:1.7; color:#8a8178;">
                          此邮件由系统自动发送，仅用于消息通知。如需处理相关事项，请前往系统查看。
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="padding:18px 32px; border-top:1px solid #eee8df; background:#fbfaf8;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation">
                <tr>
                  <td style="font-size:12px; color:#9a938b; line-height:1.6;">
                    请勿直接回复此邮件
                  </td>
                  <td align="right" style="font-size:12px; color:#b8b0a7; line-height:1.6;">
                    Powered by System
                  </td>
                </tr>
              </table>
            </td>
          </tr>

        </table>

      </td>
    </tr>
  </table>
</body>
</html>`,
		},
		{
			Scene:     "NOTICE",
			Name:      "通知公告",
			Subject:   "【通知公告】{{title}}",
			IsBuiltin: true,
			Content: `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>{{title}}</title>
</head>

<body style="margin:0; padding:0; background:#f6f7f9; font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','PingFang SC','Hiragino Sans GB','Microsoft YaHei',Arial,sans-serif; color:#1f2329;">
  <table width="100%" cellpadding="0" cellspacing="0" role="presentation" style="width:100%; background:#f6f7f9; padding:40px 16px;">
    <tr>
      <td align="center">

        <!-- Container -->
        <table width="600" cellpadding="0" cellspacing="0" role="presentation" style="width:600px; max-width:100%; background:#ffffff; border:1px solid #e5e7eb; border-radius:10px; overflow:hidden;">

          <!-- Top Bar -->
          <tr>
            <td style="padding:18px 28px; border-bottom:1px solid #eef0f3;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation">
                <tr>
                  <td style="vertical-align:middle;">
                    <table cellpadding="0" cellspacing="0" role="presentation">
                      <tr>
                        <td style="vertical-align:middle; padding-right:10px;">
                          <!-- SVG Icon -->
                          <span style="display:inline-block; width:28px; height:28px; border-radius:8px; background:#e8f7ee; text-align:center; line-height:28px;">
                            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" style="vertical-align:-3px;" xmlns="http://www.w3.org/2000/svg">
                              <path d="M12 22C13.1046 22 14 21.1046 14 20H10C10 21.1046 10.8954 22 12 22Z" fill="#18A058"/>
                              <path d="M18 16V11C18 7.68629 15.3137 5 12 5C8.68629 5 6 7.68629 6 11V16L4.70711 17.2929C4.07714 17.9229 4.52331 19 5.41421 19H18.5858C19.4767 19 19.9229 17.9229 19.2929 17.2929L18 16Z" fill="#18A058"/>
                              <path d="M14.5 4.5C14.5 3.11929 13.3807 2 12 2C10.6193 2 9.5 3.11929 9.5 4.5V5.1C10.2749 4.73018 11.1224 4.5 12 4.5C12.8776 4.5 13.7251 4.73018 14.5 5.1V4.5Z" fill="#18A058"/>
                            </svg>
                          </span>
                        </td>
                        <td style="vertical-align:middle;">
                          <div style="font-size:14px; font-weight:600; color:#1f2329; line-height:1.3;">
                            通知公告
                          </div>
                          <div style="font-size:12px; color:#86909c; line-height:1.5; margin-top:1px;">
                            System Notification
                          </div>
                        </td>
                      </tr>
                    </table>
                  </td>

                  <td align="right" style="vertical-align:middle; font-size:12px; color:#86909c; white-space:nowrap;">
                    {{currentTime}}
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Body -->
          <tr>
            <td style="padding:30px 32px 8px;">
              <div style="display:inline-block; padding:4px 10px; border-radius:999px; background:#f0f6ff; color:#2080f0; font-size:12px; line-height:1.5; margin-bottom:14px;">
                新消息
              </div>

              <h1 style="margin:0; padding:0; font-size:20px; font-weight:600; line-height:1.45; color:#1f2329;">
                {{title}}
              </h1>
            </td>
          </tr>

          <tr>
            <td style="padding:14px 32px 32px;">
              <div style="font-size:14px; line-height:1.85; color:#4e5969;">
                {{content}}
              </div>
            </td>
          </tr>

          <!-- Info Box -->
          <tr>
            <td style="padding:0 32px 30px;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation" style="background:#fafafa; border:1px solid #edf0f2; border-radius:8px;">
                <tr>
                  <td style="padding:14px 16px;">
                    <table cellpadding="0" cellspacing="0" role="presentation">
                      <tr>
                        <td style="vertical-align:top; padding-right:10px;">
                          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" style="margin-top:2px;" xmlns="http://www.w3.org/2000/svg">
                            <path d="M12 22C17.5228 22 22 17.5228 22 12C22 6.47715 17.5228 2 12 2C6.47715 2 2 6.47715 2 12C2 17.5228 6.47715 22 12 22Z" fill="#C9CDD4"/>
                            <path d="M12 10.75C12.4142 10.75 12.75 11.0858 12.75 11.5V16.5C12.75 16.9142 12.4142 17.25 12 17.25C11.5858 17.25 11.25 16.9142 11.25 16.5V11.5C11.25 11.0858 11.5858 10.75 12 10.75Z" fill="#FFFFFF"/>
                            <path d="M12 7C12.5523 7 13 7.44772 13 8C13 8.55228 12.5523 9 12 9C11.4477 9 11 8.55228 11 8C11 7.44772 11.4477 7 12 7Z" fill="#FFFFFF"/>
                          </svg>
                        </td>
                        <td style="font-size:12px; line-height:1.7; color:#86909c;">
                          此邮件由系统自动发送，仅用于消息通知。如需处理相关事项，请前往系统查看。
                        </td>
                      </tr>
                    </table>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          <!-- Footer -->
          <tr>
            <td style="padding:18px 32px; border-top:1px solid #eef0f3; background:#fbfbfc;">
              <table width="100%" cellpadding="0" cellspacing="0" role="presentation">
                <tr>
                  <td style="font-size:12px; color:#a0a7b2; line-height:1.6;">
                    请勿直接回复此邮件
                  </td>
                  <td align="right" style="font-size:12px; color:#c0c4cc; line-height:1.6;">
                    Powered by System
                  </td>
                </tr>
              </table>
            </td>
          </tr>

        </table>

      </td>
    </tr>
  </table>
</body>
</html>`,
		},
	}
}

func seedSystemConfigIfEmpty() error {
	var count int64
	if err := DB.Model(&models.SystemConfig{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cfg := models.SystemConfig{
		SiteName:  "Kite Admin",
		Copyright: "© 2024 Kite Admin. All rights reserved.",
	}
	if err := DB.Create(&cfg).Error; err != nil {
		return err
	}
	log.Println("Default system config seeded")
	return nil
}
