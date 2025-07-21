package chat

import (
	"context"
	"sync"

	"github.com/spf13/viper"

	"xiaozhi-esp32-server-golang/constants"
	types_conn "xiaozhi-esp32-server-golang/internal/app/server/types"
	types_audio "xiaozhi-esp32-server-golang/internal/data/audio"
	"xiaozhi-esp32-server-golang/internal/data/client"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	userconfig "xiaozhi-esp32-server-golang/internal/domain/config"
	"xiaozhi-esp32-server-golang/internal/domain/vad/silero_vad"
	"xiaozhi-esp32-server-golang/internal/util"
	log "xiaozhi-esp32-server-golang/logger"
)

type ChatManager struct {
	DeviceID  string
	transport types_conn.IConn

	clientState *ClientState
	session     *ChatSession
	ctx         context.Context
	cancel      context.CancelFunc
}

type ChatManagerOption func(*ChatManager)

func NewChatManager(deviceID string, transport types_conn.IConn, options ...ChatManagerOption) (*ChatManager, error) {
	cm := &ChatManager{
		DeviceID:  deviceID,
		transport: transport,
	}

	for _, option := range options {
		option(cm)
	}

	ctx := context.WithValue(context.Background(), "chat_session_operator", ChatSessionOperator(cm))

	cm.ctx, cm.cancel = context.WithCancel(ctx)

	cm.transport.OnClose(cm.OnClose)

	clientState, err := GenClientState(cm.ctx, cm.DeviceID, cm.transport)
	if err != nil {
		log.Errorf("初始化客户端状态失败: %v", err)
		return nil, err
	}
	cm.clientState = clientState

	serverTransport := NewServerTransport(cm.transport, clientState)

	cm.session = NewChatSession(
		clientState,
		serverTransport,
	)

	return cm, nil
}

func GenClientState(pctx context.Context, deviceID string, transport types_conn.IConn) (*ClientState, error) {
	configProvider, err := userconfig.GetProvider()
	if err != nil {
		log.Errorf("获取 用户配置提供者失败: %+v", err)
		return nil, err
	}
	deviceConfig, err := configProvider.GetUserConfig(pctx, deviceID)
	if err != nil {
		log.Errorf("获取 设备 %s 配置失败: %+v", deviceID, err)
		return nil, err
	}

	if deviceConfig.Vad.Provider == "silero_vad" {
		silero_vad.InitVadPool(deviceConfig.Vad.Config)
	}

	// 创建带取消功能的上下文
	ctx, cancel := context.WithCancel(pctx)

	maxSilenceDuration := viper.GetInt64("chat.chat_max_silence_duration")
	if maxSilenceDuration == 0 {
		maxSilenceDuration = 200
	}

	clientState := &ClientState{
		Dialogue:     &Dialogue{},
		Abort:        false,
		ListenMode:   "auto",
		DeviceID:     deviceID,
		Ctx:          ctx,
		Cancel:       cancel,
		SystemPrompt: deviceConfig.SystemPrompt,
		DeviceConfig: deviceConfig,
		OutputAudioFormat: types_audio.AudioFormat{
			SampleRate:    types_audio.SampleRate,
			Channels:      types_audio.Channels,
			FrameDuration: types_audio.FrameDuration,
			Format:        types_audio.Format,
		},
		OpusAudioBuffer: make(chan []byte, 100),
		AsrAudioBuffer: &AsrAudioBuffer{
			PcmData:          make([]float32, 0),
			AudioBufferMutex: sync.RWMutex{},
			PcmFrameSize:     0,
		},
		VoiceStatus: VoiceStatus{
			HaveVoice:            false,
			HaveVoiceLastTime:    0,
			VoiceStop:            false,
			SilenceThresholdTime: maxSilenceDuration,
		},
		SessionCtx:   Ctx{},
		LocationInfo: &LocationInfo{}, // 初始化位置信息
	}

	ttsType := clientState.DeviceConfig.Tts.Provider
	//如果使用 xiaozhi tts，则固定使用24000hz, 20ms帧长
	if ttsType == constants.TtsTypeXiaozhi || ttsType == constants.TtsTypeEdgeOffline {
		clientState.OutputAudioFormat.SampleRate = 24000
		clientState.OutputAudioFormat.FrameDuration = 20
	}

	// 异步获取位置信息
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("获取位置信息时发生错误: %v", r)
			}
		}()

		clientIP := transport.GetIP()
		log.Debugf("开始获取设备 %s 的位置信息，IP: %s", deviceID, clientIP)

		amapAPI := util.NewAmapAPI()
		locationInfo, err := amapAPI.GetLocationByIP(ctx, clientIP)
		if err != nil {
			log.Warnf("获取设备 %s 位置信息失败: %v", deviceID, err)
			// 设置默认值
			clientState.LocationInfo = &client.LocationInfo{IP: clientIP}
		} else {
			clientState.LocationInfo = &client.LocationInfo{
				IP:       clientIP,
				Province: locationInfo.Province,
				City:     locationInfo.City,
				District: locationInfo.District,
				Address:  "未知位置",
			}
			log.Infof("设备 %s 位置信息获取成功: %s", deviceID, locationInfo.Address)
		}
	}()

	return clientState, nil
}

func (c *ChatManager) Start() error {
	return c.session.Start(c.ctx)
}

// 主动关闭断开连接
func (c *ChatManager) Close() error {
	log.Infof("主动关闭断开连接, 设备 %s", c.clientState.DeviceID)

	// 先关闭会话级别的资源
	if c.session != nil {
		c.session.Close()
	}

	// 最后取消管理器级别的上下文
	c.cancel()

	return nil
}

func (c *ChatManager) OnClose(deviceId string) {
	log.Infof("设备 %s 断开连接", deviceId)
	c.cancel()
	return
}

func (c *ChatManager) GetClientState() *ClientState {
	return c.clientState
}

func (c *ChatManager) GetDeviceId() string {
	return c.clientState.DeviceID
}
