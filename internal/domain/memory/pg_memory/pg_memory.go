package pg_memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	instance     *PGMemory
	instanceOnce sync.Once
	instanceErr  error
)

// PGMemory PostgreSQL记忆提供者
type PGMemory struct {
	db     *gorm.DB
	config *Config
	logger *logrus.Logger
}

// NewPGMemory 创建PostgreSQL记忆提供者
func NewPGMemory(config *Config) (*PGMemory, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Shanghai",
		config.Host, config.Username, config.Password, config.Database, config.Port, config.SSLMode)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// 配置连接池
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// 自动迁移表结构
	if err := db.AutoMigrate(&ConversationSession{}, &ConversationMessage{}); err != nil {
		return nil, fmt.Errorf("failed to auto migrate: %w", err)
	}

	return &PGMemory{
		db:     db,
		config: config,
		logger: logrus.New(),
	}, nil
}

// GetInstance 获取单例实例
func GetInstance(config *Config) (*PGMemory, error) {
	instanceOnce.Do(func() {
		instance, instanceErr = NewPGMemory(config)
	})
	return instance, instanceErr
}

// GetWithConfig 从配置map创建实例
func GetWithConfig(config map[string]interface{}) (*PGMemory, error) {
	cfg := DefaultConfig()

	if v, ok := config["host"].(string); ok && v != "" {
		cfg.Host = v
	}
	if v, ok := config["port"].(string); ok && v != "" {
		cfg.Port = v
	}
	if v, ok := config["username"].(string); ok && v != "" {
		cfg.Username = v
	}
	if v, ok := config["password"].(string); ok && v != "" {
		cfg.Password = v
	}
	if v, ok := config["database"].(string); ok && v != "" {
		cfg.Database = v
	}
	if v, ok := config["ssl_mode"].(string); ok && v != "" {
		cfg.SSLMode = v
	}
	if v, ok := config["enable_audio_storage"].(bool); ok {
		cfg.EnableAudioStorage = v
	}
	if v, ok := config["message_retention_days"].(int); ok {
		cfg.MessageRetentionDays = v
	}

	return GetInstance(cfg)
}

// AddMessage 添加消息到记忆
func (p *PGMemory) AddMessage(ctx context.Context, agentID string, msg schema.Message) error {
	// 解析 agentID 获取 deviceID 和 sessionID
	deviceID, sessionID := parseAgentID(agentID)

	// 确保会话存在
	if err := p.ensureSession(ctx, sessionID, deviceID, agentID); err != nil {
		return fmt.Errorf("failed to ensure session: %w", err)
	}

	// 获取下一个序列号
	var maxSeq int64
	p.db.WithContext(ctx).Model(&ConversationMessage{}).
		Where("session_id = ?", sessionID).
		Select("COALESCE(MAX(sequence_num), 0)").
		Scan(&maxSeq)

	// 创建消息记录
	message := &ConversationMessage{
		SessionID:   sessionID,
		DeviceID:    deviceID,
		MessageID:   uuid.New().String(),
		SequenceNum: maxSeq + 1,
		Role:        string(msg.Role),
		Content:     msg.Content,
		CreatedAt:   time.Now(),
	}

	// 处理多模态内容
	if len(msg.MultiContent) > 0 {
		multiContent := make([]interface{}, len(msg.MultiContent))
		for i, c := range msg.MultiContent {
			multiContent[i] = map[string]interface{}{
				"type": c.Type,
				"text": c.Text,
			}
		}
		message.MultiContent = multiContent
	}

	// 处理工具调用
	if len(msg.ToolCalls) > 0 {
		toolCalls := make([]interface{}, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			toolCalls[i] = map[string]interface{}{
				"id":   tc.ID,
				"type": tc.Type,
				"function": map[string]interface{}{
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				},
			}
		}
		message.ToolCalls = toolCalls
	}

	if msg.ToolCallID != "" {
		message.ToolCallID = msg.ToolCallID
	}

	if err := p.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	return nil
}

// GetMessages 获取历史消息
func (p *PGMemory) GetMessages(ctx context.Context, agentID string, count int) ([]*schema.Message, error) {
	_, sessionID := parseAgentID(agentID)

	var messages []ConversationMessage
	if err := p.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence_num DESC").
		Limit(count).
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}

	// 反转顺序（从旧到新）
	result := make([]*schema.Message, len(messages))
	for i, msg := range messages {
		result[len(messages)-1-i] = p.convertToSchemaMessage(&msg)
	}

	return result, nil
}

// GetContext 获取上下文信息
func (p *PGMemory) GetContext(ctx context.Context, agentID string, maxToken int) (string, error) {
	// PostgreSQL 记忆不支持摘要功能，返回空
	return "", nil
}

