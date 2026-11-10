package storagebudget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// documentNameLocked 将安装级运行文档与同盘共享预算分开。旧目录的归属
// 优先取已有执行绑定，保留原文件名；其他安装使用完整 scope 摘要隔离。
// 必须在主机锁内调用，归属一经写入不可被另一个安装抢占。
func (c *Coordinator) documentNameLocked(name string) (string, error) {
	sum := sha256.Sum256([]byte(c.cfg.HostID + "\x00" + c.cfg.FilesystemID + "\x00" + c.cfg.InstanceID))
	scope := hex.EncodeToString(sum[:])
	var owner struct {
		Version int    `json:"version"`
		Scope   string `json:"scope"`
	}
	raw, err := c.readPrivate("document-owner.json", 4096)
	if errors.Is(err, os.ErrNotExist) {
		owner.Version, owner.Scope = 1, scope
		// 旧安装的控制绑定已经含有不可混用的 scope，不能按启动顺序覆盖。
		var previousScope string
		for _, binding := range []string{"execution-control-binding", "database-retention-binding", "system-retention-probe-binding", "system-retention-discovery-binding"} {
			old, e := c.readPrivate(binding+".json", 4096)
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			var b struct {
				Scope string `json:"scope"`
			}
			if e != nil || json.Unmarshal(old, &b) != nil || len(b.Scope) != 64 {
				return "", ErrUnavailable
			}
			if previousScope != "" && previousScope != b.Scope {
				return "", ErrUnavailable
			}
			owner.Scope = b.Scope
			previousScope = b.Scope
		}
		if previousScope == "" {
			fd, e := c.openDirectory()
			if e != nil {
				return "", e
			}
			dir := os.NewFile(uintptr(fd), "private-storage-directory")
			names, e := dir.Readdirnames(-1)
			dir.Close()
			if e != nil {
				return "", ErrUnavailable
			}
			for _, existing := range names {
				if strings.HasPrefix(existing, "instance-") {
					return "", ErrUnavailable
				}
			}
		}
		raw, _ = json.Marshal(owner)
		if e := c.writePrivate("document-owner.json", raw); e != nil {
			return "", e
		}
	} else if err != nil || json.Unmarshal(raw, &owner) != nil || owner.Version != 1 || len(owner.Scope) != 64 {
		return "", ErrUnavailable
	}
	if _, e := hex.DecodeString(owner.Scope); e != nil {
		return "", ErrUnavailable
	}
	if owner.Scope == scope {
		return name + ".json", nil
	}
	return "instance-" + scope + "-" + name + ".json", nil
}
