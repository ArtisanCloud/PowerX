package archive_ops

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"github.com/google/uuid"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

func (s *Service) Job(ctx context.Context, id string) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	j, e := s.readJob(id)
	return j.Job, e
}
func (s *Service) Download(ctx context.Context, id string, w io.Writer) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	j, e := s.readJob(id)
	if e != nil {
		return e
	}
	if j.Status != "success" && !(j.Status == "pruned" && j.RemoteState == "success") {
		return failure(409, "ARCHIVE_NOT_READY", "归档尚未完成或已清理")
	}
	unlock, e := s.lock(ctx, j.PolicyID)
	if e != nil {
		return failure(409, "ARCHIVE_BUSY", "该策略正在执行或清理")
	}
	defer unlock()
	if e = s.ensureObjects(ctx, j); e != nil {
		return e
	}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	defer gz.Close()
	defer tw.Close()
	for _, entry := range j.Files {
		if !cleanEntryPath(entry.Path) || !shaPattern.MatchString(entry.SHA256) {
			return errors.New("归档清单路径损坏")
		}
		path := s.objectPath(entry.SHA256)
		if e = verifyObject(ctx, path, entry); e != nil {
			return e
		}
		if e = tw.WriteHeader(&tar.Header{Name: entry.Path, Size: entry.Size, Mode: 0600, ModTime: entry.ModifiedAt}); e != nil {
			return e
		}
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		_, e = io.Copy(tw, &contextReader{ctx, f})
		f.Close()
		if e != nil {
			return e
		}
	}
	raw, e := os.ReadFile(s.path("jobs", id))
	if e != nil {
		return e
	}
	if e = tw.WriteHeader(&tar.Header{Name: "archive-manifest.json", Size: int64(len(raw)), Mode: 0600}); e != nil {
		return e
	}
	if _, e = tw.Write(raw); e != nil {
		return e
	}
	if e = tw.Close(); e != nil {
		return e
	}
	return gz.Close()
}
func (s *Service) StartRestore(ctx context.Context, id string, allowOlder bool, operator, trace string) (Restore, error) {
	var out Restore
	if e := s.check(ctx); e != nil {
		return out, e
	}
	j, e := s.readJob(id)
	if e != nil {
		return out, e
	}
	if (j.Status != "success" && !(j.Status == "pruned" && j.RemoteState == "success")) || j.Kind != "media" {
		return out, failure(409, "ARCHIVE_RESTORE_UNAVAILABLE", "只能恢复完整媒体快照")
	}
	unlock, e := s.lock(ctx, j.PolicyID)
	if e != nil {
		return out, failure(409, "ARCHIVE_BUSY", "策略正在执行")
	}
	for _, entry := range j.Files {
		if entry.Path == "database/database.dump" && s.opts.RestoreDatabase == nil {
			unlock()
			return out, failure(503, "ARCHIVE_DATABASE_RESTORE_UNAVAILABLE", "请在 Core 页面恢复配套数据库，独立定时任务不提供应用连接切换")
		}
	}
	out = Restore{ID: uuid.NewString(), JobID: id, Status: "running", CreatedAt: time.Now().UTC()}
	out.Directory = filepath.Join(s.cfg.Directory, "restores", out.ID)
	if e = writeJSON(s.path("restores", out.ID), out); e != nil {
		unlock()
		return out, e
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		unlock()
		return out, failure(503, "ARCHIVE_STOPPED", "服务已停止")
	}
	s.workers.Add(1)
	s.mu.Unlock()
	response := out
	go func() {
		defer s.workers.Done()
		defer unlock()
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		e := s.restore(ctx, j, &out, allowOlder, operator, trace)
		if e != nil {
			out.Status = "failed"
			out.Error = e.Error()
		} else {
			out.Status = "success"
		}
		_ = writeJSON(s.path("restores", out.ID), out)
	}()
	return response, nil
}
func (s *Service) restore(ctx context.Context, j jobRecord, r *Restore, allowOlder bool, operator, trace string) error {
	if e := s.ensureObjects(ctx, j); e != nil {
		return e
	}
	if e := os.Mkdir(r.Directory, 0700); e != nil {
		return errors.New("无法创建本次独立恢复目录")
	}
	info, e := os.Stat(r.Directory)
	if e != nil {
		return e
	}
	if v, ok := info.Sys().(*syscall.Stat_t); ok {
		r.DirectoryDevice = uint64(v.Dev)
		r.DirectoryInode = uint64(v.Ino)
	}
	for _, entry := range j.Files {
		if !cleanEntryPath(entry.Path) || !shaPattern.MatchString(entry.SHA256) {
			return errors.New("归档清单格式无效")
		}
		path := s.objectPath(entry.SHA256)
		if e := verifyObject(ctx, path, entry); e != nil {
			return e
		}
		target := filepath.Join(r.Directory, filepath.FromSlash(entry.Path))
		if e := os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		f, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		source, e := os.Open(path)
		if e != nil {
			f.Close()
			return e
		}
		_, e = io.Copy(f, &contextReader{ctx, source})
		source.Close()
		closed := f.Close()
		if e != nil {
			return e
		}
		if closed != nil {
			return closed
		}
		r.FileCount++
	}
	for _, entry := range j.Files {
		if entry.Path == "database/database.dump" {
			id, name, e := s.opts.RestoreDatabase(ctx, filepath.Join(r.Directory, "database", "database.dump"), allowOlder, operator, trace)
			if e != nil {
				return e
			}
			r.DatabaseCopyID = id
			r.DatabaseCopyName = name
		}
	}
	return nil
}
func (s *Service) Protect(ctx context.Context, id string, value bool) (Job, error) {
	if e := s.check(ctx); e != nil {
		return Job{}, e
	}
	j, e := s.readJob(id)
	if e != nil {
		return Job{}, e
	}
	unlock, e := s.lock(ctx, j.PolicyID)
	if e != nil {
		return Job{}, failure(409, "ARCHIVE_BUSY", "该策略正在执行")
	}
	defer unlock()
	j.Protected = value
	if j.RemoteState == "success" && j.Target != nil {
		if _, e = s.remote(ctx, *j.Target, "protect", j.PolicyID, j.ID, map[string]any{"protected": value}); e != nil {
			return Job{}, failure(503, "ARCHIVE_PROTECTION_FAILED", "远端保护状态更新失败；本地保护状态保持原值")
		}
	}
	e = s.saveJob(&j)
	return j.Job, e
}
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	if e := s.check(ctx); e != nil {
		return 0, e
	}
	ids, e := s.records("policies")
	if e != nil {
		return 0, e
	}
	total := 0
	for _, id := range ids {
		n, e := s.cleanup(ctx, id)
		if e != nil {
			return total, e
		}
		total += n
	}
	return total, nil
}
func (s *Service) cleanup(ctx context.Context, policyID string) (int, error) {
	unlock, e := s.lock(ctx, policyID)
	if e != nil {
		return 0, nil
	}
	defer unlock()
	p, e := s.policy(ctx, policyID)
	if e != nil {
		return 0, e
	}
	ids, e := s.records("jobs")
	if e != nil {
		return 0, e
	}
	rows := []jobRecord{}
	for _, id := range ids {
		j, e := s.readJob(id)
		if e != nil {
			return 0, e
		}
		if j.PolicyID == p.ID && j.Status == "success" {
			rows = append(rows, j)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	n := 0
	for i, j := range rows {
		if i == 0 || j.Protected || (j.Target != nil && j.RemoteState != "success") {
			continue
		}
		if i < p.RetentionCount && time.Since(j.CreatedAt) < time.Duration(p.RetentionDays)*24*time.Hour {
			continue
		}
		j.Status = "pruned"
		j.Phase = "pruned"
		if e = s.saveJob(&j); e != nil {
			return n, e
		}
		n++
	}
	// GC 使用独立锁，且任何备份或恢复正在运行时均不清理对象。
	gcUnlock, e := s.lock(ctx, "gc")
	if e != nil {
		return n, nil
	}
	defer gcUnlock()
	policyIDs, _ := s.records("policies")
	locks := []func(){}
	defer func() {
		for _, u := range locks {
			u()
		}
	}()
	for _, id := range policyIDs {
		if id == policyID {
			continue
		}
		u, e := s.lock(ctx, id)
		if e != nil {
			return n, nil
		}
		locks = append(locks, u)
	}
	used := map[string]bool{}
	all, _ := s.records("jobs")
	for _, id := range all {
		j, e := s.readJob(id)
		if e != nil {
			return n, e
		}
		if j.Status == "running" {
			return n, nil
		}
		if j.Status == "success" {
			for _, f := range j.Files {
				used[f.SHA256] = true
			}
		}
	}
	dirs, e := os.ReadDir(filepath.Join(s.cfg.Directory, "objects"))
	if e != nil {
		return n, e
	}
	for _, dir := range dirs {
		if !dir.IsDir() || len(dir.Name()) != 2 {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(s.cfg.Directory, "objects", dir.Name()))
		for _, f := range files {
			if shaPattern.MatchString(f.Name()) && !used[f.Name()] {
				path := filepath.Join(s.cfg.Directory, "objects", dir.Name(), f.Name())
				info, e := os.Lstat(path)
				if e == nil && info.Mode().IsRegular() {
					if e = os.Remove(path); e != nil {
						return n, e
					}
				}
			}
		}
	}
	return n, nil
}
func (s *Service) RemoveRestore(ctx context.Context, id, confirmation string) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	if !validID(id) {
		return failure(400, "ARCHIVE_ID_INVALID", "恢复标识无效")
	}
	var r Restore
	if e := readJSON(s.path("restores", id), &r); e != nil {
		return e
	}
	if r.Status == "running" {
		return failure(409, "ARCHIVE_RESTORE_RUNNING", "恢复正在进行")
	}
	expected := filepath.Join(s.cfg.Directory, "restores", id)
	if r.Directory != expected || confirmation != id {
		return failure(400, "ARCHIVE_CONFIRMATION_REQUIRED", "请输入完整恢复 ID 确认删除本次新目录")
	}
	info, e := os.Lstat(expected)
	if e == nil && (!info.IsDir() || !sameRestoreDirectory(info, r)) {
		return failure(409, "ARCHIVE_RESTORE_CHANGED", "恢复目录身份已改变")
	}
	if e == nil {
		if e = os.RemoveAll(expected); e != nil {
			return e
		}
	}
	r.Status = "deleted"
	return writeJSON(s.path("restores", id), r)
}
func (s *Service) RestoreDirectory(ctx context.Context, id string) (Restore, error) {
	var r Restore
	if e := s.check(ctx); e != nil {
		return r, e
	}
	if !validID(id) {
		return r, failure(400, "ARCHIVE_ID_INVALID", "恢复标识无效")
	}
	e := readJSON(s.path("restores", id), &r)
	return r, e
}

func (s *Service) DownloadFile(ctx context.Context, id string) (string, func(), error) {
	f, e := os.CreateTemp(filepath.Join(s.cfg.Directory, "staging"), "download-*.tar.gz")
	if e != nil {
		return "", nil, e
	}
	path := f.Name()
	cleanup := func() { os.Remove(path) }
	e = s.Download(ctx, id, f)
	closed := f.Close()
	if e != nil {
		cleanup()
		return "", nil, e
	}
	if closed != nil {
		cleanup()
		return "", nil, closed
	}
	return path, cleanup, nil
}

func sameRestoreDirectory(info os.FileInfo, r Restore) bool {
	v, ok := info.Sys().(*syscall.Stat_t)
	return ok && r.DirectoryInode != 0 && uint64(v.Ino) == r.DirectoryInode && uint64(v.Dev) == r.DirectoryDevice
}
