package snapshot_ops

import (
	"encoding/json"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const (
	maxConfigBytes             = 16 * 1024 * 1024
	maxConfigCipherBytes       = 32 * 1024 * 1024
	spaceReserve         int64 = 128 * 1024 * 1024
)

func requireSpace(dir string, needed int64) error {
	var fs unix.Statfs_t
	if e := unix.Statfs(dir, &fs); e != nil {
		return failure(503, "SNAPSHOT_SPACE_UNKNOWN", "无法核对恢复目录的可用空间")
	}
	if int64(fs.Bavail) <= 0 || fs.Bsize <= 0 {
		return checkSpace(0, needed)
	}
	return checkSpace(uint64(fs.Bavail)*uint64(fs.Bsize), needed)
}

func checkSpace(available uint64, needed int64) error {
	if needed < 0 || uint64(needed)+uint64(spaceReserve) > available {
		return failure(507, "SNAPSHOT_SPACE_INSUFFICIENT", "可用空间不足，需为任务内容和运行环境保留空间")
	}
	return nil
}

func packageSize(m Manifest) (int64, error) {
	// tar 头、对齐、清单及加密帧开销；超限包在写临时文件之前拒绝。
	size := int64(64*1024*1024) + int64(len(m.Files))*1024
	for _, file := range m.Files {
		if file.Size > maxPackageBytes-size {
			return 0, failure(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限")
		}
		size += file.Size
	}
	return size, nil
}

func readConfiguration(dir string, key []byte) (map[string][]byte, error) {
	file := filepath.Join(dir, "configuration", "runtime.enc")
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Size() > maxConfigCipherBytes {
		return nil, failure(422, "SNAPSHOT_CONFIG_INVALID", "配置密文缺失或超过大小上限")
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		return nil, e
	}
	plain, e := decryptConfig(raw, key)
	if e != nil {
		return nil, failure(422, "SNAPSHOT_CONFIG_INVALID", "配置密文无法解密")
	}
	var config map[string][]byte
	if json.Unmarshal(plain, &config) != nil || len(config["effective.yaml"]) == 0 || len(config) > 2048 {
		return nil, failure(422, "SNAPSHOT_CONFIG_INVALID", "生效配置缺失或格式无效")
	}
	total := 0
	for name, raw := range config {
		total += len(raw)
		if !cleanPath(name) || total > maxConfigBytes {
			return nil, failure(422, "SNAPSHOT_CONFIG_INVALID", "配置路径无效或总量超过大小上限")
		}
	}
	return config, nil
}

func supportsManagedRecovery(configuration map[string][]byte) bool {
	var contract struct {
		Protocol      int    `json:"protocol"`
		WritersFrozen bool   `json:"writers_frozen"`
		LocalOnly     bool   `json:"local_only,omitempty"`
		InstanceID    string `json:"instance_id"`
	}
	return json.Unmarshal(configuration["recovery/contract.json"], &contract) == nil && contract.Protocol == 1 && contract.WritersFrozen && !contract.LocalOnly && validID(contract.InstanceID)
}
