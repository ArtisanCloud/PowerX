package snapshot_ops

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func rootContext() context.Context {
	return reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 1}), true)
}

type fixtureBackend struct {
	source   string
	fail     bool
	change   func()
	captures atomic.Int32
	restored []byte
}

func (b *fixtureBackend) Capture(ctx context.Context, dest string) (string, error) {
	b.captures.Add(1)
	if b.fail {
		return "", errors.New("fixture failure")
	}
	raw, e := os.ReadFile(b.source)
	if e != nil {
		return "", e
	}
	inventory := []archiveops.Entry{}
	for name, data := range map[string][]byte{"media/photo.jpg": raw, "database/database.dump": append([]byte("database:"), raw...)} {
		file := filepath.Join(dest, name)
		if e = os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			return "", e
		}
		if e = os.WriteFile(file, data, 0600); e != nil {
			return "", e
		}
		inventory = append(inventory, archiveops.Entry{Path: name, SHA256: digest(data), Size: int64(len(data))})
	}
	if e = os.MkdirAll(filepath.Join(dest, "metadata"), 0700); e != nil {
		return "", e
	}
	if e = atomicJSON(filepath.Join(dest, "metadata", "media-inventory.json"), inventory); e != nil {
		return "", e
	}
	if b.change != nil {
		b.change()
	}
	return "fixture-mvcc", nil
}
func (b *fixtureBackend) RestoreDatabase(ctx context.Context, file string) (string, string, error) {
	raw, e := os.ReadFile(file)
	b.restored = raw
	return uuid.NewString(), "owned_fixture_copy", e
}
func fixtureService(t *testing.T) (*Service, *fixtureBackend, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "photo.jpg")
	require.NoError(t, os.WriteFile(source, []byte("state A"), 0600))
	program := filepath.Join(dir, "powerx")
	require.NoError(t, os.WriteFile(program, []byte("fixture program"), 0700))
	b := &fixtureBackend{source: source}
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "test", Version: "fixture-v1", EffectiveConfig: []byte("password: source-secret-marker"), Artifacts: map[string]string{"core/powerx": program}}, Options{Backend: b})
	require.NoError(t, s.initErr)
	t.Cleanup(s.Close)
	return s, b, source
}
func captureReady(t *testing.T, s *Service) Snapshot {
	t.Helper()
	j, e := s.Start(rootContext(), uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		got, e := s.Get(rootContext(), j.ID)
		if e == nil {
			j = got
		}
		return e == nil && (j.Status == "ready" || j.Status == "failed")
	}, 30*time.Second, 10*time.Millisecond)
	require.Equal(t, "ready", j.Status, j.Error)
	waitIdle(t, s)
	return j
}

