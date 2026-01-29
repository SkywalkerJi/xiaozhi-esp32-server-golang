package server

import (
	"context"
	"fmt"
	. "xiaozhi-esp32-server-golang/internal/data/client"
	"xiaozhi-esp32-server-golang/internal/domain/eventbus"
	"xiaozhi-esp32-server-golang/internal/domain/memory/llm_memory"
	"xiaozhi-esp32-server-golang/internal/storage/minio"
	workpool "xiaozhi-esp32-server-golang/internal/util/work"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/cloudwego/eino/schema"
	"github.com/spf13/viper"
)

type EventHandle struct {
	audioStorage *minio.AudioStorage
}

func NewEventHandle() *EventHandle {
	return &EventHandle{}
}

func (s *EventHandle) Start() error {
	// 初始化 MinIO 音频存储（如果配置了）
	if err := s.initAudioStorage(); err != nil {
		log.Warnf("MinIO 音频存储初始化失败，音频将不会保存: %v", err)
	}

	go s.HandleAddMessage()
	go s.HandleSessionEnd()
	go s.HandleSaveAudio()
	return nil
}

// initAudioStorage 初始化 MinIO 音频存储
func (s *EventHandle) initAudioStorage() error {
	// 检查是否配置了 MinIO
	endpoint := viper.GetString("minio.endpoint")
	if endpoint == "" {
		return fmt.Errorf("MinIO endpoint not configured")
	}

	config := &minio.Config{
		Endpoint:        endpoint,
		AccessKeyID:     viper.GetString("minio.access_key_id"),
		SecretAccessKey: viper.GetString("minio.secret_access_key"),
		UseSSL:          viper.GetBool("minio.use_ssl"),
		BucketAudio:     viper.GetString("minio.bucket_audio"),
		Region:          viper.GetString("minio.region"),
	}

	if config.BucketAudio == "" {
		config.BucketAudio = "xiaozhi-audio"
	}
	if config.Region == "" {
		config.Region = "us-east-1"
	}

	client, err := minio.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create MinIO client: %w", err)
	}

	audioStorage, err := minio.NewAudioStorage(client)
	if err != nil {
		return fmt.Errorf("failed to create audio storage: %w", err)
	}

	s.audioStorage = audioStorage
	log.Infof("MinIO 音频存储初始化成功, endpoint: %s, bucket: %s", endpoint, config.BucketAudio)
	return nil
}

func (s *EventHandle) HandleAddMessage() {
	type AddMessageJob struct {
		clientState *ClientState
		Msg         schema.Message
	}
	f := func(job workpool.Job) error {
		addMessageJob, ok := job.(AddMessageJob)
		if !ok {
			return fmt.Errorf("invalid job info")
		}
		clientState := addMessageJob.clientState
		//添加到 消息列表中
		llm_memory.Get().AddMessage(clientState.Ctx, clientState.DeviceID, clientState.AgentID, addMessageJob.Msg)
		//将消息加到 长期记忆体中
		if clientState.MemoryProvider != nil {
			err := clientState.MemoryProvider.AddMessage(clientState.Ctx, clientState.GetDeviceIDOrAgentID(), addMessageJob.Msg)
			if err != nil {
				return fmt.Errorf("add message to memory provider failed: %w", err)
			}
		}
		return nil
	}
	workPool := workpool.NewWorkPool(10, 1000, workpool.JobHandler(f))
	eventbus.Get().Subscribe(eventbus.TopicAddMessage, func(clientState *ClientState, msg schema.Message) {
		workPool.Submit(AddMessageJob{
			clientState: clientState,
			Msg:         msg,
		})
	})
}

func (s *EventHandle) HandleSessionEnd() error {
	f := func(job workpool.Job) error {
		clientState, ok := job.(*ClientState)
		if !ok {
			return fmt.Errorf("invalid job info")
		}

		//将消息加到 长期记忆体中
		if clientState.MemoryProvider != nil {
			err := clientState.MemoryProvider.Flush(clientState.Ctx, clientState.GetDeviceIDOrAgentID())
			if err != nil {
				return fmt.Errorf("add message to memory provider failed: %w", err)
			}
		}
		return nil
	}
	workPool := workpool.NewWorkPool(10, 1000, workpool.JobHandler(f))
	eventbus.Get().Subscribe(eventbus.TopicSessionEnd, func(clientState *ClientState) {
		if clientState == nil {
			log.Warnf("HandleSessionEnd: clientState is nil, skipping")
			return
		}
		if clientState.MemoryProvider == nil {
			return
		}
		log.Infof("HandleSessionEnd: deviceId: %s", clientState.DeviceID)
		workPool.Submit(clientState)
	})
	return nil
}

// HandleSaveAudio 处理音频保存事件
func (s *EventHandle) HandleSaveAudio() {
	if s.audioStorage == nil {
		log.Warnf("HandleSaveAudio: audioStorage is nil, audio saving disabled")
		return
	}

	f := func(job workpool.Job) error {
		event, ok := job.(eventbus.AudioSaveEvent)
		if !ok {
			return fmt.Errorf("invalid job info")
		}

		// 转换音频类型
		var audioType minio.AudioFileType
		switch event.AudioType {
		case "opus":
			audioType = minio.AudioTypeOpus
		case "wav":
			audioType = minio.AudioTypeWav
		case "mp3":
			audioType = minio.AudioTypeMp3
		case "pcm":
			audioType = minio.AudioTypePcm
		default:
			audioType = minio.AudioTypeOpus
		}

		// 转换来源类型
		var sourceType minio.AudioSourceType
		switch event.SourceType {
		case "user":
			sourceType = minio.AudioSourceUser
		case "tts":
			sourceType = minio.AudioSourceTTS
		case "asr":
			sourceType = minio.AudioSourceASR
		default:
			sourceType = minio.AudioSourceUser
		}

		// 上传到 MinIO
		metadata, err := s.audioStorage.UploadAudio(context.Background(), minio.UploadParams{
			DeviceID:   event.DeviceID,
			SessionID:  event.SessionID,
			MessageID:  event.MessageID,
			Data:       event.AudioData,
			FileType:   audioType,
			SourceType: sourceType,
			SampleRate: event.SampleRate,
			Channels:   event.Channels,
		})
		if err != nil {
			return fmt.Errorf("failed to upload audio: %w", err)
		}

		log.Infof("音频保存成功: device=%s, session=%s, fileId=%s, size=%d",
			event.DeviceID, event.SessionID, metadata.FileID, metadata.FileSize)
		return nil
	}

	workPool := workpool.NewWorkPool(5, 500, workpool.JobHandler(f))
	eventbus.Get().Subscribe(eventbus.TopicSaveAudio, func(event eventbus.AudioSaveEvent) {
		if len(event.AudioData) == 0 {
			return
		}
		workPool.Submit(event)
	})
}
