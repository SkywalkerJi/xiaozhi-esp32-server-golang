package minio

import (
	"fmt"
)

// Config MinIO配置
type Config struct {
	Endpoint        string `mapstructure:"endpoint" json:"endpoint"`
	AccessKeyID     string `mapstructure:"access_key_id" json:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key" json:"secret_access_key"`
	UseSSL          bool   `mapstructure:"use_ssl" json:"use_ssl"`
	BucketAudio     string `mapstructure:"bucket_audio" json:"bucket_audio"`
	Region          string `mapstructure:"region" json:"region"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Endpoint:        "localhost:9000",
		AccessKeyID:     "minioadmin",
		SecretAccessKey: "minioadmin123",
		UseSSL:          false,
		BucketAudio:     "xiaozhi-audio",
		Region:          "us-east-1",
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("MinIO endpoint is required")
	}
	if c.AccessKeyID == "" {
		return fmt.Errorf("MinIO access key ID is required")
	}
	if c.SecretAccessKey == "" {
		return fmt.Errorf("MinIO secret access key is required")
	}
	if c.BucketAudio == "" {
		return fmt.Errorf("MinIO audio bucket name is required")
	}
	return nil
}
