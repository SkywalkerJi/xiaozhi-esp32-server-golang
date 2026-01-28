package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	JWT      JWTConfig      `json:"jwt"`
}

type ServerConfig struct {
	Port string `json:"port"`
	Mode string `json:"mode"`
}

type DatabaseConfig struct {
	Type     string `json:"type"`     // mysql, postgres, sqlite
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
	SSLMode  string `json:"ssl_mode"` // PostgreSQL SSL 模式: disable, require, verify-ca, verify-full
}

type JWTConfig struct {
	Secret     string `json:"secret"`
	ExpireHour int    `json:"expire_hour"`
}

func Load() *Config {
	return LoadWithPath("config/config.json")
}

func LoadWithPath(configPath string) *Config {
	config := LoadFromFile(configPath)

	// 优先使用环境变量覆盖数据库配置
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		config.Database.Type = dbType
	}
	if host := os.Getenv("DB_HOST"); host != "" {
		config.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		config.Database.Port = port
	}
	if username := os.Getenv("DB_USER"); username != "" {
		config.Database.Username = username
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		config.Database.Password = password
	}
	if database := os.Getenv("DB_NAME"); database != "" {
		config.Database.Database = database
	}
	if sslMode := os.Getenv("DB_SSL_MODE"); sslMode != "" {
		config.Database.SSLMode = sslMode
	}

	fmt.Println("config", config)

	return config
}

func LoadFromFile(configPath string) *Config {
	file, err := os.Open(configPath)
	if err != nil {
		log.Fatalf("无法打开配置文件 %s: %v", configPath, err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		log.Fatalf("解析配置文件失败 %s: %v", configPath, err)
	}

	return &config
}