func waitIdle(t *testing.T, s *Service) {
	t.Helper()
	require.Eventually(t, func() bool {
		unlock, err := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
		if err != nil {
			return false
		}
		unlock()
		return true
	}, 5*time.Second, 5*time.Millisecond)
}
func packageBytes(t *testing.T, s *Service, id string) []byte {
	t.Helper()
	file, cleanup, e := s.ExportFile(rootContext(), id, "independent-test-passphrase")
	require.NoError(t, e)
	defer cleanup()
	raw, e := os.ReadFile(file)
	require.NoError(t, e)
	return raw
}
func TestPortablePackageRestoresAWithoutSource(t *testing.T) {
	s, _, source := fixtureService(t)
	j := captureReady(t, s)
	require.Len(t, j.Components, 4)
	require.False(t, j.BusinessVerified)
	pkg := packageBytes(t, s, j.ID)
	require.NotContains(t, string(pkg), "source-secret-marker")
	require.NoError(t, os.WriteFile(source, []byte("state B"), 0600))
	target, b, _ := fixtureService(t)
	require.NotEqual(t, s.key, target.key)
	imported, e := target.Import(rootContext(), bytes.NewReader(pkg), "independent-test-passphrase")
	require.NoError(t, e)
	require.Equal(t, j.ID, imported.ID)
	s.Close()
	require.NoError(t, os.RemoveAll(s.cfg.Directory))
	require.NoError(t, os.Remove(s.cfg.Artifacts["core/powerx"]))
	r, e := target.StartRestore(rootContext(), j.ID, uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		got, e := target.GetRestore(rootContext(), r.ID)
		if e == nil {
			r = got
		}
		return e == nil && (r.Status == "prepared" || r.Status == "failed")
	}, 5*time.Second, 5*time.Millisecond)
	require.Equal(t, "prepared", r.Status, r.Error)
	require.False(t, r.BusinessVerified)
	require.False(t, r.CutoverAvailable)
	raw, e := os.ReadFile(filepath.Join(r.Directory, "media", "photo.jpg"))
	require.NoError(t, e)
	require.Equal(t, "state A", string(raw))
	require.Equal(t, "database:state A", string(b.restored))
	raw, e = os.ReadFile(source)
	require.NoError(t, e)
	require.Equal(t, "state B", string(raw))
}
func TestRejectsWrongPasswordTamperingTruncation(t *testing.T) {
	s, _, _ := fixtureService(t)
	j := captureReady(t, s)
	pkg := packageBytes(t, s, j.ID)
	for _, kind := range []string{"password", "tamper", "truncated", "appended"} {
		t.Run(kind, func(t *testing.T) {
			target, _, _ := fixtureService(t)
			raw := append([]byte{}, pkg...)
			pass := "independent-test-passphrase"
			switch kind {
			case "password":
				pass = "wrong-password-value"
			case "tamper":
				raw[len(raw)/2] ^= 1
			case "truncated":
				raw = raw[:len(raw)-10]
			case "appended":
				raw = append(raw, 1)
			}
			_, e := target.Import(rootContext(), bytes.NewReader(raw), pass)
			require.Error(t, e)
			ids, e := target.ids("snapshots")
			require.NoError(t, e)
			require.Empty(t, ids)
			entries, e := os.ReadDir(filepath.Join(target.cfg.Directory, "staging"))
			require.NoError(t, e)
			require.Empty(t, entries)
		})
	}
}
func TestEncryptedImportRejectsUnsafeTar(t *testing.T) {
	for _, kind := range []string{"traversal", "duplicate", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := fixtureService(t)
			var plain bytes.Buffer
			tw := tar.NewWriter(&plain)
			switch kind {
			case "traversal":
				require.NoError(t, tarBytes(tw, "../outside", []byte("bad")))
			case "duplicate":
				require.NoError(t, tarBytes(tw, "manifest.json", []byte("{}")))
				require.NoError(t, tarBytes(tw, "manifest.json", []byte("{}")))
			case "symlink":
				require.NoError(t, tw.WriteHeader(&tar.Header{Name: "media/link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/outside"}))
			}
			require.NoError(t, tw.Close())
			var encrypted bytes.Buffer
			require.NoError(t, encryptStream(&encrypted, &plain, "independent-test-passphrase"))
			_, e := s.Import(rootContext(), &encrypted, "independent-test-passphrase")
			require.Error(t, e)
		})
	}
}
func TestCaptureFailureAndConfigMutationCannotBecomeReady(t *testing.T) {
	for _, kind := range []string{"backend", "config", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			s, b, _ := fixtureService(t)
			s.captureRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
			switch kind {
			case "backend":
				b.fail = true
			case "config":
				file := filepath.Join(t.TempDir(), "host-values")
				require.NoError(t, os.WriteFile(file, []byte("A"), 0600))
				s.cfg.ConfigFiles = map[string]string{"host-values": file}
				b.change = func() { _ = os.WriteFile(file, []byte(uuid.NewString()), 0600) }
			case "symlink":
				link := filepath.Join(t.TempDir(), "link")
				require.NoError(t, os.Symlink(s.cfg.Artifacts["core/powerx"], link))
				s.cfg.Artifacts["core/powerx"] = link
			}
			j, e := s.Start(rootContext(), uuid.NewString())
			require.NoError(t, e)
			require.Eventually(t, func() bool { j, e = s.Get(rootContext(), j.ID); return e == nil && j.Status == "failed" }, 5*time.Second, 5*time.Millisecond)
			_, _, e = s.ExportFile(rootContext(), j.ID, "independent-test-passphrase")
			require.Error(t, e)
		})
	}
}
func TestRootPolicyAndIdempotency(t *testing.T) {
	s, b, _ := fixtureService(t)
	_, e := s.Overview(context.Background())
	require.Error(t, e)
	ctx := rootContext()
	reqctx.GetClaims(ctx).Issuer = "powerx-sts"
	_, e = s.Overview(ctx)
	require.Error(t, e)
	p, e := s.SavePolicy(rootContext(), Policy{Enabled: true, IntervalMinutes: 60, RetentionDays: 1, RetentionCount: 2})
	require.NoError(t, e)
	got, e := s.policy()
	require.NoError(t, e)
	require.Equal(t, p, got)
	_, e = s.SavePolicy(rootContext(), Policy{IntervalMinutes: 20160, RetentionDays: 7, RetentionCount: 168})
	require.Error(t, e)
	key := uuid.NewString()
	j, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Eventually(t, func() bool { j, e = s.Get(rootContext(), j.ID); return e == nil && j.Status == "ready" }, 5*time.Second, 5*time.Millisecond)
	waitIdle(t, s)
	again, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Equal(t, j.ID, again.ID)
	require.Equal(t, int32(1), b.captures.Load())
}
func TestMissingContentBlocksRestore(t *testing.T) {
	s, _, _ := fixtureService(t)
	j := captureReady(t, s)
	require.NoError(t, os.Remove(filepath.Join(s.data(j.ID), "media", "photo.jpg")))
	_, _, e := s.ExportFile(rootContext(), j.ID, "independent-test-passphrase")
	require.Error(t, e)
	r, e := s.StartRestore(rootContext(), j.ID, uuid.NewString())
	require.NoError(t, e)
	require.Eventually(t, func() bool { r, e = s.GetRestore(rootContext(), r.ID); return e == nil && r.Status == "failed" }, 5*time.Second, 5*time.Millisecond)
	require.Empty(t, r.DatabaseCopyID)
}
func TestCryptoChunkBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, chunkSize, chunkSize + 1} {
		data := strings.Repeat("x", n)
		var encrypted, plain bytes.Buffer
		require.NoError(t, encryptStream(&encrypted, strings.NewReader(data), "independent-test-passphrase"))
		require.NoError(t, decryptStream(&plain, &encrypted, "independent-test-passphrase"))
		require.Equal(t, data, plain.String())
	}
	var out bytes.Buffer
	require.Error(t, encryptStream(&out, strings.NewReader("a"), "short"))
}

