package main

import (
	"backend/config"
	"backend/models"
	"backend/routes"
	"backend/utils"
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	cfg := config.LoadConfig()

	// 初始化数据库
	if err := config.InitDB(cfg.Database); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// 自动迁移数据库表
	if err := config.DB.AutoMigrate(
		&models.User{},
		&models.Profile{},
		&models.Role{},
		&models.Permission{},
		&models.SysLog{},
	); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// 初始化默认数据
	initDefaultData()

	// 创建 Gin 引擎
	r := gin.Default()

	// 设置路由
	routes.SetupRoutes(r)

	// 启动服务器
	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := r.Run(cfg.Server.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func ptrStr(s string) *string { return &s }
func ptrUint(i uint) *uint    { return &i }
func ptrBool(b bool) *bool    { return &b }

func initDefaultData() {
	// 检查是否已有管理员用户
	var count int64
	config.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		return
	}

	// 创建默认权限
	permissions := []models.Permission{
		// 系统管理
		{ID: 2, Name: "系统管理", Code: "SysMgt", Type: "MENU", Icon: ptrStr("i-fe:grid"), Show: ptrBool(true), Enable: ptrBool(true), Order: 1},

		// 日志管理（归属于系统管理）
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

		// 个人资料（不在菜单显示）
		{ID: 8, Name: "个人资料", Code: "UserProfile", Type: "MENU", Path: ptrStr("/profile"), Icon: ptrStr("i-fe:user"), Component: ptrStr("/src/views/profile/index.vue"), Show: ptrBool(false), Enable: ptrBool(true), Order: 99},
	}
	for i := range permissions {
		config.DB.Create(&permissions[i])
	}

	// 创建默认角色
	superAdminRole := models.Role{
		Code:   "SUPER_ADMIN",
		Name:   "超级管理员",
		Enable: true,
	}
	config.DB.Create(&superAdminRole)

	// 将所有权限赋予超级管理员
	var allPerms []models.Permission
	for i := range permissions {
		allPerms = append(allPerms, permissions[i])
	}
	config.DB.Model(&superAdminRole).Association("Permissions").Append(&allPerms)

	qaRole := models.Role{
		Code:   "ROLE_QA",
		Name:   "质检员",
		Enable: true,
	}
	config.DB.Create(&qaRole)

	// 创建默认管理员用户
	hashedPassword, _ := utils.HashPassword("123456")
	admin := models.User{
		Username: "admin",
		Password: hashedPassword,
		Enable:   true,
	}
	config.DB.Create(&admin)

	// 创建用户资料
	profile := models.Profile{
		UserID:   admin.ID,
		NickName: "Admin",
		Avatar:   "https://wpimg.wallstcn.com/f778738c-e4f8-4870-b634-56703b4acafe.gif?imageView2/1/w/80/h/80",
	}
	config.DB.Create(&profile)

	// 关联角色
	config.DB.Model(&admin).Association("Roles").Append(&superAdminRole, &qaRole)

	log.Println("Default data initialized successfully")
}
