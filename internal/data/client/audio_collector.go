package client

import (
	"sync"
	"xiaozhi-esp32-server-golang/internal/domain/eventbus"
)

// AudioCollector 音频收集器，用于收集用户输入和 TTS 输出的音频
type AudioCollector struct {
	mu sync.Mutex

	// 用户输入音频（opus 格式）
	userAudioData []byte
	userEnabled   bool

	// TTS 输出音频（opus 格式）
	ttsAudioData []byte
	ttsEnabled   bool

	// 设备和会话信息
	deviceID  string
	sessionID string
}

// NewAudioCollector 创建新的音频收集器
func NewAudioCollector(deviceID, sessionID string) *AudioCollector {
	return &AudioCollector{
		deviceID:      deviceID,
		sessionID:     sessionID,
		userAudioData: make([]byte, 0),
		ttsAudioData:  make([]byte, 0),
		userEnabled:   true,
		ttsEnabled:    true,
	}
}

// SetEnabled 设置是否启用音频收集
func (c *AudioCollector) SetEnabled(userEnabled, ttsEnabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userEnabled = userEnabled
	c.ttsEnabled = ttsEnabled
}

// AddUserAudio 添加用户输入音频数据
func (c *AudioCollector) AddUserAudio(data []byte) {
	if !c.userEnabled || len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userAudioData = append(c.userAudioData, data...)
}

// AddTTSAudio 添加 TTS 输出音频数据
func (c *AudioCollector) AddTTSAudio(data []byte) {
	if !c.ttsEnabled || len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ttsAudioData = append(c.ttsAudioData, data...)
}

// GetUserAudio 获取并清空用户音频数据
func (c *AudioCollector) GetUserAudio() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.userAudioData
	c.userAudioData = make([]byte, 0)
	return data
}

// GetTTSAudio 获取并清空 TTS 音频数据
func (c *AudioCollector) GetTTSAudio() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.ttsAudioData
	c.ttsAudioData = make([]byte, 0)
	return data
}

// SaveUserAudio 保存用户音频并发布事件
func (c *AudioCollector) SaveUserAudio(messageID string, sampleRate, channels int) {
	data := c.GetUserAudio()
	if len(data) == 0 {
		return
	}

	eventbus.Get().Publish(eventbus.TopicSaveAudio, eventbus.AudioSaveEvent{
		DeviceID:   c.deviceID,
		SessionID:  c.sessionID,
		MessageID:  messageID,
		AudioData:  data,
		AudioType:  "opus",
		SourceType: "user",
		SampleRate: sampleRate,
		Channels:   channels,
	})
}

// SaveTTSAudio 保存 TTS 音频并发布事件
func (c *AudioCollector) SaveTTSAudio(messageID string, sampleRate, channels int) {
	data := c.GetTTSAudio()
	if len(data) == 0 {
		return
	}

	eventbus.Get().Publish(eventbus.TopicSaveAudio, eventbus.AudioSaveEvent{
		DeviceID:   c.deviceID,
		SessionID:  c.sessionID,
		MessageID:  messageID,
		AudioData:  data,
		AudioType:  "opus",
		SourceType: "tts",
		SampleRate: sampleRate,
		Channels:   channels,
	})
}

// Clear 清空所有收集的音频
func (c *AudioCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userAudioData = make([]byte, 0)
	c.ttsAudioData = make([]byte, 0)
}

// GetUserAudioSize 获取用户音频数据大小
func (c *AudioCollector) GetUserAudioSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.userAudioData)
}

// GetTTSAudioSize 获取 TTS 音频数据大小
func (c *AudioCollector) GetTTSAudioSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.ttsAudioData)
}
