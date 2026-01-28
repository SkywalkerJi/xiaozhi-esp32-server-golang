# PostgreSQL + MinIO 存储改造方案

## 概述

本文档描述了将现有的 MySQL + Redis 存储架构改造为 PostgreSQL + MinIO 的实现方案。

### 改造目标

1. 用户/设备/配置等结构化数据存储到 PostgreSQL
2. 对话历史从 Redis 迁移到 PostgreSQL（支持 JSONB）
3. 音频文件存储到 MinIO 对象存储
4. Redis 保留为缓存层（可选）

---

## 架构变更

### 原架构
```
MySQL (结构化数据) + Redis (对话历史/缓存)
```

### 新架构
```
PostgreSQL (结构化数据 + 对话历史) + MinIO (音频文件) + Redis (缓存，可选)
```

---

## 新增数据库表结构

### 1. conversation_sessions（对话会话表）

```sql
CREATE TABLE conversation_sessions (
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
```

### 2. conversation_messages（对话消息表）

```sql
CREATE TABLE conversation_messages (
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
```

### 3. system_prompts（系统提示词表）

```sql
CREATE TABLE system_prompts (
    id              BIGSERIAL PRIMARY KEY,
    device_id       VARCHAR(128) NOT NULL UNIQUE,
    agent_id        VARCHAR(128),
    prompt          TEXT NOT NULL,
    is_active       BOOLEAN DEFAULT true,
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### 4. audio_files（音频文件元数据表）

```sql
CREATE TABLE audio_files (
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
```

---

## 文件变更清单

### 新增文件

| 文件路径 | 说明 |
|---------|------|
| `manager/backend/storage/postgres/config.go` | PostgreSQL 配置 |
| `manager/backend/storage/postgres/storage.go` | PostgreSQL 存储实现 |
| `manager/backend/models/conversation.go` | 对话模型定义（Session、Message、AudioFile） |
| `internal/storage/minio/config.go` | MinIO 配置 |
| `internal/storage/minio/client.go` | MinIO 客户端封装 |
| `internal/storage/minio/audio_storage.go` | 音频存储服务 |
| `internal/domain/memory/pg_memory/types.go` | PostgreSQL 记忆类型定义 |
| `internal/domain/memory/pg_memory/pg_memory.go` | PostgreSQL 记忆提供者实现 |
| `docker/docker-composer/init-scripts/01-init.sql` | 数据库初始化 SQL |
| `scripts/migration/mysql_to_pg/main.go` | MySQL 迁移脚本 |
| `scripts/migration/redis_to_pg/main.go` | Redis 迁移脚本 |

### 修改文件

| 文件路径 | 修改内容 |
|---------|---------|
| `go.mod` | 添加 `minio-go/v7` 依赖 |
| `manager/backend/go.mod` | 添加 `gorm.io/driver/postgres`、`gorm.io/datatypes` 依赖 |
| `manager/backend/storage/factory.go` | 添加 `postgres` 存储类型支持 |
| `manager/backend/database/database.go` | 支持 PostgreSQL 数据库初始化 |
| `manager/backend/config/config.go` | 添加 `Type` 和 `SSLMode` 配置字段 |
| `internal/domain/memory/base.go` | 注册 `postgres` 记忆类型 |
| `docker/docker-composer/docker-compose.yml` | 更新为 PostgreSQL + Redis + MinIO 服务 |
| `config/config.yaml` | 添加 MinIO 和对话存储配置 |

---

## 使用指南

### 1. Docker Compose 部署

#### 启动服务

```bash
cd docker/docker-composer
docker-compose up -d
```

#### 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| PostgreSQL | 25432 | 数据库 |
| Redis | 26379 | 缓存 |
| MinIO API | 9000 | 对象存储 API |
| MinIO Console | 9001 | 管理控制台 |
| WebSocket | 8989 | 主服务 |
| Backend | 8081 | 后端管理 |
| Frontend | 8080 | 前端管理 |

#### 访问 MinIO 控制台

- 地址: http://localhost:9001
- 用户名: `minioadmin`
- 密码: `minioadmin123`

### 2. 配置说明

#### 数据库配置 (config.yaml)

```yaml
# 数据库类型配置（环境变量 DB_TYPE）
database:
  type: "postgres"      # mysql, postgres, sqlite
  host: "localhost"
  port: "5432"
  username: "xiaozhi"
  password: "xiaozhi_password"
  database: "xiaozhi_admin"
  ssl_mode: "disable"   # disable, require, verify-ca, verify-full
```

#### MinIO 配置 (config.yaml)

```yaml
minio:
  endpoint: "localhost:9000"          # MinIO 服务地址
  access_key_id: "minioadmin"         # 访问密钥ID
  secret_access_key: "minioadmin123"  # 访问密钥
  use_ssl: false                      # 是否使用SSL
  bucket_audio: "xiaozhi-audio"       # 音频文件存储桶
  region: "us-east-1"                 # 区域
```

#### 对话存储配置 (config.yaml)

```yaml
conversation:
  storage_type: "postgres"            # 对话存储类型: redis, postgres
  postgres:
    enable_audio_storage: true        # 是否启用音频存储
    message_retention_days: 90        # 消息保留天数（0表示永久保留）
```

#### 记忆配置 (config.yaml)

```yaml
memory:
  provider: "postgres"  # nomemo, memobase, mem0, postgres
  postgres:
    host: "localhost"
    port: "5432"
    username: "xiaozhi"
    password: "xiaozhi_password"
    database: "xiaozhi_admin"
    ssl_mode: "disable"
    enable_audio_storage: true
    message_retention_days: 90
```

### 3. 环境变量

支持通过环境变量覆盖配置：

| 环境变量 | 说明 | 默认值 |
|---------|------|--------|
| `DB_TYPE` | 数据库类型 | mysql |
| `DB_HOST` | 数据库主机 | localhost |
| `DB_PORT` | 数据库端口 | 3306/5432 |
| `DB_USER` | 数据库用户 | - |
| `DB_PASSWORD` | 数据库密码 | - |
| `DB_NAME` | 数据库名称 | xiaozhi_admin |
| `DB_SSL_MODE` | SSL 模式 | disable |
| `MINIO_ENDPOINT` | MinIO 地址 | localhost:9000 |
| `MINIO_ACCESS_KEY` | MinIO 访问密钥 | minioadmin |
| `MINIO_SECRET_KEY` | MinIO 密钥 | minioadmin123 |

---

## 数据迁移

### 1. MySQL 到 PostgreSQL 迁移

迁移用户、设备、智能体、配置等结构化数据。

```bash
# 安装依赖
go get github.com/go-sql-driver/mysql
go get github.com/lib/pq

# 执行迁移（先进行 dry-run 测试）
go run scripts/migration/mysql_to_pg/main.go \
  --mysql-host=localhost \
  --mysql-port=3306 \
  --mysql-user=root \
  --mysql-password=your_mysql_password \
  --mysql-db=xiaozhi_admin \
  --pg-host=localhost \
  --pg-port=5432 \
  --pg-user=xiaozhi \
  --pg-password=xiaozhi_password \
  --pg-db=xiaozhi_admin \
  --dry-run

# 确认无误后执行正式迁移
go run scripts/migration/mysql_to_pg/main.go \
  --mysql-host=localhost \
  --mysql-port=3306 \
  --mysql-user=root \
  --mysql-password=your_mysql_password \
  --mysql-db=xiaozhi_admin \
  --pg-host=localhost \
  --pg-port=5432 \
  --pg-user=xiaozhi \
  --pg-password=xiaozhi_password \
  --pg-db=xiaozhi_admin
```

### 2. Redis 到 PostgreSQL 迁移

迁移 Redis 中的对话历史到 PostgreSQL。

```bash
# 执行迁移（先进行 dry-run 测试）
go run scripts/migration/redis_to_pg/main.go \
  --redis-host=localhost \
  --redis-port=6379 \
  --redis-password=your_redis_password \
  --redis-db=0 \
  --key-prefix=xiaozhi \
  --pg-host=localhost \
  --pg-port=5432 \
  --pg-user=xiaozhi \
  --pg-password=xiaozhi_password \
  --pg-db=xiaozhi_admin \
  --dry-run

# 确认无误后执行正式迁移
go run scripts/migration/redis_to_pg/main.go \
  --redis-host=localhost \
  --redis-port=6379 \
  --key-prefix=xiaozhi \
  --pg-host=localhost \
  --pg-port=5432 \
  --pg-user=xiaozhi \
  --pg-password=xiaozhi_password \
  --pg-db=xiaozhi_admin
```

### 3. 验证迁移结果

```bash
# 连接 PostgreSQL
psql -h localhost -p 25432 -U xiaozhi -d xiaozhi_admin

# 检查记录数
SELECT 'users' as table_name, count(*) FROM users
UNION ALL SELECT 'devices', count(*) FROM devices
UNION ALL SELECT 'agents', count(*) FROM agents
UNION ALL SELECT 'configs', count(*) FROM configs
UNION ALL SELECT 'conversation_sessions', count(*) FROM conversation_sessions
UNION ALL SELECT 'conversation_messages', count(*) FROM conversation_messages;
```

---

## MinIO 对象存储结构

音频文件按以下结构存储：

```
xiaozhi-audio/
├── {device_id}/
│   ├── {date}/
│   │   ├── {session_id}/
│   │   │   ├── {file_id}.opus  # 用户输入音频
│   │   │   ├── {file_id}.wav   # TTS 输出音频
```

### 音频文件类型

| 类型 | 扩展名 | 说明 |
|------|--------|------|
| opus | .opus | Opus 编码音频 |
| wav | .wav | WAV 音频 |
| mp3 | .mp3 | MP3 音频 |
| pcm | .pcm | PCM 原始音频 |

### 音频来源类型

| 类型 | 说明 |
|------|------|
| user | 用户输入音频 |
| tts | TTS 合成音频 |
| asr | ASR 处理音频 |

---

## 代码使用示例

### 使用 PostgreSQL 记忆提供者

```go
import "xiaozhi-esp32-server-golang/internal/domain/memory"

// 获取 PostgreSQL 记忆提供者
provider, err := memory.GetProvider(memory.MemoryTypePostgres, map[string]interface{}{
    "host":     "localhost",
    "port":     "5432",
    "username": "xiaozhi",
    "password": "xiaozhi_password",
    "database": "xiaozhi_admin",
    "ssl_mode": "disable",
})

// 添加消息
err = provider.AddMessage(ctx, "device123:session456", schema.Message{
    Role:    schema.RoleUser,
    Content: "你好",
})

// 获取历史消息
messages, err := provider.GetMessages(ctx, "device123:session456", 10)
```

### 使用 MinIO 音频存储

```go
import "xiaozhi-esp32-server-golang/internal/storage/minio"

// 创建 MinIO 客户端
config := &minio.Config{
    Endpoint:        "localhost:9000",
    AccessKeyID:     "minioadmin",
    SecretAccessKey: "minioadmin123",
    UseSSL:          false,
    BucketAudio:     "xiaozhi-audio",
}
client, err := minio.NewClient(config)

// 创建音频存储服务
audioStorage, err := minio.NewAudioStorage(client)

// 上传音频
metadata, err := audioStorage.UploadAudio(ctx, minio.UploadParams{
    DeviceID:   "device123",
    SessionID:  "session456",
    MessageID:  "msg789",
    Data:       audioData,
    FileType:   minio.AudioTypeOpus,
    SourceType: minio.AudioSourceUser,
    SampleRate: 16000,
    Channels:   1,
})

// 下载音频
data, err := audioStorage.DownloadAudio(ctx, metadata.ObjectKey)

// 获取预签名 URL
url, err := audioStorage.GetPresignedURL(ctx, metadata.ObjectKey, time.Hour)
```

---

## 回滚方案

如需回滚到 MySQL 架构：

1. **代码回滚**
   ```bash
   git checkout <previous-commit>
   ```

2. **配置回滚**
   - 修改 `DB_TYPE` 环境变量为 `mysql`
   - 或修改 `config.yaml` 中的 `database.type` 为 `mysql`

3. **Docker 回滚**
   ```bash
   # 停止新服务
   docker-compose down

   # 使用旧版 docker-compose.yml
   git checkout <previous-commit> -- docker/docker-composer/docker-compose.yml
   docker-compose up -d
   ```

4. **数据恢复**
   - 从备份恢复 MySQL 数据库

---

## 常见问题

### Q: PostgreSQL 连接失败

检查以下配置：
- 确认 PostgreSQL 服务已启动
- 检查端口是否正确（默认 5432）
- 检查用户名密码是否正确
- 检查 `ssl_mode` 设置

### Q: MinIO 上传失败

检查以下配置：
- 确认 MinIO 服务已启动
- 检查 `endpoint` 配置是否包含端口
- 检查访问密钥是否正确
- 确认 bucket 已创建或有创建权限

### Q: 迁移脚本运行报错

- 确保安装了相关数据库驱动依赖
- 检查源数据库和目标数据库连接参数
- 先使用 `--dry-run` 模式测试

---

## 性能优化建议

1. **PostgreSQL 索引优化**
   - 对话消息表的 `session_id` + `sequence_num` 复合索引已创建
   - 根据查询模式可添加额外索引

2. **连接池配置**
   - `MaxIdleConns`: 10
   - `MaxOpenConns`: 100
   - `ConnMaxLifetime`: 1 hour

3. **MinIO 性能**
   - 生产环境建议使用分布式部署
   - 启用 SSL/TLS 加密传输

4. **定期清理**
   - 使用 `message_retention_days` 配置自动清理过期消息
   - 定期清理 MinIO 中的过期音频文件
