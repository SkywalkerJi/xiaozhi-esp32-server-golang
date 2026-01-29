# PostgreSQL + MinIO 存储改造方案

## 快速开始（本地开发）

```bash
# 1. 启动基础设施服务
cd docker/docker-composer
docker-compose -f docker-compose.dev.yml up -d

# 2. 等待服务就绪（约 30 秒）
docker-compose -f docker-compose.dev.yml ps

# 3. 配置后端使用 PostgreSQL
cp manager/backend/config/config.dev.json manager/backend/config/config.json

# 4. 启动后端服务（新终端）
cd manager/backend && go run main.go

# 5. 启动前端服务（新终端）
cd manager/frontend && npm run dev

# 6. 设置 onnxruntime 环境变量并启动主服务（新终端）
export CGO_CFLAGS="-I/usr/local/include/onnxruntime"
export CGO_LDFLAGS="-L/usr/local/lib -lonnxruntime"
cd cmd/server && go run main.go -c ../../config/config.yaml

# 访问地址：
# - 前端管理：http://localhost:5173 (Vite dev server)
# - 后端 API：http://localhost:8080
# - MinIO 控制台：http://localhost:19001 (minioadmin/minioadmin123)
# - PostgreSQL：localhost:15432 (xiaozhi/xiaozhi_password)
# - Redis：localhost:16379
```

### 验证存储功能

```bash
# 检查 PostgreSQL 中的聊天记录
PGPASSWORD=xiaozhi_password psql -h localhost -p 15432 -U xiaozhi -d xiaozhi_admin -c \
  "SELECT COUNT(*) as sessions FROM conversation_sessions;"

PGPASSWORD=xiaozhi_password psql -h localhost -p 15432 -U xiaozhi -d xiaozhi_admin -c \
  "SELECT COUNT(*) as messages FROM conversation_messages;"

# 检查 MinIO 中的音频文件
docker exec xiaozhi-minio mc ls local/xiaozhi-audio/ --recursive
```

---

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
| `docker/docker-composer/docker-compose.yml` | 更新为 PostgreSQL + Redis + MinIO 服务（生产环境） |
| `docker/docker-composer/docker-compose.dev.yml` | 新增本地开发环境配置（仅基础设施） |
| `config/config.yaml` | 添加 MinIO 和对话存储配置 |
| `manager/backend/config/config.dev.json` | 新增 PostgreSQL 开发环境配置示例 |

---

## 使用指南

### 1. Docker Compose 部署

#### 方式一：完整部署（生产环境）

启动所有服务（PostgreSQL + Redis + MinIO + 前后端 + 主服务）：

```bash
cd docker/docker-composer
docker-compose up -d
```

#### 方式二：本地开发（推荐）

仅启动基础设施服务，前后端和主服务在本地运行：

```bash
# 启动基础设施
cd docker/docker-composer
docker-compose -f docker-compose.dev.yml up -d

# 本地启动后端服务
cd manager/backend
go run main.go

# 本地启动前端服务
cd manager/frontend
npm run dev

# 本地启动主服务
cd cmd/server
go run main.go -c ../../config/config.yaml
```

#### 开发环境常用命令

```bash
# 查看服务状态
docker-compose -f docker-compose.dev.yml ps

# 查看日志
docker-compose -f docker-compose.dev.yml logs -f

# 查看单个服务日志
docker-compose -f docker-compose.dev.yml logs -f postgres

# 停止服务
docker-compose -f docker-compose.dev.yml down

# 停止并清理数据（慎用）
docker-compose -f docker-compose.dev.yml down -v

# 重启单个服务
docker-compose -f docker-compose.dev.yml restart postgres
```

#### 服务端口

**开发环境 (docker-compose.dev.yml)**

| 服务 | 端口 | 说明 |
|------|------|------|
| PostgreSQL | 15432 | 数据库 |
| Redis | 16379 | 缓存 |
| MinIO API | 9000 | 对象存储 API |
| MinIO Console | 19001 | 管理控制台 |

