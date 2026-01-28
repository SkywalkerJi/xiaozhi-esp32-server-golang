package minio

import (
	"context"
	"fmt"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/sirupsen/logrus"
)

var (
	client     *Client
	clientOnce sync.Once
	clientErr  error
)

// Client MinIO客户端封装
type Client struct {
	minioClient *minio.Client
	config      *Config
	logger      *logrus.Logger
}

// NewClient 创建新的MinIO客户端
func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	minioClient, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
		Region: config.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	c := &Client{
		minioClient: minioClient,
		config:      config,
		logger:      logrus.New(),
	}

	return c, nil
}

// GetInstance 获取单例客户端
func GetInstance(config *Config) (*Client, error) {
	clientOnce.Do(func() {
		client, clientErr = NewClient(config)
	})
	return client, clientErr
}

// EnsureBucket 确保bucket存在
func (c *Client) EnsureBucket(ctx context.Context, bucketName string) error {
	exists, err := c.minioClient.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = c.minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{
			Region: c.config.Region,
		})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
		c.logger.Infof("Bucket %s created successfully", bucketName)
	}

	return nil
}

// GetMinioClient 获取底层MinIO客户端
func (c *Client) GetMinioClient() *minio.Client {
	return c.minioClient
}

// GetConfig 获取配置
func (c *Client) GetConfig() *Config {
	return c.config
}

// HealthCheck 健康检查
func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.minioClient.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("MinIO health check failed: %w", err)
	}
	return nil
}
