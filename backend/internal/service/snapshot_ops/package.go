package snapshot_ops

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

const maxPackageBytes int64 = 8 * 1024 * 1024 * 1024

// ExportFile 返回已经完整校验和加密的临时包，发送失败不能伪装导出成功。
func (s *Service) ExportFile(ctx context.Context, id, password string) (string, func(), error) {
	if e := s.check(ctx); e != nil {
		return "", nil, e
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return "", nil, e
	}
	defer unlock()
	j, e := s.snapshot(id)
	if e != nil {
		return "", nil, e
	}
	if j.Status != "ready" {
		return "", nil, failure(409, "SNAPSHOT_NOT_READY", "快照尚未完成或已清理")
	}
	m, e := s.manifest(id)
	if e != nil {
		return "", nil, e
	}
	if e = verifyFiles(ctx, s.data(id), m); e != nil {
		return "", nil, e
	}
	size, e := packageSize(m)
	if e != nil {
		return "", nil, e
	}
	if e = requireSpace(s.cfg.Directory, size); e != nil {
		return "", nil, e
	}
	wrapped, e := os.ReadFile(filepath.Join(s.data(id), "key.enc"))
	if e != nil {
		return "", nil, e
	}
	key, e := decryptConfig(wrapped, s.key)
	if e != nil {
		return "", nil, failure(422, "SNAPSHOT_KEY_UNAVAILABLE", "快照解密材料不可用")
	}
	if _, e = readConfiguration(s.data(id), key); e != nil {
		return "", nil, e
	}
	f, e := os.CreateTemp(filepath.Join(s.cfg.Directory, "staging"), "export-*.pxsnapshot")
	if e != nil {
		return "", nil, e
	}
	name := f.Name()
	cleanup := func() { _ = os.Remove(name) }
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		tw := tar.NewWriter(writer)
		raw, te := json.Marshal(m)
		if te == nil {
			te = tarBytes(tw, "manifest.json", raw)
		}
		if te == nil {
			te = tarBytes(tw, "configuration/key.bin", key)
		}
		for _, entry := range m.Files {
			if te != nil {
				break
			}
			te = tw.WriteHeader(&tar.Header{Name: entry.Path, Size: entry.Size, Mode: int64(entry.Mode), Typeflag: tar.TypeReg})
			if te != nil {
				break
			}
			in, oe := os.Open(filepath.Join(s.data(id), filepath.FromSlash(entry.Path)))
			if oe != nil {
				te = oe
				break
			}
			_, te = io.Copy(tw, contextReader{ctx, in})
			in.Close()
		}
		if te == nil {
			te = tw.Close()
		}
		_ = writer.CloseWithError(te)
		done <- te
	}()
	e = encryptStream(&packageWriter{writer: f, remaining: maxPackageBytes}, contextReader{ctx, reader}, password)
	_ = reader.CloseWithError(e)
	te := <-done
	if e == nil {
		e = te
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = s.audit(ctx, "export", id, "success")
	}
	if e != nil {
		cleanup()
		return "", nil, e
	}
	return name, cleanup, nil
}

type packageWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *packageWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > w.remaining {
		return 0, failure(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限")
	}
	n, e := w.writer.Write(p)
	w.remaining -= int64(n)
	return n, e
}
func tarBytes(tw *tar.Writer, name string, raw []byte) error {
	if e := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(raw)), Mode: 0600, Typeflag: tar.TypeReg}); e != nil {
		return e
	}
	_, e := tw.Write(raw)
	return e
}

