package eventbus

const (
	TopicAddMessage = "add_message"
	TopicSessionEnd = "session_end"
	TopicSaveAudio  = "save_audio" // 保存音频到 MinIO
)

// AudioSaveEvent 音频保存事件
type AudioSaveEvent struct {
	DeviceID   string
	SessionID  string
	MessageID  string
	AudioData  []byte
	AudioType  string // opus, wav, pcm
	SourceType string // user, tts, asr
	SampleRate int
	Channels   int
}
