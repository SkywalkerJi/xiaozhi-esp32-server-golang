package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSONB 自定义JSONB类型，支持PostgreSQL JSONB
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

// ConversationSession 对话会话表
type ConversationSession struct {
	ID        int64      `json:"id" gorm:"primarykey;autoIncrement"`
	SessionID string     `json:"session_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	DeviceID  string     `json:"device_id" gorm:"type:varchar(128);not null;index"`
	AgentID   string     `json:"agent_id" gorm:"type:varchar(128);index"`
	UserID    *int64     `json:"user_id" gorm:"index"`
	StartedAt time.Time  `json:"started_at" gorm:"autoCreateTime"`
	EndedAt   *time.Time `json:"ended_at"`
	Status    string     `json:"status" gorm:"type:varchar(20);default:'active'"` // active, ended, timeout
	Metadata  JSONB      `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (ConversationSession) TableName() string {
	return "conversation_sessions"
}

// ConversationMessage 对话消息表
type ConversationMessage struct {
	ID           int64      `json:"id" gorm:"primarykey;autoIncrement"`
	SessionID    string     `json:"session_id" gorm:"type:varchar(64);not null;index:idx_session_seq"`
	DeviceID     string     `json:"device_id" gorm:"type:varchar(128);not null;index"`
	MessageID    string     `json:"message_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	SequenceNum  int64      `json:"sequence_num" gorm:"not null;index:idx_session_seq"`
	Role         string     `json:"role" gorm:"type:varchar(20);not null"` // user, assistant, system, tool
	Content      string     `json:"content" gorm:"type:text"`
	MultiContent JSONBArray `json:"multi_content" gorm:"type:jsonb"`       // 多模态内容
	ToolCalls    JSONBArray `json:"tool_calls" gorm:"type:jsonb"`          // 工具调用
	ToolCallID   string     `json:"tool_call_id" gorm:"type:varchar(64)"`  // 工具调用ID
	AudioFileID  string     `json:"audio_file_id" gorm:"type:varchar(128)"` // 音频文件ID
	CreatedAt    time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (ConversationMessage) TableName() string {
	return "conversation_messages"
}

// SystemPrompt 系统提示词表
type SystemPrompt struct {
	ID        int64     `json:"id" gorm:"primarykey;autoIncrement"`
	DeviceID  string    `json:"device_id" gorm:"type:varchar(128);not null;uniqueIndex"`
	AgentID   string    `json:"agent_id" gorm:"type:varchar(128);index"`
	Prompt    string    `json:"prompt" gorm:"type:text;not null"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (SystemPrompt) TableName() string {
	return "system_prompts"
}

// AudioFile 音频文件元数据表
type AudioFile struct {
	ID            int64     `json:"id" gorm:"primarykey;autoIncrement"`
	FileID        string    `json:"file_id" gorm:"type:varchar(128);not null;uniqueIndex"`
	SessionID     string    `json:"session_id" gorm:"type:varchar(64);index"`
	MessageID     string    `json:"message_id" gorm:"type:varchar(64);index"`
	DeviceID      string    `json:"device_id" gorm:"type:varchar(128);not null;index"`
	BucketName    string    `json:"bucket_name" gorm:"type:varchar(64);not null"`
	ObjectKey     string    `json:"object_key" gorm:"type:varchar(512);not null"`
	FileType      string    `json:"file_type" gorm:"type:varchar(20);not null"` // opus, wav, mp3, pcm
	FileSize      int64     `json:"file_size"`
	DurationMs    int       `json:"duration_ms"`
	SampleRate    int       `json:"sample_rate" gorm:"default:16000"`
	Channels      int       `json:"channels" gorm:"default:1"`
	SourceType    string    `json:"source_type" gorm:"type:varchar(20);not null"` // user, tts, asr
	Transcription string    `json:"transcription" gorm:"type:text"`               // ASR转写文本
	Metadata      JSONB     `json:"metadata" gorm:"type:jsonb;default:'{}'"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (AudioFile) TableName() string {
	return "audio_files"
}

// GetConversationModels 获取所有对话相关模型，用于数据库迁移
func GetConversationModels() []interface{} {
	return []interface{}{
		&ConversationSession{},
		&ConversationMessage{},
		&SystemPrompt{},
		&AudioFile{},
	}
}