**生产环境 (docker-compose.yml)**

| 服务 | 端口 | 说明 |
|------|------|------|
| PostgreSQL | 25432 | 数据库（映射避免冲突） |
| Redis | 26379 | 缓存（映射避免冲突） |
| MinIO API | 9000 | 对象存储 API |
| MinIO Console | 9001 | 管理控制台 |
| WebSocket | 8989 | 主服务 |
| Backend | 8081 | 后端管理 |
| Frontend | 8080 | 前端管理 |

#### 访问 MinIO 控制台

- 地址: http://localhost:9001
- 用户名: `minioadmin`
- 密码: `minioadmin123`

#### 本地开发配置示例

开发环境下 `config/config.yaml` 相关配置：

```yaml
# 后端管理服务地址（本地运行）
manager:
  backend_url: "http://127.0.0.1:8080"

# Memory 配置使用 PostgreSQL
memory:
  provider: "postgres"
  postgres:
    host: "localhost"
    port: "15432"
    username: "xiaozhi"
    password: "xiaozhi_password"
    database: "xiaozhi_admin"
    ssl_mode: "disable"

# MinIO 配置
minio:
  endpoint: "localhost:9000"
  access_key_id: "minioadmin"
  secret_access_key: "minioadmin123"
  use_ssl: false
  bucket_audio: "xiaozhi-audio"
```

后端管理服务配置：

```bash
# 方式一：使用开发配置文件
cp manager/backend/config/config.dev.json manager/backend/config/config.json

# 方式二：使用环境变量覆盖
export DB_TYPE=postgres
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=xiaozhi
export DB_PASSWORD=xiaozhi_password
export DB_NAME=xiaozhi_admin
```

`manager/backend/config/config.dev.json` 已包含 PostgreSQL 配置：

```json
{
  "server": { "port": "8080", "mode": "debug" },
  "database": {
    "type": "postgres",
    "host": "127.0.0.1",
    "port": "15432",
    "username": "xiaozhi",
    "password": "xiaozhi_password",
    "database": "xiaozhi_admin",
    "ssl_mode": "disable"
  },
  "jwt": { "secret": "xiaozhi_admin_secret_key", "expire_hour": 24 }
}
```

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

## 聊天记录存储

### 工作原理

聊天记录通过 PostgreSQL Memory Provider 自动保存：

```
用户说话 → ASR 识别 → 消息保存到 PostgreSQL
LLM 回复 → 消息保存到 PostgreSQL
```

### 数据表结构

**conversation_sessions（会话表）**
| 字段 | 类型 | 说明 |
|------|------|------|
| session_id | VARCHAR(64) | 会话唯一标识 |
| device_id | VARCHAR(128) | 设备 ID |
| agent_id | VARCHAR(128) | 智能体 ID |
| status | VARCHAR(20) | 状态：active, ended, reset |
| metadata | JSONB | 元数据 |

**conversation_messages（消息表）**
| 字段 | 类型 | 说明 |
|------|------|------|
| session_id | VARCHAR(64) | 所属会话 |
| message_id | VARCHAR(64) | 消息唯一标识 |
| sequence_num | BIGINT | 消息序号 |
| role | VARCHAR(20) | 角色：user, assistant, system, tool |
| content | TEXT | 消息内容 |
| multi_content | JSONB | 多模态内容 |
| tool_calls | JSONB | 工具调用 |
| audio_file_id | VARCHAR(128) | 关联的音频文件 ID |

### 配置方式

1. **在管理后台配置**：添加 Memory 配置，类型选择 `postgres`

2. **直接数据库配置**：
```sql
INSERT INTO configs (type, name, config_id, provider, json_data, enabled, is_default)
VALUES (
  'memory', 'PostgreSQL Memory', 'postgres', 'postgres',
  '{"host":"localhost","port":"15432","username":"xiaozhi","password":"xiaozhi_password","database":"xiaozhi_admin","ssl_mode":"disable"}',
  true, true
);
```