// Import 接受完整加密系统包，密钥在包内受外层口令保护，导入后按目标主密钥重新包裹。
func (s *Service) Import(ctx context.Context, source io.Reader, password string) (Snapshot, error) {
	var out Snapshot
	if e := s.check(ctx); e != nil {
		return out, e
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return out, e
	}
	defer unlock()
	temp, e := os.MkdirTemp(filepath.Join(s.cfg.Directory, "staging"), "import-")
	if e != nil {
		return out, e
	}
	defer os.RemoveAll(temp)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		de := decryptStream(writer, io.LimitReader(contextReader{ctx, source}, maxPackageBytes+1), password)
		_ = writer.CloseWithError(de)
		done <- de
	}()
	e = extractPackage(ctx, reader, temp)
	if e == nil {
		_, e = io.Copy(io.Discard, reader)
	}
	_ = reader.CloseWithError(e)
	de := <-done
	if e == nil {
		e = de
	}
	if e != nil {
		return out, e
	}
	var m Manifest
	if e = readJSON(filepath.Join(temp, "manifest.json"), &m); e != nil {
		return out, e
	}
	if e = validateManifest(m, m.ID); e != nil {
		return out, e
	}
	if e = verifyFiles(ctx, temp, m); e != nil {
		return out, e
	}
	allowed := map[string]bool{"manifest.json": true, "configuration/key.bin": true}
	for _, f := range m.Files {
		allowed[f.Path] = true
	}
	count := 0
	e = filepath.WalkDir(temp, func(p string, d os.DirEntry, we error) error {
		if we != nil {
			return we
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(temp, p)
		if !allowed[filepath.ToSlash(rel)] {
			return failure(422, "SNAPSHOT_EXTRA_CONTENT", "恢复包含未声明内容")
		}
		count++
		return nil
	})
	if e != nil {
		return out, e
	}
	if count != len(allowed) {
		return out, failure(422, "SNAPSHOT_COMPONENT_MISSING", "恢复包内容不齐全")
	}
	key, e := os.ReadFile(filepath.Join(temp, "configuration", "key.bin"))
	if e != nil || len(key) != 32 {
		return out, failure(422, "SNAPSHOT_KEY_UNAVAILABLE", "配置解密材料缺失")
	}
	configuration, e := readConfiguration(temp, key)
	if e != nil {
		return out, e
	}
	for _, f := range m.Files {
		mode := os.FileMode(0600)
		if strings.HasPrefix(f.Path, "programs/") {
			mode |= os.FileMode(f.Mode) & 0100
		}
		if e = os.Chmod(filepath.Join(temp, filepath.FromSlash(f.Path)), mode); e != nil {
			return out, e
		}
	}
	if _, e = os.Stat(s.data(m.ID)); e == nil {
		return out, failure(409, "SNAPSHOT_EXISTS", "此快照已存在，请使用现有记录")
	}
	if !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	wrapped, e := encryptConfig(key, s.key)
	if e != nil {
		return out, e
	}
	if e = atomicFile(filepath.Join(temp, "key.enc"), wrapped); e != nil {
		return out, e
	}
	if e = os.Remove(filepath.Join(temp, "configuration", "key.bin")); e != nil {
		return out, e
	}
	out = Snapshot{RecoverySupported: supportsManagedRecovery(configuration), ID: m.ID, Status: "ready", Phase: "imported", CreatedAt: m.CapturedAt, CapturedAt: &m.CapturedAt, UpdatedAt: s.opts.Now(), Trigger: "import", Instance: m.Instance, Version: m.Version, Components: components(m.Files), Imported: true, Protected: true, Operator: operator(ctx)}
	if e = s.audit(ctx, "import", m.ID, "started"); e != nil {
		return Snapshot{}, e
	}
	if e = s.publishImportedSnapshot(temp, out, atomicJSONPublished); e != nil {
		s.finishAudit(ctx, "import", m.ID, "failed")
		return Snapshot{}, e
	}
	s.finishAudit(ctx, "import", m.ID, "success")
	return out, nil
}

// publishImportedSnapshot 在执行锁内发布内容与记录；未发布记录时退回暂存目录，
// 交给 Import 的清理收尾。记录已经可见而同步失败时保留两者，不破坏可读取的包。
func (s *Service) publishImportedSnapshot(temp string, out Snapshot, publish func(string, any) (bool, error)) error {
	if err := os.Rename(temp, s.data(out.ID)); err != nil {
		return err
	}
	published, err := publish(s.record("snapshots", out.ID), out)
	if err == nil || published {
		return err
	}
	if rollbackErr := os.Rename(s.data(out.ID), temp); rollbackErr != nil {
		// 磁盘耗尽等情况下 rename 也可能失败；新内容尚未对外发布，可直接清理。
		if cleanupErr := os.RemoveAll(s.data(out.ID)); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("导入记录发布失败，内容目录回滚未完成：%w", rollbackErr), fmt.Errorf("导入内容清理失败：%w", cleanupErr))
		}
	}
	return err
}

