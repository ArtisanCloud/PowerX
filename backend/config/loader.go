package config

import (
	"context"
	"fmt"
	"os"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gopkg.in/yaml.v3"
)

// loadFromYAML 从YAML文件加载配置
func loadFromYAML(cfg *Config, configPath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logger.InfoF(context.Background(), "提示：配置文件 %s 不存在，使用默认配置", configPath)
		return nil
	}

	// 读取文件内容
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析YAML
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("解析YAML配置失败: %w", err)
	}

	logger.InfoF(context.Background(), "✅ 成功加载配置文件: %s", configPath)
	return nil
}

// SaveToYAML 将配置保存到YAML文件
func SaveToYAML(cfg *Config, configPath string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	logger.InfoF(context.Background(), "✅ 配置已保存到: %s", configPath)
	return nil
}
