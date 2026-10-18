package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SnapshotConfig 只配置系统快照存储；留空沿用安装根目录或用户目录默认值。
// 不接受浏览器请求写入服务器路径；独立协调器的 snapshot_directory 优先。
type SnapshotConfig struct {
	Directory       string `yaml:"directory,omitempty" json:"directory,omitempty"`
	directorySource string
}

func (c SnapshotConfig) Validate() error {
	if c.Directory != "" && (!filepath.IsAbs(c.Directory) || strings.ContainsRune(c.Directory, 0)) {
		return fmt.Errorf("system_snapshots.directory 必须是绝对路径，留空使用默认目录")
	}
	return nil
}

// DirectorySource 说明已加载的配置来源；不保存到恢复包的生效 YAML 中。
func (c SnapshotConfig) DirectorySource() string {
	if c.directorySource != "" {
		return c.directorySource
	}
	return "configuration"
}
