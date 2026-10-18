package snapshot_ops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
)

func (s *Service) capture(ctx context.Context, j *Snapshot) error {
	dest := s.data(j.ID)
	j.Status = "capturing"
	j.Phase = "database_and_media"
	j.UpdatedAt = s.opts.Now()
	if e := atomicJSON(s.record("snapshots", j.ID), j); e != nil {
		return e
	}
	var e error
	cfg := s.cfg
	if s.opts.Sources != nil {
		cfg, e = s.opts.Sources(ctx)
		if e != nil {
			return e
		}
	}
	capturedAt := s.opts.Now()
	// 包内复制校验之外，再覆盖整个采集窗口，检测已复制文件被改写或新增/删除版本。
	trees := map[string]string{}
	for _, source := range append(artifactRoots(cfg.Artifacts), cfg.LifecycleRoots...) {
		hash, e := treeFingerprint(ctx, source)
		if e != nil {
			return e
		}
		trees[source] = hash
	}
	if cfg.MediaRoot != "" {
		if e = rejectContained(cfg.MediaRoot, dest); e != nil {
			return e
		}
	}
	if len(cfg.EffectiveConfig) == 0 || len(cfg.EffectiveConfig) > maxConfigBytes {
		return errors.New("生效配置缺失或超过大小上限")
	}
	config := map[string][]byte{"effective.yaml": cfg.EffectiveConfig}
	before := map[string]string{}
	configBytes := len(cfg.EffectiveConfig)
	for logical, source := range cfg.ConfigFiles {
		if !cleanPath(logical) || logical == "effective.yaml" || len(config) >= 2048 {
			return errors.New("配置清单路径无效")
		}
		raw, e := readPrivateSource(source)
		if e != nil {
			return e
		}
		config[logical] = raw
		configBytes += len(raw)
		if configBytes > maxConfigBytes {
			return errors.New("配置总量超过大小上限")
		}
		before[source] = digest(raw)
	}
	dataKey := make([]byte, 32)
	if _, e = rand.Read(dataKey); e != nil {
		return e
	}
	raw, e := json.Marshal(config)
	if e != nil {
		return e
	}
	enc, e := encryptConfig(raw, dataKey)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Join(dest, "configuration"), 0700); e != nil {
		return e
	}
	if e = atomicFile(filepath.Join(dest, "configuration", "runtime.enc"), enc); e != nil {
		return e
	}
	wrapped, e := encryptConfig(dataKey, s.key)
	if e != nil {
		return e
	}
	if e = atomicFile(filepath.Join(dest, "key.enc"), wrapped); e != nil {
		return e
	}
	for logical, source := range cfg.Artifacts {
		if !cleanPath(logical) {
			return errors.New("程序产物路径无效")
		}
		if e = rejectContained(source, dest); e != nil {
			return e
		}
		if e = copyTree(ctx, source, filepath.Join(dest, "programs", filepath.FromSlash(logical))); e != nil {
			return fmt.Errorf("程序产物采集失败（%s）: %w", logical, e)
		}
	}
	var snapshot string
	if b, ok := s.opts.Backend.(interface {
		CapturePoint(context.Context, string) (string, time.Time, error)
	}); ok {
		snapshot, capturedAt, e = b.CapturePoint(ctx, dest)
	} else {
		snapshot, e = s.opts.Backend.Capture(ctx, dest)
	}
	if e != nil {
		return e
	}
	if capturedAt.IsZero() {
		return errors.New("数据库恢复点时间缺失，不能封存快照")
	}
	j.CapturedAt = &capturedAt
	for source, hash := range before {
		raw, e := readPrivateSource(source)
		if e != nil || digest(raw) != hash {
			return fmt.Errorf("%w: 配置或插件绑定变化", archiveops.ErrSourceChanged)
		}
	}
	for source, hash := range trees {
		after, e := treeFingerprint(ctx, source)
		if e != nil || after != hash {
			return fmt.Errorf("%w: 程序、插件生命周期或配置目录变化", archiveops.ErrSourceChanged)
		}
	}
	j.Status = "verifying"
	j.Phase = "verifying"
	j.UpdatedAt = s.opts.Now()
	if e = atomicJSON(s.record("snapshots", j.ID), j); e != nil {
		return e
	}
	m := Manifest{Contract: Contract, ID: j.ID, Instance: cfg.Instance, Version: cfg.Version, Platform: cfg.Platform, CapturedAt: capturedAt, DatabaseSnapshot: snapshot, Files: []Entry{}, Components: []Component{}}
	if e = filepath.WalkDir(dest, func(p string, d os.DirEntry, we error) error {
		if we != nil {
			return we
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dest, p)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if rel == "key.enc" || rel == "request.json" {
			return nil
		}
		if !strings.Contains(rel, "/") {
			return errors.New("采集生成了未声明文件")
		}
		entry, e := fileEntry(ctx, p, rel)
		if e == nil {
			m.Files = append(m.Files, entry)
		}
		return e
	}); e != nil {
		return e
	}
	sort.Slice(m.Files, func(i, k int) bool { return m.Files[i].Path < m.Files[k].Path })
	m.Components = components(m.Files)
	if e = validateManifest(m, j.ID); e != nil {
		return e
	}
	if e = verifyInventory(dest, m); e != nil {
		return e
	}
	if e = atomicJSON(filepath.Join(dest, "manifest.json"), m); e != nil {
		return e
	}
	j.RecoverySupported = supportsManagedRecovery(config)
	j.Components = m.Components
	return nil
}

