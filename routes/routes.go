package routes

import (
	"backend/controllers"
	"backend/middleware"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRoutes(r *gin.Engine) {
	r.Use(middleware.CORSMiddleware())

	// Swagger UI（生产环境需要认证）
	swaggerHandler := ginSwagger.WrapHandler(swaggerFiles.Handler)
	if os.Getenv("APP_ENV") == "production" {
		r.GET("/swagger/*any", middleware.AuthMiddleware(), swaggerHandler)
	} else {
		r.GET("/swagger/*any", swaggerHandler)
	}

	authCtrl := &controllers.AuthController{}
	userCtrl := &controllers.UserController{}
	roleCtrl := &controllers.RoleController{}
	permCtrl := &controllers.PermissionController{}
	syslogCtrl := &controllers.SysLogController{}
	loginLogCtrl := &controllers.LoginLogController{}
	sysConfigCtrl := &controllers.SystemConfigController{}
	taskCtrl := &controllers.TaskController{}
	queueCtrl := &controllers.QueueController{}
	mediaCtrl := &controllers.MediaController{}
	msgCtrl := &controllers.MessageController{}
	emailCtrl := &controllers.EmailConfigController{}
	tmplCtrl := &controllers.EmailTemplateController{}

	// 认证相关路由（无需认证）
	auth := r.Group("/auth")
	{
		auth.POST("/login", middleware.RateLimit(5, time.Minute), authCtrl.Login)
		auth.GET("/captcha", authCtrl.GetCaptcha)
		auth.POST("/logout", authCtrl.Logout)
		auth.GET("/system/config", sysConfigCtrl.Get)
	}

	// 需要认证的路由
	api := r.Group("")
	api.Use(middleware.AuthMiddleware())
	api.Use(middleware.OperationLog())
	{
		// 认证相关
		api.POST("/auth/current-role/switch/:roleCode", authCtrl.SwitchRole)

		// 用户相关
		api.GET("/user/detail", userCtrl.GetDetail)
		api.GET("/user", userCtrl.GetList)
		api.POST("/user", middleware.RequirePermission("AddUser"), userCtrl.Create)
		api.DELETE("/user/:id", middleware.RequirePermission("DeleteUser"), userCtrl.Delete)
		api.PATCH("/user/profile/:id", userCtrl.UpdateProfile)
		api.PATCH("/user/:id", middleware.RequirePermission("EditUser"), userCtrl.Update)
		api.PATCH("/user/password/reset/:id", middleware.RequirePermission("ResetPassword"), userCtrl.ResetPassword)

		// 角色相关
		api.GET("/role/page", middleware.RequirePermission("RoleMgt"), roleCtrl.GetPage)
		api.GET("/role", roleCtrl.GetAll)
		api.POST("/role", middleware.RequirePermission("AddRole"), roleCtrl.Create)
		api.PATCH("/role/:id", middleware.RequirePermission("EditRole"), roleCtrl.Update)
		api.DELETE("/role/:id", middleware.RequirePermission("DeleteRole"), roleCtrl.Delete)
		api.PATCH("/role/users/add/:id", middleware.RequirePermission("AssignPermission"), roleCtrl.AddUsers)
		api.PATCH("/role/users/remove/:id", middleware.RequirePermission("AssignPermission"), roleCtrl.RemoveUsers)

		// 权限相关
		api.GET("/role/permissions/tree", permCtrl.GetRolePermissionsTree)
		api.GET("/permission/menu/tree", permCtrl.GetMenuTree)
		api.GET("/permission/tree", permCtrl.GetTree)
		api.GET("/permission/button/:parentId", permCtrl.GetButtonsByParentID)
		api.POST("/permission", middleware.RequirePermission("AddResource"), permCtrl.Create)
		api.PATCH("/permission/:id", middleware.RequirePermission("EditResource"), permCtrl.Update)
		api.DELETE("/permission/:id", middleware.RequirePermission("DeleteResource"), permCtrl.Delete)

		// 日志相关
		api.GET("/syslog/list", middleware.RequirePermission("LogMgt"), syslogCtrl.GetLogs)
		api.GET("/loginlog/list", middleware.RequirePermission("LoginLog"), loginLogCtrl.GetLogs)

		// 系统配置
		api.GET("/system/config", sysConfigCtrl.Get)
		api.PUT("/system/config", middleware.RequirePermission("SaveSystemConfig"), sysConfigCtrl.Save)

		// 定时任务
		api.GET("/task/page", middleware.RequirePermission("ScheduleTask"), taskCtrl.GetPage)
		api.GET("/task/funcs", middleware.RequirePermission("ScheduleTask"), taskCtrl.GetFuncs)
		api.GET("/task/stats", middleware.RequirePermission("ScheduleTask"), taskCtrl.Stats)
		api.GET("/task/preview-next", middleware.RequirePermission("ScheduleTask"), taskCtrl.PreviewNext)
		api.POST("/task", middleware.RequirePermission("AddTask"), taskCtrl.Create)
		api.PATCH("/task/:id", middleware.RequirePermission("EditTask"), taskCtrl.Update)
		api.DELETE("/task/:id", middleware.RequirePermission("DeleteTask"), taskCtrl.Delete)
		api.PATCH("/task/:id/toggle", middleware.RequirePermission("EditTask"), taskCtrl.Toggle)
		api.POST("/task/:id/run", middleware.RequirePermission("RunTask"), taskCtrl.Run)
		api.POST("/task/bulk/delete", middleware.RequirePermission("DeleteTask"), taskCtrl.BulkDelete)
		api.POST("/task/bulk/toggle", middleware.RequirePermission("EditTask"), taskCtrl.BulkToggle)
		api.GET("/task/log/page", middleware.RequirePermission("ScheduleTask"), taskCtrl.GetLogs)

		// 队列管理
		api.GET("/queue/page", middleware.RequirePermission("QueueMgt"), queueCtrl.GetPage)
		api.GET("/queue/stats", middleware.RequirePermission("QueueMgt"), queueCtrl.Stats)
		api.GET("/queue/handlers", middleware.RequirePermission("QueueMgt"), queueCtrl.GetHandlers)
		api.GET("/queue/:id", middleware.RequirePermission("QueueDetail"), queueCtrl.GetOne)
		api.PATCH("/queue/:id", middleware.RequirePermission("EditQueue"), queueCtrl.Update)
		api.DELETE("/queue/:id", middleware.RequirePermission("DeleteQueue"), queueCtrl.Delete)
		api.PATCH("/queue/:id/toggle", middleware.RequirePermission("EditQueue"), queueCtrl.Toggle)
		api.POST("/queue/:id/kick", middleware.RequirePermission("KickQueueJob"), queueCtrl.KickAll)
		api.GET("/queue/:id/jobs", middleware.RequirePermission("QueueDetail"), queueCtrl.GetJobs)
		api.POST("/queue/:id/job", middleware.RequirePermission("AddQueueJob"), queueCtrl.AddJob)
		api.POST("/queue/:id/jobs/bulk", middleware.RequirePermission("AddQueueJob"), queueCtrl.BulkAddJobs)
		api.DELETE("/queue/:id/jobs", middleware.RequirePermission("DeleteQueue"), queueCtrl.ClearJobs)
		api.POST("/queue/job/:jobId/kick", middleware.RequirePermission("KickQueueJob"), queueCtrl.KickJob)
		api.DELETE("/queue/job/:jobId", middleware.RequirePermission("DeleteQueue"), queueCtrl.DeleteJob)

		// 媒体库
		api.POST("/media/upload", middleware.RateLimit(30, time.Minute), middleware.RequirePermission("UploadMedia"), mediaCtrl.Upload)
		api.GET("/media/page", middleware.RequirePermission("MediaList"), mediaCtrl.GetPage)
		api.DELETE("/media/:id", middleware.RequirePermission("DeleteMedia"), mediaCtrl.Delete)
		api.POST("/media/bulk/delete", middleware.RequirePermission("DeleteMedia"), mediaCtrl.BulkDelete)
		api.POST("/media/move", middleware.RequirePermission("UploadMedia"), mediaCtrl.MoveMedia)

		// 媒体文件夹
		api.GET("/media/folder/tree", mediaCtrl.ListFolders)
		api.GET("/media/folder/resolve", mediaCtrl.ResolveFolder)
		api.POST("/media/folder", middleware.RequirePermission("ManageFolder"), mediaCtrl.CreateFolder)
		api.PATCH("/media/folder/:id", middleware.RequirePermission("ManageFolder"), mediaCtrl.RenameFolder)
		api.DELETE("/media/folder/:id", middleware.RequirePermission("ManageFolder"), mediaCtrl.DeleteFolder)

		// 存储配置
		api.GET("/storage/config", middleware.RequirePermission("ManageStorage"), mediaCtrl.ListConfigs)
		api.POST("/storage/config", middleware.RequirePermission("ManageStorage"), mediaCtrl.CreateConfig)
		api.PATCH("/storage/config/:id", middleware.RequirePermission("ManageStorage"), mediaCtrl.UpdateConfig)
		api.DELETE("/storage/config/:id", middleware.RequirePermission("ManageStorage"), mediaCtrl.DeleteConfig)
		api.PATCH("/storage/config/:id/default", middleware.RequirePermission("ManageStorage"), mediaCtrl.SetDefault)
		api.POST("/storage/config/:id/test", middleware.RequirePermission("ManageStorage"), mediaCtrl.TestConfig)

		// 消息管理
		api.GET("/message/page", middleware.RequirePermission("MessageList"), msgCtrl.GetPage)
		api.POST("/message", middleware.RequirePermission("SendMessage"), msgCtrl.Create)
		api.DELETE("/message/:id", middleware.RequirePermission("DeleteMessage"), msgCtrl.Delete)
		api.POST("/message/bulk/delete", middleware.RequirePermission("DeleteMessage"), msgCtrl.BulkDelete)
		api.GET("/message/mine", msgCtrl.GetMyMessages)
		api.GET("/message/unread/count", msgCtrl.GetUnreadCount)
		api.PATCH("/message/:id/read", msgCtrl.MarkRead)
		api.PATCH("/message/read/all", msgCtrl.MarkAllRead)
		api.PATCH("/message/bulk/read", msgCtrl.BulkMarkRead)

		// 邮件配置
		api.GET("/email/config", middleware.RequirePermission("EmailConfigMgt"), emailCtrl.Get)
		api.PUT("/email/config", middleware.RequirePermission("SaveEmailConfig"), emailCtrl.Save)
		api.POST("/email/config/test", middleware.RequirePermission("TestEmailConfig"), emailCtrl.Test)

		// 邮件模板
		api.GET("/email-template/list", middleware.RequirePermission("EmailTemplateMgt"), tmplCtrl.GetList)
		api.GET("/email-template/:id", middleware.RequirePermission("EmailTemplateMgt"), tmplCtrl.GetOne)
		api.PUT("/email-template/:id", middleware.RequirePermission("SaveEmailTemplate"), tmplCtrl.Update)
		api.POST("/email-template/:id/preview", middleware.RequirePermission("EmailTemplateMgt"), tmplCtrl.Preview)
	}

	// SSE 路由（仅 AuthMiddleware，跳过 OperationLog 避免缓冲响应）
	sseGroup := r.Group("")
	sseGroup.Use(middleware.AuthMiddleware())
	{
		sseGroup.GET("/message/sse", msgCtrl.SSE)
	}
}
