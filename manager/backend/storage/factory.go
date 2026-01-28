package storage

import (
	"fmt"

	"xiaozhi/manager/backend/config"
	"xiaozhi/manager/backend/storage/mysql"
	"xiaozhi/manager/backend/storage/postgres"
)

// StorageType 存储类型
type StorageType string

const (
	StorageTypeMySQL    StorageType = "mysql"
	StorageTypePostgres StorageType = "postgres"
)

// Factory 存储工厂
type Factory struct{}

// NewFactory 创建存储工厂
func NewFactory() *Factory {
	return &Factory{}
}

// CreateStorage 创建存储实例
func CreateStorage(dbConfig config.DatabaseConfig) (*StorageAdapter, error) {
	dbType := StorageType(dbConfig.Type)
	if dbType == "" {
		dbType = StorageTypeMySQL // 默认使用 MySQL 保持向后兼容
	}

	switch dbType {
	case StorageTypePostgres:
		// 验证PostgreSQL配置
		if err := postgres.ValidateConfig(dbConfig); err != nil {
			return nil, fmt.Errorf("invalid PostgreSQL config: %w", err)
		}
		// 创建PostgreSQL配置
		pgConfig := postgres.NewConfigFromDatabase(dbConfig)
		// 创建PostgreSQL存储
		pgStorage, err := postgres.NewStorage(pgConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create PostgreSQL storage: %w", err)
		}
		// 创建基础存储
		baseStorage := NewGormBaseStorage(pgStorage.DB)
		// 返回适配器
		return NewStorageAdapter(baseStorage), nil

	case StorageTypeMySQL:
		fallthrough
	default:
		// 验证MySQL配置
		if err := mysql.ValidateConfig(dbConfig); err != nil {
			return nil, fmt.Errorf("invalid MySQL config: %w", err)
		}
		// 创建MySQL配置
		mysqlConfig := mysql.NewConfigFromDatabase(dbConfig)
		// 创建MySQL存储
		mysqlStorage, err := mysql.NewStorage(mysqlConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create MySQL storage: %w", err)
		}
		// 创建基础存储
		baseStorage := NewGormBaseStorage(mysqlStorage.DB)
		// 返回适配器
		return NewStorageAdapter(baseStorage), nil
	}
}

// GetSupportedTypes 获取支持的存储类型
func (f *Factory) GetSupportedTypes() []StorageType {
	return []StorageType{
		StorageTypeMySQL,
		StorageTypePostgres,
	}
}

// ValidateConfig 验证存储配置
func ValidateConfig(storageType string, dbConfig config.DatabaseConfig) error {
	switch StorageType(storageType) {
	case StorageTypeMySQL:
		return mysql.ValidateConfig(dbConfig)
	case StorageTypePostgres:
		return postgres.ValidateConfig(dbConfig)
	default:
		return fmt.Errorf("unsupported storage type: %s", storageType)
	}
}