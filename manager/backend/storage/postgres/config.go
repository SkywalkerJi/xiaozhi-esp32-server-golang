package postgres

import (
	"fmt"
	"xiaozhi/manager/backend/config"
)

// Config PostgreSQL配置
type Config struct {
	Host            string `json:"host"`
	Port            string `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Database        string `json:"database"`
	SSLMode         string `json:"ssl_mode"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	MaxOpenConns    int    `json:"max_open_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
}

// NewConfigFromDatabase 从数据库配置创建PostgreSQL配置
func NewConfigFromDatabase(dbConfig config.DatabaseConfig) *Config {
	sslMode := dbConfig.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return &Config{
		Host:            dbConfig.Host,
		Port:            dbConfig.Port,
		Username:        dbConfig.Username,
		Password:        dbConfig.Password,
		Database:        dbConfig.Database,
		SSLMode:         sslMode,
		MaxIdleConns:    10,
		MaxOpenConns:    100,
		ConnMaxLifetime: 3600,
	}
}

// DSN 生成数据源名称
func (c *Config) DSN() string {
	return fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Shanghai",
		c.Host, c.Username, c.Password, c.Database, c.Port, c.SSLMode)
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("PostgreSQL host is required")
	}
	if c.Port == "" {
		return fmt.Errorf("PostgreSQL port is required")
	}
	if c.Username == "" {
		return fmt.Errorf("PostgreSQL username is required")
	}
	if c.Database == "" {
		return fmt.Errorf("PostgreSQL database name is required")
	}
	return nil
}

// ValidateConfig 验证PostgreSQL配置
func ValidateConfig(config config.DatabaseConfig) error {
	if config.Host == "" {
		return fmt.Errorf("PostgreSQL host is required")
	}
	if config.Port == "" {
		return fmt.Errorf("PostgreSQL port is required")
	}
	if config.Username == "" {
		return fmt.Errorf("PostgreSQL username is required")
	}
	if config.Database == "" {
		return fmt.Errorf("PostgreSQL database name is required")
	}
	return nil
}
