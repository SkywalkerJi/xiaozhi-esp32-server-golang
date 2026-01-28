package pg_memory

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSONB 自定义JSONB类型
type JSONB map[string]interface{}

// Value 实现driver.Valuer接口
func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现sql.Scanner接口
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// JSONBArray 自定义JSONB数组类型
type JSONBArray []interface{}

// Value 实现driver.Valuer接口
func (j JSONBArray) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan 实现sql.Scanner接口
func (j *JSONBArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(bytes, j)
}

// ConversationSession 对话会话模型
type ConversationSession struct {
	ID        int64     `gorm:"primarykey;autoIncrement"`
	SessionID string    `gorm:"type:varchar(64);not null;uniqueIndex"`
	DeviceID  string    `gorm:"type:varchar(128);not null;index"`
	AgentID   string    `gorm:"type:varchar(128);index"`
	UserID    *int64    `gorm:"index"`
	StartedAt time.Time `gorm:"autoCreateTime"`
	EndedAt   *time.Time
	Status    string    `gorm:"type:varchar(20);default:'active'"`
	Metadata  JSONB     `gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (ConversationSession) TableName() string {
	return "conversation_sessions"
}

// ConversationMessage 对话消息模型
type ConversationMessage struct {
	ID           int64      `gorm:"primarykey;autoIncrement"`
	SessionID    string     `gorm:"type:varchar(64);not null;index:idx_session_seq"`
	DeviceID     string     `gorm:"type:varchar(128);not null;index"`
	MessageID    string     `gorm:"type:varchar(64);not null;uniqueIndex"`
	SequenceNum  int64      `gorm:"not null;index:idx_session_seq"`
	Role         string     `gorm:"type:varchar(20);not null"`
	Content      string     `gorm:"type:text"`
	MultiContent JSONBArray `gorm:"type:jsonb"`
	ToolCalls    JSONBArray `gorm:"type:jsonb"`
	ToolCallID   string     `gorm:"type:varchar(64)"`
	AudioFileID  string     `gorm:"type:varchar(128)"`
	CreatedAt    time.Time  `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (ConversationMessage) TableName() string {
	return "conversation_messages"
}

// Config PostgreSQL记忆配置
type Config struct {
	Host                  string `mapstructure:"host"`
	Port                  string `mapstructure:"port"`
	Username              string `mapstructure:"username"`
	Password              string `mapstructure:"password"`
	Database              string `mapstructure:"database"`
	SSLMode               string `mapstructure:"ssl_mode"`
	EnableAudioStorage    bool   `mapstructure:"enable_audio_storage"`
	MessageRetentionDays  int    `mapstructure:"message_retention_days"`
	MaxMessagesPerSession int    `mapstructure:"max_messages_per_session"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Host:                  "localhost",
		Port:                  "5432",
		Username:              "xiaozhi",
		Password:              "xiaozhi_password",
		Database:              "xiaozhi_admin",
		SSLMode:               "disable",
		EnableAudioStorage:    false,
		MessageRetentionDays:  90,
		MaxMessagesPerSession: 1000,
	}
}
