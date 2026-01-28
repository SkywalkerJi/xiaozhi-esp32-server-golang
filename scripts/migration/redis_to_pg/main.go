package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// 迁移配置
type Config struct {
	// Redis 配置
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int
	KeyPrefix     string

	// PostgreSQL 配置
	PGHost     string
	PGPort     string
	PGUser     string
	PGPassword string
	PGDatabase string
	PGSSLMode  string

	// 迁移选项
	DryRun    bool
	BatchSize int
}

// RedisMessage Redis中存储的消息格式
type RedisMessage struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  interface{} `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	Timestamp  int64       `json:"timestamp,omitempty"`
}

func main() {
	config := parseFlags()

	log.Println("Starting Redis to PostgreSQL conversation migration...")

	// 连接 Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})
	defer rdb.Close()

	ctx := context.Background()

	// 测试 Redis 连接
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis successfully")

	// 连接 PostgreSQL
	pgDSN := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		config.PGHost, config.PGPort, config.PGUser, config.PGPassword, config.PGDatabase, config.PGSSLMode)
	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil {
		log.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	if err := pgDB.Ping(); err != nil {
		log.Fatalf("Failed to ping PostgreSQL: %v", err)
	}
	log.Println("Connected to PostgreSQL successfully")

	if config.DryRun {
		log.Println("DRY RUN MODE - No data will be written")
	}

	// 扫描 Redis 中的对话历史键
	pattern := fmt.Sprintf("%s:conversation:*", config.KeyPrefix)
	log.Printf("Scanning keys with pattern: %s", pattern)

	var cursor uint64
	var totalKeys int
	var migratedSessions int

	for {
		keys, nextCursor, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			log.Fatalf("Failed to scan Redis keys: %v", err)
		}

		for _, key := range keys {
			totalKeys++
			if err := migrateConversation(ctx, rdb, pgDB, key, config); err != nil {
				log.Printf("Warning: Failed to migrate key %s: %v", key, err)
			} else {
				migratedSessions++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	log.Printf("Migration completed! Total keys scanned: %d, Sessions migrated: %d", totalKeys, migratedSessions)
}

func parseFlags() *Config {
	config := &Config{}

	// Redis flags
	flag.StringVar(&config.RedisHost, "redis-host", "localhost", "Redis host")
	flag.StringVar(&config.RedisPort, "redis-port", "6379", "Redis port")
	flag.StringVar(&config.RedisPassword, "redis-password", "", "Redis password")
	flag.IntVar(&config.RedisDB, "redis-db", 0, "Redis database")
	flag.StringVar(&config.KeyPrefix, "key-prefix", "xiaozhi", "Redis key prefix")

	// PostgreSQL flags
	flag.StringVar(&config.PGHost, "pg-host", "localhost", "PostgreSQL host")
	flag.StringVar(&config.PGPort, "pg-port", "5432", "PostgreSQL port")
	flag.StringVar(&config.PGUser, "pg-user", "xiaozhi", "PostgreSQL user")
	flag.StringVar(&config.PGPassword, "pg-password", "xiaozhi_password", "PostgreSQL password")
	flag.StringVar(&config.PGDatabase, "pg-db", "xiaozhi_admin", "PostgreSQL database")
	flag.StringVar(&config.PGSSLMode, "pg-sslmode", "disable", "PostgreSQL SSL mode")

	// Migration options
	flag.BoolVar(&config.DryRun, "dry-run", false, "Dry run mode")
	flag.IntVar(&config.BatchSize, "batch-size", 100, "Batch size for migration")

	flag.Parse()
	return config
}

func migrateConversation(ctx context.Context, rdb *redis.Client, pgDB *sql.DB, key string, config *Config) error {
	// 解析 key 获取 deviceID/sessionID
	// 格式: xiaozhi:conversation:{deviceID} 或 xiaozhi:conversation:{deviceID}:{sessionID}
	parts := strings.Split(key, ":")
	if len(parts) < 3 {
		return fmt.Errorf("invalid key format: %s", key)
	}

	deviceID := parts[2]
	sessionID := deviceID // 默认使用 deviceID 作为 sessionID
	if len(parts) > 3 {
		sessionID = parts[3]
	}

	// 获取 Redis 中的消息列表
	messages, err := rdb.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}

	if len(messages) == 0 {
		return nil
	}

	log.Printf("  Migrating session %s with %d messages", sessionID, len(messages))

	if config.DryRun {
		return nil
	}

	// 开始事务
	tx, err := pgDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 创建会话记录
	_, err = tx.ExecContext(ctx, `
		INSERT INTO conversation_sessions (session_id, device_id, status, started_at)
		VALUES ($1, $2, 'migrated', $3)
		ON CONFLICT (session_id) DO NOTHING
	`, sessionID, deviceID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// 插入消息
	for i, msgJSON := range messages {
		var msg RedisMessage
		if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
			log.Printf("    Warning: Failed to parse message %d: %v", i, err)
			continue
		}

		messageID := fmt.Sprintf("%s-%d", sessionID, i)
		createdAt := time.Now()
		if msg.Timestamp > 0 {
			createdAt = time.Unix(msg.Timestamp/1000, (msg.Timestamp%1000)*1000000)
		}

		var toolCallsJSON []byte
		if msg.ToolCalls != nil {
			toolCallsJSON, _ = json.Marshal(msg.ToolCalls)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO conversation_messages (session_id, device_id, message_id, sequence_num, role, content, tool_calls, tool_call_id, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (message_id) DO NOTHING
		`, sessionID, deviceID, messageID, i+1, msg.Role, msg.Content, toolCallsJSON, msg.ToolCallID, createdAt)
		if err != nil {
			log.Printf("    Warning: Failed to insert message %d: %v", i, err)
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