// Search 搜索记忆
func (p *PGMemory) Search(ctx context.Context, agentID string, query string, topK int, timeRangeDays int64) (string, error) {
	_, sessionID := parseAgentID(agentID)

	var messages []ConversationMessage
	queryBuilder := p.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Where("content ILIKE ?", "%"+query+"%")

	if timeRangeDays > 0 {
		startTime := time.Now().AddDate(0, 0, -int(timeRangeDays))
		queryBuilder = queryBuilder.Where("created_at >= ?", startTime)
	}

	if err := queryBuilder.
		Order("created_at DESC").
		Limit(topK).
		Find(&messages).Error; err != nil {
		return "", fmt.Errorf("failed to search messages: %w", err)
	}

	// 构建搜索结果
	var result string
	for _, msg := range messages {
		result += fmt.Sprintf("[%s] %s: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05"), msg.Role, msg.Content)
	}

	return result, nil
}

// Flush 刷新记忆（立即保存）
func (p *PGMemory) Flush(ctx context.Context, agentID string) error {
	// PostgreSQL 自动持久化，无需额外操作
	return nil
}

// ResetMemory 重置记忆
func (p *PGMemory) ResetMemory(ctx context.Context, agentID string) error {
	_, sessionID := parseAgentID(agentID)

	// 删除会话的所有消息
	if err := p.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Delete(&ConversationMessage{}).Error; err != nil {
		return fmt.Errorf("failed to delete messages: %w", err)
	}

	// 更新会话状态
	if err := p.db.WithContext(ctx).
		Model(&ConversationSession{}).
		Where("session_id = ?", sessionID).
		Update("status", "reset").Error; err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	return nil
}

// ensureSession 确保会话存在
func (p *PGMemory) ensureSession(ctx context.Context, sessionID, deviceID, agentID string) error {
	var session ConversationSession
	result := p.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&session)

	if result.Error == gorm.ErrRecordNotFound {
		// 创建新会话
		newSession := &ConversationSession{
			SessionID: sessionID,
			DeviceID:  deviceID,
			AgentID:   agentID,
			Status:    "active",
			StartedAt: time.Now(),
		}
		if err := p.db.WithContext(ctx).Create(newSession).Error; err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}
	} else if result.Error != nil {
		return fmt.Errorf("failed to query session: %w", result.Error)
	}

	return nil
}

// convertToSchemaMessage 转换为eino的Message类型
func (p *PGMemory) convertToSchemaMessage(msg *ConversationMessage) *schema.Message {
	result := &schema.Message{
		Role:       schema.RoleType(msg.Role),
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}

	// 转换多模态内容
	if len(msg.MultiContent) > 0 {
		for _, c := range msg.MultiContent {
			if contentMap, ok := c.(map[string]interface{}); ok {
				part := &schema.ChatMessagePart{}
				if t, ok := contentMap["type"].(string); ok {
					part.Type = schema.ChatMessagePartType(t)
				}
				if text, ok := contentMap["text"].(string); ok {
					part.Text = text
				}
				result.MultiContent = append(result.MultiContent, *part)
			}
		}
	}

	// 转换工具调用
	if len(msg.ToolCalls) > 0 {
		for _, tc := range msg.ToolCalls {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				toolCall := schema.ToolCall{}
				if id, ok := tcMap["id"].(string); ok {
					toolCall.ID = id
				}
				if t, ok := tcMap["type"].(string); ok {
					toolCall.Type = t
				}
				if fn, ok := tcMap["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok {
						toolCall.Function.Name = name
					}
					if args, ok := fn["arguments"].(string); ok {
						toolCall.Function.Arguments = args
					}
				}
				result.ToolCalls = append(result.ToolCalls, toolCall)
			}
		}
	}

	return result
}

// parseAgentID 解析agentID获取deviceID和sessionID
// agentID格式: deviceID 或 deviceID:sessionID
func parseAgentID(agentID string) (deviceID, sessionID string) {
	deviceID = agentID
	sessionID = agentID

	// 尝试解析格式
	for i := len(agentID) - 1; i >= 0; i-- {
		if agentID[i] == ':' {
			deviceID = agentID[:i]
			sessionID = agentID[i+1:]
			break
		}
	}

	return deviceID, sessionID
}

// EndSession 结束会话
func (p *PGMemory) EndSession(ctx context.Context, sessionID string) error {
	now := time.Now()
	return p.db.WithContext(ctx).
		Model(&ConversationSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":   "ended",
			"ended_at": now,
		}).Error
}

// GetSessionMessages 获取会话的所有消息
func (p *PGMemory) GetSessionMessages(ctx context.Context, sessionID string) ([]ConversationMessage, error) {
	var messages []ConversationMessage
	if err := p.db.WithContext(ctx).
		Where("session_id = ?", sessionID).
		Order("sequence_num ASC").
		Find(&messages).Error; err != nil {
		return nil, fmt.Errorf("failed to get session messages: %w", err)
	}
	return messages, nil
}

// CleanupOldMessages 清理过期消息
func (p *PGMemory) CleanupOldMessages(ctx context.Context) error {
	if p.config.MessageRetentionDays <= 0 {
		return nil
	}

	cutoffTime := time.Now().AddDate(0, 0, -p.config.MessageRetentionDays)

	return p.db.WithContext(ctx).
		Where("created_at < ?", cutoffTime).
		Delete(&ConversationMessage{}).Error
}
