package main

import (
	"backend/config"
	"backend/models"
	"backend/routes"
	"backend/scheduler"
	"log"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.LoadConfig()

	if err := config.InitDB(cfg.Database); err != nil {
		log.Fatal("init db:", err)
	}
	if err := config.AutoMigrate(); err != nil {
		log.Fatal("migrate:", err)
	}
	if err := config.Seed(); err != nil {
		log.Fatal("seed:", err)
	}
	if err := scheduler.Init(); err != nil {
		log.Fatal("scheduler:", err)
	}
	defer scheduler.Stop()

	r := gin.Default()

	// 媒体库本地存储静态目录：从默认 LOCAL 配置读取
	mountLocalStorage(r)

	routes.SetupRoutes(r)

	log.Printf("Server starting on port %s", cfg.Server.Port)
	if err := r.Run(cfg.Server.Port); err != nil {
		log.Fatal("start server:", err)
	}
}

// mountLocalStorage 将所有启用的 LOCAL 存储配置挂载为 Gin 静态目录。
// 同一前缀只挂载一次。
func mountLocalStorage(r *gin.Engine) {
	var cfgs []models.StorageConfig
	if err := config.DB.Where("type = ? AND enabled = ?", "LOCAL", true).Find(&cfgs).Error; err != nil {
		log.Printf("mount local storage: %v", err)
		return
	}
	mounted := make(map[string]struct{})
	for _, cfg := range cfgs {
		dir := cfg.LocalDir
		if dir == "" {
			dir = "./uploads"
		}
		prefix := cfg.PublicPrefix
		if prefix == "" {
			prefix = "/uploads"
		}
		if _, ok := mounted[prefix]; ok {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("ensure local dir %s: %v", dir, err)
			continue
		}
		r.Static(prefix, dir)
		mounted[prefix] = struct{}{}
		log.Printf("Mounted local storage: %s -> %s", prefix, dir)
	}
}
