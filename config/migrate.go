package config

import (
	"backend/models"
	"log"
	"os"
)

// RegisteredModels 列出所有需要自动迁移的模型，作为单一事实来源
func RegisteredModels() []any {
	return []any{
		&models.User{},
		&models.Profile{},
		&models.Role{},
		&models.Permission{},
		&models.SysLog{},
		&models.Task{},
		&models.TaskLog{},
		&models.Queue{},
		&models.QueueJob{},
		&models.Media{},
		&models.MediaFolder{},
		&models.StorageConfig{},
		&models.Message{},
		&models.MessageRecipient{},
		&models.EmailConfig{},
		&models.EmailTemplate{},
		&models.LoginLog{},
	}
}

// AutoMigrate 对所有已注册模型执行 GORM 自动迁移。
// 开发期可设置 RESET_DB=1 在迁移前先 DROP 所有表（仅开发环境使用！）
func AutoMigrate() error {
	if os.Getenv("RESET_DB") == "1" {
		log.Println("RESET_DB=1, dropping all tables before migrate")
		if err := DB.Migrator().DropTable(RegisteredModels()...); err != nil {
			return err
		}
		// 中间表
		_ = DB.Exec("DROP TABLE IF EXISTS role_permissions").Error
		_ = DB.Exec("DROP TABLE IF EXISTS user_roles").Error
	}
	return DB.AutoMigrate(RegisteredModels()...)
}
