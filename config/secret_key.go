package config

import (
	"encoding/base64"
	"fmt"
	"os"
)

type WrapConfig struct {
	MasterKeyID   string `yaml:"master_key_id"`   // 例如 v1
	MasterKeyB64  string `yaml:"master_key_b64"`  // 可选：Base64（解码后 32 字节）
	MasterKeyFile string `yaml:"master_key_file"` // 可选：指向一个二进制 32 字节文件
}

type WrapKey struct {
	ID  string
	Key []byte // 32 bytes
}

func (c *Config) ParseWrapKey() (*WrapKey, error) {
	if c.Wrap.MasterKeyID == "" {
		return nil, fmt.Errorf("wrap.master_key_id 不能为空")
	}
	var raw []byte
	switch {
	case c.Wrap.MasterKeyB64 != "":
		dec, err := base64.StdEncoding.DecodeString(c.Wrap.MasterKeyB64)
		if err != nil {
			return nil, fmt.Errorf("wrap.master_key_b64 非法: %w", err)
		}
		raw = dec
	case c.Wrap.MasterKeyFile != "":
		b, err := os.ReadFile(c.Wrap.MasterKeyFile)
		if err != nil {
			return nil, fmt.Errorf("读取 wrap.master_key_file 失败: %w", err)
		}
		raw = b
	default:
		return nil, fmt.Errorf("wrap.master_key_b64 与 wrap.master_key_file 至少提供一个")
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("主密钥长度须为 32 字节，当前=%d", len(raw))
	}
	return &WrapKey{ID: c.Wrap.MasterKeyID, Key: raw}, nil
}
