package database

import (
	"fmt"
	"log"
	"xiaozhi/manager/backend/config"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Init(cfg config.DatabaseConfig) *gorm.DB {
	var db *gorm.DB
	var err error

	dbType := cfg.Type
	if dbType == "" {
		// 向后兼容：如果没有设置类型，根据数据库名判断
		if cfg.Database == "sqlite" {
			dbType = "sqlite"
		} else {
			dbType = "mysql"
		}
	}

	switch dbType {
	case "sqlite":
		// SQLite 数据库连接
		log.Println("使用SQLite数据库:", cfg.Host)
		db, err = gorm.Open(sqlite.Open(cfg.Host), &gorm.Config{})

	case "postgres":
		// PostgreSQL 数据库连接
		sslMode := cfg.SSLMode
		if sslMode == "" {
			sslMode = "disable"
		}
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Shanghai",
			cfg.Host, cfg.Username, cfg.Password, cfg.Database, cfg.Port, sslMode)
		log.Println("使用PostgreSQL数据库:", cfg.Host)
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})

	case "mysql":
		fallthrough
	default:
		// MySQL 数据库连接
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
		log.Println("使用MySQL数据库:", cfg.Host)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	}

	if err != nil {
		log.Println("数据库连接失败:", err)
		log.Println("将使用fallback模式运行（硬编码用户验证）")
		return nil
	}

	log.Println("数据库连接成功")

	// 注意：不再自动创建表结构和默认管理员用户
	// 这些操作现在由引导页面通过API接口来处理
	log.Println("数据库连接成功，等待引导页面初始化...")

	return db
}

func Close(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		log.Println("获取数据库连接失败:", err)
		return
	}
	sqlDB.Close()
}
