package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// loadFromYAML 从YAML文件加载配置
func loadFromYAML(cfg *Config, configPath string) error {
	// 检查文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("提示：配置文件 %s 不存在，使用默认配置\n", configPath)
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

	fmt.Printf("✅ 成功加载配置文件: %s\n", configPath)
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

	fmt.Printf("✅ 配置已保存到: %s\n", configPath)
	return nil
}