func TestActiveOnlineCaptureReplay(t *testing.T) {
	s, b, _ := fixtureService(t)
	done := make(chan struct{})
	var closeOnce atomic.Bool
	t.Cleanup(func() {
		if closeOnce.CompareAndSwap(false, true) {
			close(done)
		}
	})
	b.change = func() { <-done }
	key := uuid.NewString()
	j, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Eventually(t, func() bool { return b.captures.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	again, e := s.Start(rootContext(), key)
	require.NoError(t, e)
	require.Equal(t, j.ID, again.ID)
	require.Equal(t, int32(1), b.captures.Load())
	if closeOnce.CompareAndSwap(false, true) {
		close(done)
	}
	waitIdle(t, s)

}
func TestProgramInternalLinksMaterializeAndExternalLinksFail(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(source, "module.js"), []byte("same module"), 0600))
	require.NoError(t, os.Symlink("module.js", filepath.Join(source, "alias.js")))
	target := filepath.Join(t.TempDir(), "program")
	require.NoError(t, copyTree(context.Background(), source, target))
	info, e := os.Lstat(filepath.Join(target, "alias.js"))
	require.NoError(t, e)
	require.True(t, info.Mode().IsRegular())
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(source, "escape")))
	require.Error(t, copyTree(context.Background(), source, filepath.Join(t.TempDir(), "rejected")))
}
func TestRestartMarksInterruptedTasksWithoutDeletingCandidates(t *testing.T) {
	s, b, _ := fixtureService(t)
	id := uuid.NewString()
	j := Snapshot{ID: id, Status: "capturing", CreatedAt: s.opts.Now(), UpdatedAt: s.opts.Now()}
	require.NoError(t, atomicJSON(s.record("snapshots", id), j))
	restoreID := uuid.NewString()
	directory := filepath.Join(s.cfg.Directory, "restores", restoreID)
	require.NoError(t, os.Mkdir(directory, 0700))
	r := Restore{ID: restoreID, SnapshotID: id, Status: "running", Directory: directory}
	require.NoError(t, atomicJSON(s.record("restores", restoreID), r))
	s.Close()
	restarted := NewService(context.Background(), s.cfg, Options{Backend: b})
	defer restarted.Close()
	snapshot, e := restarted.Get(rootContext(), id)
	require.NoError(t, e)
	require.Equal(t, "interrupted", snapshot.Status)
	restored, e := restarted.GetRestore(rootContext(), restoreID)
	require.NoError(t, e)
	require.Equal(t, "interrupted", restored.Status)
	require.DirExists(t, directory)
}

