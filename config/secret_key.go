// config/secret_key.go
package config

import (
	"encoding/base64"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
)

// ParseKey 解析并校验 server.secret_key（Base64，解码后必须 32 字节）。
// 成功返回解码后的 32 字节密钥；失败返回错误。
func (s *ServerConfig) ParseKey() ([]byte, error) {
	if s.SecretKey == "" {
		return nil, fmt.Errorf("server.secret_key 不能为空（需要 Base64 编码的 32 字节）")
	}

	key, err := base64.StdEncoding.DecodeString(s.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("server.secret_key 不是合法的 Base64：%w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("server.secret_key 解码后需为 32 字节，当前=%d", len(key))
	}

	// 注入到 crypto 全局（tenant_rsa.go 已去掉环境变量读取，优先使用全局）
	if err = crypto.SetGlobalKeyB64(s.SecretKey); err != nil {
		return nil, fmt.Errorf("设置全局主密钥失败: %w", err)
	}

	return key, nil
}
