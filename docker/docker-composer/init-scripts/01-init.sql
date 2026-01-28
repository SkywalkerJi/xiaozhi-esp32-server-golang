-- PostgreSQL 初始化脚本
-- 创建扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(50) NOT NULL UNIQUE,
    password        VARCHAR(255) NOT NULL,
    email           VARCHAR(100) UNIQUE,
    role            VARCHAR(20) NOT NULL DEFAULT 'user',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 设备表
CREATE TABLE IF NOT EXISTS devices (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    agent_id        BIGINT NOT NULL DEFAULT 0,
    device_code     VARCHAR(100) UNIQUE,
    device_name     VARCHAR(100),
    challenge       VARCHAR(128),
    pre_secret_key  VARCHAR(128),
    activated       BOOLEAN DEFAULT false,
    last_active_at  TIMESTAMP WITH TIME ZONE,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 智能体表
CREATE TABLE IF NOT EXISTS agents (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    name            VARCHAR(100) NOT NULL,
    custom_prompt   TEXT,
    llm_config_id   VARCHAR(100),
    tts_config_id   VARCHAR(100),
    asr_speed       VARCHAR(20) DEFAULT 'normal',
    status          VARCHAR(20) DEFAULT 'active',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 配置表
CREATE TABLE IF NOT EXISTS configs (
    id              BIGSERIAL PRIMARY KEY,
    type            VARCHAR(50) NOT NULL,
    name            VARCHAR(100) NOT NULL,
    config_id       VARCHAR(100) NOT NULL,
    provider        VARCHAR(50),
    json_data       TEXT,
    enabled         BOOLEAN DEFAULT true,
    is_default      BOOLEAN DEFAULT false,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(type, config_id)
);

-- 全局角色表
CREATE TABLE IF NOT EXISTS global_roles (
    id              BIGSERIAL PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    prompt          TEXT,
    is_default      BOOLEAN DEFAULT false,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 对话会话表
CREATE TABLE IF NOT EXISTS conversation_sessions (
    id              BIGSERIAL PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL UNIQUE,
    device_id       VARCHAR(128) NOT NULL,
    agent_id        VARCHAR(128),
    user_id         BIGINT REFERENCES users(id),
    started_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    ended_at        TIMESTAMP WITH TIME ZONE,
    status          VARCHAR(20) DEFAULT 'active',
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 对话消息表
CREATE TABLE IF NOT EXISTS conversation_messages (
    id              BIGSERIAL PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL,
    device_id       VARCHAR(128) NOT NULL,
    message_id      VARCHAR(64) NOT NULL UNIQUE,
    sequence_num    BIGINT NOT NULL,
    role            VARCHAR(20) NOT NULL,
    content         TEXT,
    multi_content   JSONB,
    tool_calls      JSONB,
    tool_call_id    VARCHAR(64),
    audio_file_id   VARCHAR(128),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 系统提示词表
CREATE TABLE IF NOT EXISTS system_prompts (
    id              BIGSERIAL PRIMARY KEY,
    device_id       VARCHAR(128) NOT NULL UNIQUE,
    agent_id        VARCHAR(128),
    prompt          TEXT NOT NULL,
    is_active       BOOLEAN DEFAULT true,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 音频文件元数据表
CREATE TABLE IF NOT EXISTS audio_files (
    id              BIGSERIAL PRIMARY KEY,
    file_id         VARCHAR(128) NOT NULL UNIQUE,
    session_id      VARCHAR(64),
    message_id      VARCHAR(64),
    device_id       VARCHAR(128) NOT NULL,
    bucket_name     VARCHAR(64) NOT NULL,
    object_key      VARCHAR(512) NOT NULL,
    file_type       VARCHAR(20) NOT NULL,
    file_size       BIGINT,
    duration_ms     INTEGER,
    sample_rate     INTEGER DEFAULT 16000,
    channels        INTEGER DEFAULT 1,
    source_type     VARCHAR(20) NOT NULL,
    transcription   TEXT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
CREATE INDEX IF NOT EXISTS idx_devices_agent_id ON devices(agent_id);
CREATE INDEX IF NOT EXISTS idx_agents_user_id ON agents(user_id);
CREATE INDEX IF NOT EXISTS idx_configs_type ON configs(type);

CREATE INDEX IF NOT EXISTS idx_conversation_sessions_device_id ON conversation_sessions(device_id);
CREATE INDEX IF NOT EXISTS idx_conversation_sessions_agent_id ON conversation_sessions(agent_id);
CREATE INDEX IF NOT EXISTS idx_conversation_sessions_user_id ON conversation_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_conversation_sessions_status ON conversation_sessions(status);

CREATE INDEX IF NOT EXISTS idx_conversation_messages_session_id ON conversation_messages(session_id);
CREATE INDEX IF NOT EXISTS idx_conversation_messages_device_id ON conversation_messages(device_id);
CREATE INDEX IF NOT EXISTS idx_conversation_messages_session_seq ON conversation_messages(session_id, sequence_num);
CREATE INDEX IF NOT EXISTS idx_conversation_messages_created_at ON conversation_messages(created_at);

CREATE INDEX IF NOT EXISTS idx_audio_files_session_id ON audio_files(session_id);
CREATE INDEX IF NOT EXISTS idx_audio_files_message_id ON audio_files(message_id);
CREATE INDEX IF NOT EXISTS idx_audio_files_device_id ON audio_files(device_id);
CREATE INDEX IF NOT EXISTS idx_audio_files_created_at ON audio_files(created_at);

-- 创建更新时间触发器函数
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 为需要的表添加更新时间触发器
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_devices_updated_at BEFORE UPDATE ON devices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_agents_updated_at BEFORE UPDATE ON agents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_configs_updated_at BEFORE UPDATE ON configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_global_roles_updated_at BEFORE UPDATE ON global_roles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_conversation_sessions_updated_at BEFORE UPDATE ON conversation_sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_system_prompts_updated_at BEFORE UPDATE ON system_prompts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- 输出初始化完成信息
DO $$
BEGIN
    RAISE NOTICE 'Database initialization completed successfully!';
END $$;