func TestRestoreKeepsConfigurationEncrypted(t *testing.T) {
	s, _, _ := fixtureService(t)
	j := captureReady(t, s)
	key := uuid.NewString()
	r, e := s.StartRestore(rootContext(), j.ID, key)
	require.NoError(t, e)
	again, e := s.StartRestore(rootContext(), j.ID, key)
	require.NoError(t, e)
	require.Equal(t, r.ID, again.ID)
	require.Eventually(t, func() bool {
		r, e = s.GetRestore(rootContext(), r.ID)
		return e == nil && (r.Status == "prepared" || r.Status == "failed")
	}, 30*time.Second, 10*time.Millisecond)
	require.Equal(t, "prepared", r.Status, r.Error)
	require.NoFileExists(t, filepath.Join(r.Directory, "configuration", "source-config.private.json"))
	raw, e := os.ReadFile(filepath.Join(r.Directory, "configuration", "runtime.enc"))
	require.NoError(t, e)
	require.NotContains(t, string(raw), "source-secret-marker")
	require.FileExists(t, filepath.Join(r.Directory, "configuration", "key.enc"))
}

func TestInventoryMismatchCannotBeExportedEvenWithUpdatedFileDigest(t *testing.T) {
	s, _, _ := fixtureService(t)
	j := captureReady(t, s)
	m, e := s.manifest(j.ID)
	require.NoError(t, e)
	file := filepath.Join(s.data(j.ID), "metadata", "media-inventory.json")
	require.NoError(t, atomicJSON(file, []archiveops.Entry{}))
	entry, e := fileEntry(rootContext(), file, "metadata/media-inventory.json")
	require.NoError(t, e)
	for i := range m.Files {
		if m.Files[i].Path == entry.Path {
			m.Files[i] = entry
		}
	}
	require.NoError(t, atomicJSON(filepath.Join(s.data(j.ID), "manifest.json"), m))
	_, _, e = s.ExportFile(rootContext(), j.ID, "independent-test-passphrase")
	require.ErrorContains(t, e, "媒体引用清单")
}

func TestSpaceAndStrictMetadataPrechecks(t *testing.T) {
	require.Error(t, checkSpace(uint64(spaceReserve), 1))
	require.NoError(t, checkSpace(uint64(spaceReserve)+1, 1))
	_, e := packageSize(Manifest{Files: []Entry{{Size: maxPackageBytes}}})
	require.Error(t, e)
	file := filepath.Join(t.TempDir(), "metadata.json")
	require.NoError(t, os.WriteFile(file, []byte(`{} {}`), 0600))
	require.Error(t, readJSON(file, &map[string]any{}))
}