### 查询示例

```sql
-- 查看最近的对话会话
SELECT session_id, device_id, status, started_at
FROM conversation_sessions
ORDER BY started_at DESC LIMIT 10;

-- 查看某个会话的消息
SELECT role, content, created_at
FROM conversation_messages
WHERE session_id = 'xxx'
ORDER BY sequence_num;

-- 统计每日消息量
SELECT DATE(created_at) as date, COUNT(*) as count
FROM conversation_messages
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

---

## 音频文件存储

### 工作原理

音频通过 AudioCollector 收集，自动保存到 MinIO：

```
用户说话 → 收集 opus 音频帧 → 用户停止说话 → 保存到 MinIO
TTS 回复 → 收集 opus 音频帧 → TTS 结束 → 保存到 MinIO
```

### 存储路径结构

```
xiaozhi-audio/                    # Bucket 名称
└── {device_id}/                  # 设备目录
    └── {date}/                   # 日期目录 (YYYY-MM-DD)
        └── {session_id}/         # 会话目录
            ├── user-{timestamp}.opus   # 用户输入音频
            └── tts-{timestamp}.opus    # TTS 输出音频
```

### 音频元数据表

**audio_files（音频文件表）**
| 字段 | 类型 | 说明 |
|------|------|------|
| file_id | VARCHAR(128) | 文件唯一标识 |
| session_id | VARCHAR(64) | 所属会话 |
| message_id | VARCHAR(64) | 关联消息 ID |
| device_id | VARCHAR(128) | 设备 ID |
| bucket_name | VARCHAR(64) | 存储桶名称 |
| object_key | VARCHAR(512) | 对象存储路径 |
| file_type | VARCHAR(20) | 文件类型：opus, wav, mp3, pcm |
| file_size | BIGINT | 文件大小（字节） |
| duration_ms | INTEGER | 时长（毫秒） |
| source_type | VARCHAR(20) | 来源：user, tts, asr |

### 配置说明

在 `config/config.yaml` 中配置 MinIO：

```yaml
minio:
  endpoint: "localhost:9000"          # MinIO 服务地址
  access_key_id: "minioadmin"         # 访问密钥
  secret_access_key: "minioadmin123"  # 密钥
  use_ssl: false                      # 是否使用 SSL
  bucket_audio: "xiaozhi-audio"       # 音频存储桶
  region: "us-east-1"                 # 区域
```

### 访问音频文件

**通过 MinIO 控制台**：
- 地址：http://localhost:19001
- 用户名：minioadmin
- 密码：minioadmin123

**通过 mc 命令行**：
```bash
# 配置 mc
mc alias set myminio http://localhost:9000 minioadmin minioadmin123

# 列出音频文件
mc ls myminio/xiaozhi-audio/

# 下载音频文件
mc cp myminio/xiaozhi-audio/device123/2026-01-29/session456/user-xxx.opus ./
```

**通过预签名 URL**：
```go
url, err := audioStorage.GetPresignedURL(ctx, objectKey, time.Hour)
```

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

## 实现架构

### 核心组件

```
┌─────────────────────────────────────────────────────────────┐
│                        主服务                                │
├─────────────────────────────────────────────────────────────┤
│  ChatSession                                                 │
│    ├── HandleAudioMessage() ──→ AudioCollector.AddUserAudio()│
│    └── TTSManager                                            │
│          └── SendTTSAudio() ──→ AudioCollector.AddTTSAudio() │
├─────────────────────────────────────────────────────────────┤
│  EventHandle                                                 │
│    ├── HandleAddMessage() ──→ MemoryProvider.AddMessage()   │
│    ├── HandleSessionEnd() ──→ MemoryProvider.Flush()        │
│    └── HandleSaveAudio() ──→ AudioStorage.UploadAudio()     │
├─────────────────────────────────────────────────────────────┤
│  EventBus                                                    │
│    ├── TopicAddMessage  (消息保存事件)                        │
│    ├── TopicSessionEnd  (会话结束事件)                        │
│    └── TopicSaveAudio   (音频保存事件)                        │
└─────────────────────────────────────────────────────────────┘
          │                              │
          ▼                              ▼
