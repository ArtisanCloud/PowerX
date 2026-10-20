package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"syscall"

	"gopkg.in/yaml.v3"
)

// SecretInitialization 仅返回来源状态，绝不返回密钥内容。
type SecretInitialization struct {
	JWT    string `json:"jwt"`
	Server string `json:"server,omitempty"`
}

// PrepareInstallationSecrets 是离线首次初始化入口，已有有效密钥保持不变。
func PrepareInstallationSecrets(path string) (SecretInitialization, error) {
	return prepareInstallationSecrets(path, true)
}

func prepareInstallationSecrets(path string, includeServer bool) (SecretInitialization, error) {
	result := SecretInitialization{JWT: "existing"}
	setupJWTSecretMu.Lock()
	defer setupJWTSecretMu.Unlock()
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return result, fmt.Errorf("请提供可读写的实际 config.yaml（先从示例复制），配置未修改")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return result, fmt.Errorf("运行配置必须是普通文件")
	}
	// Setup 与不同 CLI 进程共享同一文件锁，避免首次并发生成不同密钥。
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return result, fmt.Errorf("无法锁定运行配置")
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	raw, err := io.ReadAll(f)
	if err != nil {
		return result, fmt.Errorf("无法读取运行配置")
	}
	var document yaml.Node
	var state struct {
		Install InstallConfig `yaml:"install"`
		Auth    AuthConfig    `yaml:"auth"`
		Server  ServerConfig  `yaml:"server"`
	}
	if yaml.Unmarshal(raw, &document) != nil || document.Decode(&state) != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return result, fmt.Errorf("运行配置 YAML 无效，配置未修改")
	}
	lookup, err := configurationEnvironment(path)
	if err != nil {
		return result, err
	}
	override, err := jwtSecretEnvironmentFrom(lookup)
	if err != nil {
		return result, err
	}
	needsJWT := validateJWTSecret(state.Auth.JWTSecret) != nil
	if override != "" {
		if err = validateJWTSecret(override); err != nil {
			return result, fmt.Errorf("JWT 环境配置无效: %w", err)
		}
		needsJWT = false
		result.JWT = "environment"
	}
	needsServer := includeServer && state.Server.SecretKey == ""
	if includeServer {
		result.Server = "existing"
		if !needsServer {
			key, decodeErr := base64.StdEncoding.DecodeString(state.Server.SecretKey)
			if decodeErr != nil || len(key) != 32 {
				return result, fmt.Errorf("已有 server.secret_key 格式无效，未覆盖；需要 32 字节 Base64 密钥")
			}
		}
	}
	if !needsJWT && !needsServer {
		return result, nil
	}
	if state.Install.EffectiveStatus() == "installed" {
		return result, fmt.Errorf("已安装实例缺少有效密钥，拒绝自动生成或轮换；请恢复该实例原密钥后重试")
	}
	if needsJWT {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return result, fmt.Errorf("无法生成 JWT 密钥")
		}
		setSecretNode(document.Content[0], "auth", "jwt_secret", hex.EncodeToString(key))
		result.JWT = "generated"
	}
	if needsServer {
		key := make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return result, fmt.Errorf("无法生成 server.secret_key")
		}
		setSecretNode(document.Content[0], "server", "secret_key", base64.StdEncoding.EncodeToString(key))
		result.Server = "generated"
	}
	data, err := yaml.Marshal(&document)
	if err != nil {
		return result, fmt.Errorf("无法保存密钥配置")
	}
	// Setup 保留 owner/mode/ACL；手动首装由配置所有者执行并收紧权限。
	if includeServer {
		if err = f.Chmod(0600); err != nil {
			return result, fmt.Errorf("无法保护运行配置权限，请使用配置所有者执行")
		}
	}
	if _, err = f.WriteAt(data, 0); err == nil {
		err = f.Truncate(int64(len(data)))
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		return result, fmt.Errorf("无法写入运行配置，请核对文件及磁盘状态")
	}
	return result, nil
}

func setSecretNode(root *yaml.Node, section, field, value string) {
	find := func(mapping *yaml.Node, name string) *yaml.Node {
		for i := 0; i < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == name {
				return mapping.Content[i+1]
			}
		}
		return nil
	}
	group := find(root, section)
	if group == nil || group.Kind != yaml.MappingNode {
		if group == nil {
			group = &yaml.Node{}
			root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: section}, group)
		}
		*group = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	node := find(group, field)
	if node == nil {
		node = &yaml.Node{}
		group.Content = append(group.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: field}, node)
	}
	node.Kind, node.Tag, node.Value = yaml.ScalarNode, "!!str", value
}