func TestSchedulerUsesStableAnchorAndReportsBlockedRun(t *testing.T) {
	s, _, _ := fixtureService(t)
	now := time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)
	s.opts.Now = func() time.Time { return now }
	_, e := s.SavePolicy(rootContext(), Policy{Enabled: true, IntervalMinutes: 60, RetentionDays: 1, RetentionCount: 2})
	require.NoError(t, e)
	require.NoError(t, s.Tick(context.Background()))
	out, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.Empty(t, out.Snapshots)
	require.Equal(t, now.Add(time.Hour), *out.NextRunAt)
	now = now.Add(30 * time.Minute)
	out, e = s.Overview(rootContext())
	require.NoError(t, e)
	require.Equal(t, time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC), *out.NextRunAt)
	now = now.Add(31 * time.Minute)
	require.NoError(t, os.Remove(s.cfg.Artifacts["core/powerx"]))
	require.Error(t, s.Tick(context.Background()))
	out, e = s.Overview(rootContext())
	require.NoError(t, e)
	require.NotEmpty(t, out.SchedulerError)
}

func TestCaptureRejectsSourceContainingItsOwnOutput(t *testing.T) {
	for _, kind := range []string{"program", "media"} {
		t.Run(kind, func(t *testing.T) {
			s, _, _ := fixtureService(t)
			if kind == "program" {
				s.cfg.Artifacts = map[string]string{"self": s.cfg.Directory}
			} else {
				s.cfg.MediaRoot = s.cfg.Directory
			}
			j, e := s.Start(rootContext(), uuid.NewString())
			require.NoError(t, e)
			require.Eventually(t, func() bool {
				got, err := s.Get(rootContext(), j.ID)
				if err == nil {
					j = got
				}
				return err == nil && j.Status == "failed"
			}, 5*time.Second, 10*time.Millisecond)
			require.Contains(t, j.Error, "不能包含快照目录")
			waitIdle(t, s)
			_, e = os.Stat(s.data(j.ID))
			require.ErrorIs(t, e, os.ErrNotExist)
		})
	}
}

func TestMissingMasterKeyDoesNotSilentlyReplaceExistingKey(t *testing.T) {
	s, _, _ := fixtureService(t)
	captureReady(t, s)
	s.Close()
	require.NoError(t, os.Remove(filepath.Join(s.cfg.Directory, "master.key")))
	reopened := NewService(context.Background(), s.cfg, Options{})
	defer reopened.Close()
	require.ErrorContains(t, reopened.initErr, "主密钥丢失")
	_, e := os.Stat(filepath.Join(s.cfg.Directory, "master.key"))
	require.ErrorIs(t, e, os.ErrNotExist)
}

func TestExportRejectsMismatchedConfigurationKey(t *testing.T) {
	s, _, _ := fixtureService(t)
	j := captureReady(t, s)
	wrapped, e := encryptConfig(bytes.Repeat([]byte{1}, 32), s.key)
	require.NoError(t, e)
	require.NoError(t, atomicFile(filepath.Join(s.data(j.ID), "key.enc"), wrapped))
	_, _, e = s.ExportFile(rootContext(), j.ID, "independent-test-passphrase")
	require.ErrorContains(t, e, "配置密文")
}

func TestRelativeArtifactUnderLogicalWorkingDirectory(t *testing.T) {
	physical, e := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, e)
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	require.NoError(t, os.Symlink(physical, alias))
	source := filepath.Join(physical, "plugin", "backend")
	require.NoError(t, os.MkdirAll(source, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "plugin"), []byte("installed executable"), 0700))
	t.Chdir(physical)
	// 模拟用户从 /var/... 工作目录启动；逻辑 PWD 与规范路径指向同一 inode。
	t.Setenv("PWD", alias)
	cwd, e := os.Getwd()
	require.NoError(t, e)
	require.Equal(t, alias, cwd)
	target := filepath.Join(t.TempDir(), "copied")
	require.NoError(t, copyTree(context.Background(), "plugin/backend", target))
	content, e := os.ReadFile(filepath.Join(target, "plugin"))
	require.NoError(t, e)
	require.Equal(t, "installed executable", string(content))
	// 规则仍禁止把快照目录放进来源，不能因两个别名而绕过自身备份检查。
	require.Error(t, rejectContained("plugin/backend", filepath.Join(source, "snapshots")))
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("private outside"), 0600))
	require.NoError(t, os.Symlink(outside, filepath.Join(source, "escape")))
	require.ErrorContains(t, copyTree(context.Background(), "plugin/backend", filepath.Join(t.TempDir(), "rejected")), "软链接越界")
}
