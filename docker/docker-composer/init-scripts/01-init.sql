-- PostgreSQL 初始化脚本
-- 仅创建扩展，表结构由 GORM AutoMigrate 自动创建
-- 这样可以保证约束命名与 GORM 一致

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 创建更新时间触发器函数（供 GORM 创建表后使用）
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 输出初始化完成信息
DO $$
BEGIN
    RAISE NOTICE 'PostgreSQL extensions initialized. Tables will be created by GORM AutoMigrate.';
END $$;
