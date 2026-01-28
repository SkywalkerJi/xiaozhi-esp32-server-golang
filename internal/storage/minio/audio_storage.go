package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// AudioFileType 音频文件类型
type AudioFileType string

const (
	AudioTypeOpus AudioFileType = "opus"
	AudioTypeWav  AudioFileType = "wav"
	AudioTypeMp3  AudioFileType = "mp3"
	AudioTypePcm  AudioFileType = "pcm"
)

// AudioSourceType 音频来源类型
type AudioSourceType string

const (
	AudioSourceUser AudioSourceType = "user" // 用户输入音频
	AudioSourceTTS  AudioSourceType = "tts"  // TTS 输出音频
	AudioSourceASR  AudioSourceType = "asr"  // ASR 音频
)

// AudioMetadata 音频文件元数据
type AudioMetadata struct {
	FileID     string          `json:"file_id"`
	SessionID  string          `json:"session_id"`
	MessageID  string          `json:"message_id"`
	DeviceID   string          `json:"device_id"`
	BucketName string          `json:"bucket_name"`
	ObjectKey  string          `json:"object_key"`
	FileType   AudioFileType   `json:"file_type"`
	FileSize   int64           `json:"file_size"`
	DurationMs int             `json:"duration_ms"`
	SampleRate int             `json:"sample_rate"`
	Channels   int             `json:"channels"`
	SourceType AudioSourceType `json:"source_type"`
	CreatedAt  time.Time       `json:"created_at"`
}

// AudioStorage 音频存储服务
type AudioStorage struct {
	client     *Client
	bucketName string
}

// NewAudioStorage 创建音频存储服务
func NewAudioStorage(client *Client) (*AudioStorage, error) {
	bucketName := client.GetConfig().BucketAudio

	// 确保bucket存在
	ctx := context.Background()
	if err := client.EnsureBucket(ctx, bucketName); err != nil {
		return nil, fmt.Errorf("failed to ensure bucket: %w", err)
	}

	return &AudioStorage{
		client:     client,
		bucketName: bucketName,
	}, nil
}

// generateObjectKey 生成对象存储key
// 格式: {device_id}/{date}/{session_id}/{file_id}.{ext}
func (s *AudioStorage) generateObjectKey(deviceID, sessionID, fileID string, fileType AudioFileType) string {
	date := time.Now().Format("2006-01-02")
	return path.Join(deviceID, date, sessionID, fmt.Sprintf("%s.%s", fileID, fileType))
}

// UploadAudio 上传音频文件
func (s *AudioStorage) UploadAudio(ctx context.Context, params UploadParams) (*AudioMetadata, error) {
	fileID := uuid.New().String()
	objectKey := s.generateObjectKey(params.DeviceID, params.SessionID, fileID, params.FileType)

	contentType := s.getContentType(params.FileType)

	// 上传到MinIO
	info, err := s.client.GetMinioClient().PutObject(ctx, s.bucketName, objectKey, bytes.NewReader(params.Data), int64(len(params.Data)), minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"device_id":   params.DeviceID,
			"session_id":  params.SessionID,
			"message_id":  params.MessageID,
			"source_type": string(params.SourceType),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload audio: %w", err)
	}

	metadata := &AudioMetadata{
		FileID:     fileID,
		SessionID:  params.SessionID,
		MessageID:  params.MessageID,
		DeviceID:   params.DeviceID,
		BucketName: s.bucketName,
		ObjectKey:  objectKey,
		FileType:   params.FileType,
		FileSize:   info.Size,
		DurationMs: params.DurationMs,
		SampleRate: params.SampleRate,
		Channels:   params.Channels,
		SourceType: params.SourceType,
		CreatedAt:  time.Now(),
	}

	return metadata, nil
}

// UploadParams 上传参数
type UploadParams struct {
	DeviceID   string
	SessionID  string
	MessageID  string
	Data       []byte
	FileType   AudioFileType
	SourceType AudioSourceType
	DurationMs int
	SampleRate int
	Channels   int
}

// DownloadAudio 下载音频文件
func (s *AudioStorage) DownloadAudio(ctx context.Context, objectKey string) ([]byte, error) {
	obj, err := s.client.GetMinioClient().GetObject(ctx, s.bucketName, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to read object: %w", err)
	}

	return data, nil
}

// DeleteAudio 删除音频文件
func (s *AudioStorage) DeleteAudio(ctx context.Context, objectKey string) error {
	err := s.client.GetMinioClient().RemoveObject(ctx, s.bucketName, objectKey, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// GetPresignedURL 获取预签名URL（用于临时访问）
func (s *AudioStorage) GetPresignedURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	url, err := s.client.GetMinioClient().PresignedGetObject(ctx, s.bucketName, objectKey, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return url.String(), nil
}

// ListAudioBySession 列出会话的所有音频文件
func (s *AudioStorage) ListAudioBySession(ctx context.Context, deviceID, sessionID string) ([]string, error) {
	prefix := path.Join(deviceID, "", sessionID) + "/"

	var objectKeys []string
	objectCh := s.client.GetMinioClient().ListObjects(ctx, s.bucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", object.Err)
		}
		objectKeys = append(objectKeys, object.Key)
	}

	return objectKeys, nil
}

// getContentType 根据文件类型获取MIME类型
func (s *AudioStorage) getContentType(fileType AudioFileType) string {
	switch fileType {
	case AudioTypeOpus:
		return "audio/opus"
	case AudioTypeWav:
		return "audio/wav"
	case AudioTypeMp3:
		return "audio/mpeg"
	case AudioTypePcm:
		return "audio/pcm"
	default:
		return "application/octet-stream"
	}
}