┌─────────────────┐            ┌─────────────────┐
│   PostgreSQL    │            │     MinIO       │
│ ┌─────────────┐ │            │ ┌─────────────┐ │
│ │conversation_│ │            │ │xiaozhi-audio│ │
│ │  sessions   │ │            │ │   bucket    │ │
│ ├─────────────┤ │            │ └─────────────┘ │
│ │conversation_│ │            └─────────────────┘
│ │  messages   │ │
│ └─────────────┘ │
└─────────────────┘
```

### 数据流

**用户输入流程**：
```
1. 用户说话（opus 音频流）
2. HandleAudioMessage() 接收音频帧
3. AudioCollector.AddUserAudio() 累积音频数据
4. ASR 识别完成，生成文本消息
5. EventBus 发布 TopicAddMessage 事件
6. MemoryProvider.AddMessage() 保存到 PostgreSQL
7. 用户停止说话，OnVoiceSilence() 触发
8. AudioCollector.SaveUserAudio() 发布 TopicSaveAudio 事件
9. AudioStorage.UploadAudio() 保存到 MinIO
```

**TTS 输出流程**：
```
1. LLM 生成回复文本
2. EventBus 发布 TopicAddMessage 事件
3. MemoryProvider.AddMessage() 保存到 PostgreSQL
4. TTS 合成音频，SendTTSAudio() 发送
5. AudioCollector.AddTTSAudio() 累积音频数据
6. TTS 完成，AudioCollector.SaveTTSAudio() 触发
7. EventBus 发布 TopicSaveAudio 事件
8. AudioStorage.UploadAudio() 保存到 MinIO
```

### 关键文件

| 文件 | 说明 |
|------|------|
| `internal/data/client/audio_collector.go` | 音频收集器，累积音频帧 |
| `internal/domain/eventbus/types.go` | 事件类型定义 |
| `internal/app/server/event_handle.go` | 事件处理，MinIO 初始化 |
| `internal/storage/minio/audio_storage.go` | MinIO 音频存储服务 |
| `internal/domain/memory/pg_memory/pg_memory.go` | PostgreSQL 记忆提供者 |

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

### Q: 聊天记录没有保存

检查以下配置：
1. Memory Provider 是否设置为 `postgres`
2. PostgreSQL 连接参数是否正确
3. 查看日志是否有 Memory Provider 初始化错误

```bash
# 检查数据库中的 Memory 配置
psql -h localhost -p 15432 -U xiaozhi -d xiaozhi_admin -c \
  "SELECT * FROM configs WHERE type = 'memory' AND is_default = true;"
```

### Q: 音频没有保存到 MinIO

检查以下配置：
1. `config.yaml` 中是否配置了 `minio` 部分
2. MinIO 服务是否正常运行
3. 查看日志是否有 "MinIO 音频存储初始化成功" 或错误信息

```bash
# 检查 MinIO 服务状态
docker-compose -f docker-compose.dev.yml ps minio

# 检查 bucket 是否存在
mc ls myminio/xiaozhi-audio/
```

### Q: 如何禁用音频保存

在 `config.yaml` 中删除或注释 `minio` 配置部分，主服务启动时会跳过 MinIO 初始化。

### Q: 如何清理过期数据

```sql
-- 清理 90 天前的消息
DELETE FROM conversation_messages
WHERE created_at < NOW() - INTERVAL '90 days';

-- 清理对应的会话（无消息的会话）
DELETE FROM conversation_sessions s
WHERE NOT EXISTS (
  SELECT 1 FROM conversation_messages m
  WHERE m.session_id = s.session_id
);
```

```bash
# 清理 MinIO 中的过期文件（需要配置生命周期规则）
mc ilm rule add myminio/xiaozhi-audio --expire-days 90
```

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
