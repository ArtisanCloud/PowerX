// Package runtimecredential 保存宿主管理的插件 STS 凭证；文件不随插件版本目录替换。
package runtimecredential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Credential struct {
	TenantUUID   string `json:"tenant_uuid"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Path 将凭证放在插件根目录，避免 Force 删除版本目录时一并丢失。
func Path(installedRoot, pluginID string) (string, error) {
	if strings.TrimSpace(installedRoot) == "" || !pluginIDPattern.MatchString(pluginID) || pluginID == "." || pluginID == ".." {
		return "", fmt.Errorf("runtime credential requires installed root and valid plugin_id")
	}
	return filepath.Join(installedRoot, pluginID, ".runtime-credentials.json"), nil
}

func Read(path string) (*Credential, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cred Credential
	if err := json.Unmarshal(raw, &cred); err != nil {
		return nil, fmt.Errorf("runtime credential file is invalid JSON")
	}
	return &cred, nil
}

func Write(path string, cred Credential) error {
	if strings.TrimSpace(cred.TenantUUID) == "" || strings.TrimSpace(cred.ClientID) == "" || strings.TrimSpace(cred.ClientSecret) == "" {
		return fmt.Errorf("runtime credential fields are incomplete")
	}
	raw, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return AtomicWrite(path, raw, 0o600)
}

// AtomicWrite 写入同目录临时文件后替换，避免留下半写入配置；secret 文件仅属主可读。
func AtomicWrite(path string, raw []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refuse to replace non-regular file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".powerx-credential-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