func extractPackage(ctx context.Context, source io.Reader, dir string) error {
	tr := tar.NewReader(source)
	seen := map[string]bool{}
	var total int64
	for {
		h, e := tr.Next()
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包格式损坏")
		}
		if !cleanPath(h.Name) || h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > maxPackageBytes || seen[h.Name] || len(seen) > 200002 {
			return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包路径、类型或大小无效")
		}
		total += h.Size
		if total > maxPackageBytes {
			return failure(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限")
		}
		seen[h.Name] = true
		if h.Name != "manifest.json" && h.Name != "configuration/key.bin" && !strings.HasPrefix(h.Name, "database/") && !strings.HasPrefix(h.Name, "media/") && !strings.HasPrefix(h.Name, "configuration/") && !strings.HasPrefix(h.Name, "programs/") && h.Name != "metadata/media-inventory.json" {
			return failure(422, "SNAPSHOT_PACKAGE_INVALID", "恢复包包含未知组件")
		}
		if (h.Name == "manifest.json" || h.Name == "metadata/media-inventory.json") && h.Size > 64*1024*1024 {
			return failure(413, "SNAPSHOT_MANIFEST_TOO_LARGE", "恢复清单过大")
		}
		if h.Name == "configuration/key.bin" && h.Size != 32 {
			return failure(422, "SNAPSHOT_KEY_UNAVAILABLE", "配置解密材料无效")
		}
		if h.Name == "configuration/runtime.enc" && h.Size > maxConfigCipherBytes {
			return failure(413, "SNAPSHOT_CONFIG_TOO_LARGE", "配置密文过大")
		}
		if e = requireSpace(dir, h.Size); e != nil {
			return e
		}
		dest := filepath.Join(dir, filepath.FromSlash(h.Name))
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		n, e := io.Copy(f, contextReader{ctx, tr})
		if e == nil && n != h.Size {
			e = io.ErrUnexpectedEOF
		}
		if e == nil {
			e = f.Sync()
		}
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
}

// StartRestore 创建隔离数据恢复任务。应用启动与正式切换尚需独立验证，不自动执行。
func (s *Service) StartRestore(ctx context.Context, id, key string) (Restore, error) {
	var r Restore
	if e := s.check(ctx); e != nil {
		return r, e
	}
	if !validID(key) {
		return r, failure(400, "SNAPSHOT_IDEMPOTENCY_REQUIRED", "请提供 UUID Idempotency-Key")
	}
	if prior, found, e := s.findRestoreRequest(id, key); e != nil {
		return r, e
	} else if found {
		return prior, nil
	}
	if s.opts.Backend == nil {
		return r, failure(503, "SNAPSHOT_RESTORE_UNAVAILABLE", "未配置隔离恢复适配器")
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return r, e
	}
	hash := digest([]byte("restore:" + id + ":" + key))
	ids, e := s.ids("restores")
	if e != nil {
		unlock()
		return r, e
	}
	for _, rid := range ids {
		var old Restore
		if e = readJSON(s.record("restores", rid), &old); e != nil {
			unlock()
			return r, e
		}
		var token struct {
			Hash string `json:"hash"`
		}
		_ = readJSON(filepath.Join(old.Directory, "request.json"), &token)
		if token.Hash == hash {
			unlock()
			return old, nil
		}
	}
	j, e := s.snapshot(id)
	if e != nil {
		unlock()
		return r, e
	}
	if j.Status != "ready" {
		unlock()
		return r, failure(409, "SNAPSHOT_NOT_READY", "快照未完成或已清理")
	}
	m, e := s.manifest(id)
	if e != nil {
		unlock()
		return r, e
	}
	if m.Version != s.cfg.Version {
		unlock()
		return r, failure(422, "SNAPSHOT_VERSION_MISMATCH", "目标 Core 版本与快照不匹配，请先准备对应版本")
	}
	size, e := packageSize(m)
	if e != nil {
		unlock()
		return r, e
	}
	if e = requireSpace(s.cfg.Directory, size); e != nil {
		unlock()
		return r, e
	}
	r = Restore{ID: uuid.NewString(), SnapshotID: id, Status: "queued", Phase: "queued", CreatedAt: s.opts.Now(), UpdatedAt: s.opts.Now(), Operator: operator(ctx), TraceID: reqctx.GetTraceID(ctx), Components: []Component{}, Warnings: []string{"仅恢复到隔离数据副本；尚未启动应用或完成业务验收，未切换正式站"}}
	r.Directory = filepath.Join(s.cfg.Directory, "restores", r.ID)
	if e = os.Mkdir(r.Directory, 0700); e != nil {
		unlock()
		return r, e
	}
	if e = atomicJSON(filepath.Join(r.Directory, "request.json"), map[string]string{"hash": hash}); e != nil {
		unlock()
		return r, e
	}
	if e = s.audit(ctx, "restore", r.ID, "queued"); e != nil {
		unlock()
		return r, e
	}
	if e = atomicJSON(s.record("restores", r.ID), r); e != nil {
		unlock()
		return r, e
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		unlock()
		return Restore{}, failure(503, "SNAPSHOT_STOPPED", "服务已停止")
	}
	s.workers.Add(1)
	s.mu.Unlock()
	go func(r Restore) {
		defer s.workers.Done()
		defer unlock()
		run, cancel := context.WithTimeout(s.ctx, 45*time.Minute)
		defer cancel()
		e := s.restore(run, m, &r)
		if e != nil {
			r.Status = "failed"
			r.Phase = "failed"
			r.Error = e.Error()
		} else {
			r.Status = "prepared"
			r.Phase = "data_verified"
		}
		r.UpdatedAt = s.opts.Now()
		_ = atomicJSON(s.record("restores", r.ID), r)
		s.finishAudit(ctx, "restore", r.ID, r.Status)
	}(r)
	return r, nil
}
func (s *Service) restore(ctx context.Context, m Manifest, r *Restore) error {
	if e := verifyFiles(ctx, s.data(m.ID), m); e != nil {
		return e
	}
	r.Status = "running"
	r.Phase = "files"
	r.UpdatedAt = s.opts.Now()
	if e := atomicJSON(s.record("restores", r.ID), r); e != nil {
		return e
	}
	for _, f := range m.Files {
		if e := copyTree(ctx, filepath.Join(s.data(m.ID), filepath.FromSlash(f.Path)), filepath.Join(r.Directory, filepath.FromSlash(f.Path))); e != nil {
			return e
		}
	}
	if e := verifyFiles(ctx, r.Directory, m); e != nil {
		return e
	}
	wrapped, e := os.ReadFile(filepath.Join(s.data(m.ID), "key.enc"))
	if e != nil {
		return e
	}
	key, e := decryptConfig(wrapped, s.key)
	if e != nil {
		return e
	}
	if _, e = readConfiguration(r.Directory, key); e != nil {
		return e
	}
	// 候选配置继续保持密文，目标映射阶段完成前不能被应用当作生产配置加载。
	if e = atomicFile(filepath.Join(r.Directory, "configuration", "key.enc"), wrapped); e != nil {
		return e
	}
	r.Phase = "database"
	if e = atomicJSON(s.record("restores", r.ID), r); e != nil {
		return e
	}
	r.DatabaseCopyID, r.Database, e = s.opts.Backend.RestoreDatabase(ctx, filepath.Join(r.Directory, "database", "database.dump"))
	if e != nil {
		return e
	}
	r.Components = components(m.Files)
	sort.Slice(r.Components, func(i, j int) bool { return r.Components[i].Kind < r.Components[j].Kind })
	return nil
}
