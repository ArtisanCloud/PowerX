package archive_ops

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validID(id string) bool {
	v, e := uuid.Parse(id)
	return e == nil && v != uuid.Nil && v.String() == id
}
func writeJSON(path string, value any) error {
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(path, raw)
}
func atomicWrite(path string, raw []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".archive-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e != nil {
		return e
	}
	if closed != nil {
		return closed
	}
	return os.Rename(name, path)
}
func readJSON(path string, value any) error {
	info, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return failure(404, "ARCHIVE_NOT_FOUND", "归档记录不存在")
	}
	if e != nil || !info.Mode().IsRegular() {
		return failure(503, "ARCHIVE_STORE_UNAVAILABLE", "归档清单不可读取")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if json.Unmarshal(raw, value) != nil {
		return failure(503, "ARCHIVE_STORE_CORRUPT", "归档清单损坏，拒绝自动清理")
	}
	return nil
}
func (s *Service) path(kind, id string) string {
	return filepath.Join(s.cfg.Directory, kind, id+".json")
}
func (s *Service) records(kind string) ([]string, error) {
	entries, e := os.ReadDir(filepath.Join(s.cfg.Directory, kind))
	if e != nil {
		return nil, e
	}
	ids := []string{}
	for _, f := range entries {
		id := strings.TrimSuffix(f.Name(), ".json")
		if validID(id) && f.Name() == id+".json" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (s *Service) readJob(id string) (jobRecord, error) {
	var j jobRecord
	if !validID(id) {
		return j, failure(400, "ARCHIVE_ID_INVALID", "归档标识必须是 UUID")
	}
	e := readJSON(s.path("jobs", id), &j)
	if e == nil && j.ID != id {
		e = failure(503, "ARCHIVE_STORE_CORRUPT", "归档清单身份不符")
	}
	return j, e
}
func (s *Service) saveJob(j *jobRecord) error {
	j.UpdatedAt = time.Now().UTC()
	return writeJSON(s.path("jobs", j.ID), j)
}
func (s *Service) objectPath(sha string) string {
	return filepath.Join(s.cfg.Directory, "objects", sha[:2], sha)
}
func (s *Service) policy(ctx context.Context, id string) (Policy, error) {
	var p Policy
	if !validID(id) {
		return p, failure(400, "ARCHIVE_ID_INVALID", "策略标识无效")
	}
	e := readJSON(s.path("policies", id), &p)
	return p, e
}
func (s *Service) lock(ctx context.Context, name string) (func(), error) {
	if name == "" {
		name = "one-time"
	}
	if !labelPattern.MatchString(name) && !validID(name) {
		return nil, ErrBusy
	}
	f, e := os.OpenFile(filepath.Join(s.cfg.Directory, "locks", name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, ErrBusy
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
func cleanEntryPath(path string) bool {
	if path == "" || strings.Contains(path, "\\") || strings.HasPrefix(path, "/") {
		return false
	}
	parts := strings.Split(path, "/")
	for _, p := range parts {
		if p == "" || p == "." || p == ".." || strings.ContainsRune(p, 0) {
			return false
		}
	}
	return len(path) < 4096
}