func artifactRoots(artifacts map[string]string) []string {
	out := make([]string, 0, len(artifacts))
	for _, source := range artifacts {
		out = append(out, source)
	}
	return out
}
func treeFingerprint(ctx context.Context, root string) (string, error) {
	entries := []string{}
	e := filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			target, e := os.Readlink(path)
			if e != nil {
				return e
			}
			entries = append(entries, rel+":link:"+target)
			return nil
		}
		if d.IsDir() {
			entries = append(entries, rel+":dir:"+info.ModTime().UTC().String())
			return nil
		}
		entry, e := fileEntry(ctx, path, rel)
		if e != nil {
			return e
		}
		entries = append(entries, rel+":"+entry.SHA256+":"+info.ModTime().UTC().String())
		return nil
	})
	if e != nil {
		return "", e
	}
	sort.Strings(entries)
	return digest([]byte(strings.Join(entries, "\n"))), nil
}
func components(files []Entry) []Component {
	out := []Component{}
	for _, kind := range []string{"database", "media", "configuration", "programs"} {
		c := Component{Kind: kind, Status: "verified"}
		for _, f := range files {
			if strings.HasPrefix(f.Path, kind+"/") {
				c.FileCount++
				c.Size += f.Size
			}
		}
		out = append(out, c)
	}
	return out
}
func readPrivateSource(path string) ([]byte, error) {
	i, e := os.Lstat(path)
	if e != nil || !i.Mode().IsRegular() {
		return nil, errors.New("配置来源必须是可读普通文件")
	}
	if i.Size() > 16*1024*1024 {
		return nil, errors.New("配置来源过大")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(i, opened) {
		return nil, fmt.Errorf("%w: 配置来源身份变化", archiveops.ErrSourceChanged)
	}
	raw, e := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if e != nil {
		return nil, e
	}
	after, e := f.Stat()
	if e != nil || len(raw) > maxConfigBytes || int64(len(raw)) != i.Size() || !after.ModTime().Equal(i.ModTime()) {
		return nil, errors.New("配置来源过大或读取期间变化")
	}
	return raw, nil
}
func fileEntry(ctx context.Context, file, logical string) (Entry, error) {
	out := Entry{Path: logical}
	before, e := os.Lstat(file)
	if e != nil || !before.Mode().IsRegular() {
		return out, errors.New("来源不是普通文件")
	}
	f, e := os.OpenFile(file, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return out, e
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(before, opened) {
		return out, errors.New("来源文件身份变化")
	}
	h := sha256.New()
	n, e := io.Copy(h, contextReader{ctx, f})
	if e != nil {
		return out, e
	}
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) || n != before.Size() || !before.ModTime().Equal(after.ModTime()) {
		return out, fmt.Errorf("%w: 读取期间文件变化", archiveops.ErrSourceChanged)
	}
	out.Size = n
	out.Mode = uint32(before.Mode().Perm())
	out.SHA256 = hex.EncodeToString(h.Sum(nil))
	return out, nil
}
func copyTree(ctx context.Context, source, target string) error {
	info, e := os.Lstat(source)
	if e != nil {
		return fmt.Errorf("复制来源不可读取（%s）：%w", source, e)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("程序来源根不能是软链接")
	}
	// 先绝对化再解析别名；相对路径的 EvalSymlinks 仍可能返回相对路径，
	// 后续 Abs 会重新引入 shell 的逻辑 PWD（macOS /var 与 /private/var）。
	absolute, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	resolved, e := filepath.EvalSymlinks(absolute)
	if e != nil {
		return e
	}
	root := resolved
	if !info.IsDir() {
		root = filepath.Dir(resolved)
	}
	return copyTreeWithin(ctx, resolved, target, root, map[string]bool{})
}

// Nitro 的包内软链接物化为普通内容；越过产物根或形成循环时拒绝。
func copyTreeWithin(ctx context.Context, source, target, root string, stack map[string]bool) error {
	resolved, e := filepath.EvalSymlinks(source)
	if e != nil {
		return e
	}
	rel, e := filepath.Rel(root, resolved)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		logical, _ := filepath.Rel(root, source)
		return fmt.Errorf("程序产物软链接越界（%s），链接目标须位于该产物目录内", filepath.ToSlash(logical))
	}
	info, e := os.Stat(resolved)
	if e != nil {
		return e
	}
	if info.IsDir() {
		if stack[resolved] {
			return errors.New("程序产物软链接形成循环")
		}
		stack[resolved] = true
		defer delete(stack, resolved)
		if e = os.MkdirAll(target, 0700); e != nil {
			return e
		}
		entries, e := os.ReadDir(resolved)
		if e != nil {
			return e
		}
		for _, d := range entries {
			if strings.HasPrefix(d.Name(), ".env") || d.Name() == ".git" || d.Name() == ".npmrc" || d.Name() == "host-values.yaml" {
				continue
			}
			if e = copyTreeWithin(ctx, filepath.Join(resolved, d.Name()), filepath.Join(target, d.Name()), root, stack); e != nil {
				return e
			}
		}
		after, e := os.Stat(resolved)
		if e != nil || !after.ModTime().Equal(info.ModTime()) {
			return fmt.Errorf("%w: 程序目录在采集期间变化", archiveops.ErrSourceChanged)
		}
		return nil
	}
	before, e := fileEntry(ctx, resolved, "source")
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
		return e
	}
	if e = requireSpace(filepath.Dir(target), before.Size); e != nil {
		return e
	}
	in, e := os.OpenFile(resolved, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = io.Copy(out, contextReader{ctx, in})
	if e == nil {
		e = out.Sync()
	}
	ce := out.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	copied, e := fileEntry(ctx, target, "target")
	if e != nil || copied.Size != before.Size || copied.SHA256 != before.SHA256 {
		return fmt.Errorf("%w: 程序产物在采集期间变化", archiveops.ErrSourceChanged)
	}
	return os.Chmod(target, os.FileMode(before.Mode)&0700)
}

