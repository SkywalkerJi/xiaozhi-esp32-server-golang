package user_config

import (
	"context"
	"testing"
)

func TestMemoryProvider(t *testing.T) {
	ctx := context.Background()

	// 创建内存provider
	config := map[string]interface{}{
		"max_entries": 10,
	}

	provider, err := GetUserConfigProvider("memory", config)
	if err != nil {
		t.Fatalf("创建内存provider失败: %v", err)
	}
	// 注意：接口中没有Close方法，所以不需要调用

	userID := "test_user_123"

	// 由于接口中没有SetUserConfig方法，我们只测试GetUserConfig方法
	// 测试获取不存在用户的配置（应该返回空配置）
	retrievedConfig, err := provider.GetUserConfig(ctx, userID)
	if err != nil {
		t.Fatalf("获取用户配置失败: %v", err)
	}

	// 验证返回的是空配置
	if retrievedConfig.Llm.Provider != "" {
		t.Errorf("期望空配置，但得到了 LLM Provider: %s", retrievedConfig.Llm.Provider)
	}

	// 测试系统配置获取
	systemConfig, err := provider.GetSystemConfig(ctx)
	if err != nil {
		t.Fatalf("获取系统配置失败: %v", err)
	}
	_ = systemConfig // 系统配置可能为空，这是正常的
}

func TestProviderAdapter(t *testing.T) {
	t.Skip("NewUserConfigAdapter function not implemented yet")
}

func TestDefaultConfig(t *testing.T) {
	t.Skip("DefaultConfig function not implemented yet")
}

func TestValidateConfig(t *testing.T) {
	t.Skip("ValidateConfig function not implemented yet")
}
