package snapshot_ops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func failure(status int, code, message string) error {
	return dto.NewErrorWithCode(status, code, message, nil)
}
func validID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
func cleanPath(p string) bool {
	return p != "" && len(p) < 2048 && !strings.ContainsAny(p, "\\\x00") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func atomicJSON(file string, v any) error {
	_, e := atomicJSONPublished(file, v)
	return e
}

// atomicJSONPublished 区分发布前失败和 rename 已完成后的目录同步失败。
func atomicJSONPublished(file string, v any) (bool, error) {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return false, e
	}
	return atomicFilePublished(file, b)
}
func atomicFile(file string, b []byte) error {
	_, e := atomicFilePublished(file, b)
	return e
}

func atomicFilePublished(file string, b []byte) (bool, error) {
	f, e := os.CreateTemp(filepath.Dir(file), ".snapshot-")
	if e != nil {
		return false, e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return false, e
	}
	if ce != nil {
		return false, ce
	}
	if e = os.Rename(f.Name(), file); e != nil {
		return false, e
	}
	d, e := os.Open(filepath.Dir(file))
	if e != nil {
		return true, e
	}
	defer d.Close()
	return true, d.Sync()
}
func readJSON(file string, v any) error {
	info, e := os.Lstat(file)
	if errors.Is(e, os.ErrNotExist) {
		return failure(404, "SNAPSHOT_NOT_FOUND", "快照或恢复记录不存在")
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 64*1024*1024 {
		return failure(503, "SNAPSHOT_STORE_UNAVAILABLE", "快照清单无法读取")
	}
	f, e := os.Open(file)
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return failure(503, "SNAPSHOT_STORE_CORRUPT", "快照清单损坏")
	}
	if e = d.Decode(new(any)); !errors.Is(e, io.EOF) {
		return failure(503, "SNAPSHOT_STORE_CORRUPT", "快照清单含有多余内容")
	}
	return nil
}
func lockFile(file string) (func(), error) {
	f, e := os.OpenFile(file, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, failure(409, "SNAPSHOT_BUSY", "另一个快照或恢复任务正在执行")
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
}
func (s *Service) record(kind, id string) string {
	return filepath.Join(s.cfg.Directory, kind, id+".json")
}
func (s *Service) data(id string) string { return filepath.Join(s.cfg.Directory, "contents", id) }
func (s *Service) ids(kind string) ([]string, error) {
	entries, e := os.ReadDir(filepath.Join(s.cfg.Directory, kind))
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !entry.IsDir() && validID(id) && entry.Name() == id+".json" {
			out = append(out, id)
		}
	}
	return out, nil
}
func (s *Service) snapshot(id string) (Snapshot, error) {
	var out Snapshot
	if !validID(id) {
		return out, failure(400, "SNAPSHOT_ID_INVALID", "快照标识必须是 UUID")
	}
	e := readJSON(s.record("snapshots", id), &out)
	if e == nil && out.ID != id {
		return out, failure(503, "SNAPSHOT_STORE_CORRUPT", "快照身份不符")
	}
	// 早期失败或中断的任务可能尚未建立组件清单，旧账本会保存 null。
	// 对外始终返回数组；读取不重写历史记录，也不改变失败状态。
	if e == nil && out.Components == nil {
		out.Components = []Component{}
	}
	return out, e
}
func (s *Service) manifest(id string) (Manifest, error) {
	var m Manifest
	if !validID(id) {
		return m, failure(400, "SNAPSHOT_ID_INVALID", "快照标识无效")
	}
	e := readJSON(filepath.Join(s.data(id), "manifest.json"), &m)
	if e == nil {
		e = validateManifest(m, id)
	}
	return m, e
}
func validateManifest(m Manifest, id string) error {
	if m.Contract != Contract || m.ID != id || !validID(m.ID) || m.CapturedAt.IsZero() || m.Version == "" || m.Platform == "" || m.DatabaseSnapshot == "" || len(m.Files) == 0 || len(m.Files) > 200000 {
		return failure(422, "SNAPSHOT_MANIFEST_INVALID", "快照版本或必需字段不受支持")
	}
	seen := map[string]bool{}
	components := map[string]int{}
	for _, f := range m.Files {
		if !cleanPath(f.Path) || !shaPattern.MatchString(f.SHA256) || f.Size < 0 || seen[f.Path] || f.Mode&^uint32(0777) != 0 {
			return failure(422, "SNAPSHOT_MANIFEST_INVALID", "快照文件清单无效")
		}
		seen[f.Path] = true
		components[strings.SplitN(f.Path, "/", 2)[0]]++
	}
	for _, kind := range []string{"database", "configuration", "programs"} {
		if components[kind] == 0 {
			return failure(422, "SNAPSHOT_COMPONENT_MISSING", "快照缺少必需组件")
		}
	}
	if !seen["metadata/media-inventory.json"] {
		return failure(422, "SNAPSHOT_COMPONENT_MISSING", "媒体组件必须有显式引用清单")
	}
	if !seen["database/database.dump"] || !seen["configuration/runtime.enc"] {
		return failure(422, "SNAPSHOT_COMPONENT_MISSING", "快照缺少数据库或加密配置")
	}
	return nil
}