func verifyFiles(ctx context.Context, dir string, m Manifest) error {
	for _, f := range m.Files {
		actual, e := fileEntry(ctx, filepath.Join(dir, filepath.FromSlash(f.Path)), f.Path)
		if e != nil || actual.Size != f.Size || actual.SHA256 != f.SHA256 {
			return failure(422, "SNAPSHOT_CONTENT_MISMATCH", "恢复包引用内容缺失或校验失败")
		}
	}
	return verifyInventory(dir, m)
}

func rejectContained(source, target string) error {
	absolute, e := filepath.Abs(source)
	if e != nil {
		return e
	}
	resolved, e := filepath.EvalSymlinks(absolute)
	if e != nil {
		return e
	}
	rel, e := filepath.Rel(resolved, target)
	if e != nil || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("来源根目录不能包含快照目录")
	}
	return nil
}

// 引用清单与实际打包对象必须一一对应，不能只校验包内文件自身的摘要。
func verifyInventory(dir string, m Manifest) error {
	var inventory []archiveops.Entry
	if e := readJSON(filepath.Join(dir, "metadata", "media-inventory.json"), &inventory); e != nil {
		return failure(422, "SNAPSHOT_INVENTORY_INVALID", "媒体引用清单缺失或损坏")
	}
	files := map[string]Entry{}
	for _, entry := range m.Files {
		if strings.HasPrefix(entry.Path, "media/") || entry.Path == "database/database.dump" {
			files[entry.Path] = entry
		}
	}
	seen := map[string]bool{}
	for _, entry := range inventory {
		f, ok := files[entry.Path]
		if !ok || seen[entry.Path] || f.SHA256 != entry.SHA256 || f.Size != entry.Size || (entry.Driver != "" && entry.Driver != "local") {
			return failure(422, "SNAPSHOT_INVENTORY_INVALID", "媒体引用与恢复包内容不一致")
		}
		seen[entry.Path] = true
	}
	if len(seen) != len(files) {
		return failure(422, "SNAPSHOT_INVENTORY_INVALID", "媒体引用清单未覆盖全部数据库与媒体内容")
	}
	return nil
}
