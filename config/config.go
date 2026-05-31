package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Server     ServerConfig
	Database   DatabaseConfig
	JWT        JWTConfig
	CORSOrigins []string
}

type ServerConfig struct {
	Port string
}

type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

type JWTConfig struct {
	Secret     string
	ExpireTime time.Duration
}

var (
	cachedConfig *Config
	configOnce   sync.Once
)

// LoadConfig 加载配置（单例，仅首次调用时读取 .env）
func LoadConfig() *Config {
	configOnce.Do(func() {
		// 加载 .env 文件
		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found, using default values")
		}

		// 获取环境变量，如果不存在则使用默认值
		serverPort := getEnv("SERVER_PORT", ":8080")
		dbHost := getEnv("DB_HOST", "localhost")
		dbPort := getEnv("DB_PORT", "3306")
		dbUser := getEnv("DB_USER", "root")
		dbPassword := getEnv("DB_PASSWORD", "123456")
		dbName := getEnv("DB_NAME", "admin_system")
		jwtSecret := getEnv("JWT_SECRET", "your-secret-key")
		jwtExpireHours := getEnvAsInt("JWT_EXPIRE_HOURS", 24)

		// 弱凭据警告
		if jwtSecret == "your-secret-key" {
			log.Println("[WARN] JWT_SECRET is using the default value. Set JWT_SECRET in .env for production!")
		}
		if dbPassword == "123456" {
			log.Println("[WARN] DB_PASSWORD is using the default value. Set DB_PASSWORD in .env for production!")
		}

		// CORS origins: 逗号分隔，默认 *（开发模式）
		corsOriginsStr := getEnv("CORS_ORIGINS", "*")
		var corsOrigins []string
		for _, o := range strings.Split(corsOriginsStr, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				corsOrigins = append(corsOrigins, o)
			}
		}

		cachedConfig = &Config{
			Server: ServerConfig{
				Port: serverPort,
			},
			Database: DatabaseConfig{
				Host:     dbHost,
				Port:     dbPort,
				User:     dbUser,
				Password: dbPassword,
				DBName:   dbName,
			},
			JWT: JWTConfig{
				Secret:     jwtSecret,
				ExpireTime: time.Duration(jwtExpireHours) * time.Hour,
			},
			CORSOrigins: corsOrigins,
		}
	})
	return cachedConfig
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}
