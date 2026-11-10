// Package storagerecovery 保存不属于业务恢复载荷的安装级恢复栅栏。
package storagerecovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// v7 adds typed quarantine evidence without representing damaged files as usable backups. Older executors
// must fail closed rather than resume pre-recovery requests after a rollback.
const Protocol = "powerx.storage-recovery/v7"
const ResolutionProtocol = "powerx.storage-recovery/v6"
const ImportProtocol = "powerx.storage-recovery/v5"
const PreviousProtocol = "powerx.storage-recovery/v4"
const OlderProtocol = "powerx.storage-recovery/v3"
const IntermediateProtocol = "powerx.storage-recovery/v2"
const LegacyProtocol = "powerx.storage-recovery/v1"

var ErrUnavailable = errors.New("storage recovery anchor unavailable")
var ErrBusy = errors.New("storage recovery anchor busy")

type Epoch struct {
	Protocol     string    `json:"protocol"`
	Installation string    `json:"installation"`
	AnchorID     string    `json:"anchor_id"`
	Epoch        string    `json:"epoch"`
	JobID        string    `json:"job_id"`
	SnapshotID   string    `json:"snapshot_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type binding struct {
	Protocol          string `json:"protocol"`
	Installation      string `json:"installation"`
	AnchorID          string `json:"anchor_id"`
	RetentionLedgerID string `json:"retention_ledger_id,omitempty"`
}
type Store struct{ directory, installation string }

// Open 不创建或重建丢失证明。directory 必须是已登记、恢复不能覆盖的维护根目录。
func Open(directory, installation string) (*Store, error) {
	if !filepath.IsAbs(directory) || !instanceidentity.Valid(installation) {
		return nil, ErrUnavailable
	}
	s := &Store{filepath.Clean(directory), installation}
	if e := s.WithEpoch(context.Background(), func(Epoch) error { return nil }); e != nil {
		return nil, e
	}
	return s, nil
}

// Initialize 仅供安装维护协议显式升级；标记该协议后必须改用 Open，不能自动重建。
func Initialize(directory, installation string) (*Store, error) {
	if !filepath.IsAbs(directory) || !instanceidentity.Valid(installation) {
		return nil, ErrUnavailable
	}
	s := &Store{filepath.Clean(directory), installation}
	e := s.locked(context.Background(), true, func(fd int) error {
		raw, e := read(fd, "storage-recovery-binding.json")
		if errors.Is(e, os.ErrNotExist) {
			if _, e = read(fd, "storage-recovery-epoch.json"); !errors.Is(e, os.ErrNotExist) {
				return ErrUnavailable
			}
			b := binding{Protocol: Protocol, Installation: installation, AnchorID: uuid.NewString(), RetentionLedgerID: uuid.NewString()}
			if e = initializeRetentionLedger(fd, b); e != nil {
				return e
			}
			if e = write(fd, "storage-recovery-binding.json", b); e != nil {
				return e
			}
			return write(fd, "storage-recovery-epoch.json", Epoch{Protocol: Protocol, Installation: installation, AnchorID: b.AnchorID})
		}
		if e != nil || len(raw) == 0 {
			return ErrUnavailable
		}
		_, e = s.readEpoch(fd)
		return e
	})
	if e != nil {
		return nil, e
	}
	return s, nil
}
func strict(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrUnavailable
	}
	return nil
}
func validUUID(id string) bool { v, e := uuid.Parse(id); return e == nil && v.String() == id }
func (s *Store) readEpoch(fd int) (Epoch, error) {
	return s.readEpochProtocols(fd, false)
}
func (s *Store) readEpochProtocols(fd int, upgrading bool) (Epoch, error) {
	var b binding
	var out Epoch
	accepted := func(p string) bool {
		return p == Protocol || upgrading && (p == ResolutionProtocol || p == ImportProtocol || p == PreviousProtocol || p == OlderProtocol || p == IntermediateProtocol || p == LegacyProtocol)
	}
	raw, e := read(fd, "storage-recovery-binding.json")
	if e != nil || strict(raw, &b) != nil || !accepted(b.Protocol) || b.Installation != s.installation || !validUUID(b.AnchorID) {
		return out, ErrUnavailable
	}
	if b.Protocol == Protocol && !validUUID(b.RetentionLedgerID) {
		return out, ErrUnavailable
	}
	raw, e = read(fd, "storage-recovery-epoch.json")
	if e != nil || strict(raw, &out) != nil || !accepted(out.Protocol) || out.Installation != b.Installation || out.AnchorID != b.AnchorID {
		return out, ErrUnavailable
	}
	if out.Epoch == "" {
		if out.JobID != "" || out.SnapshotID != "" || !out.CreatedAt.IsZero() {
			return out, ErrUnavailable
		}
	} else if !validUUID(out.Epoch) || !validUUID(out.JobID) || !validUUID(out.SnapshotID) || out.CreatedAt.IsZero() {
		return out, ErrUnavailable
	}
	return out, nil
}

// UpgradeV1 兼容升级现有 v1/v2/v3/v4 完整独立证明，不创建代次。凭据先落盘再登记
// v5 绑定；登记后丢失凭据禁止重建。升级仍要求旧写入者已停用。
// 接续混合版本；旧程序从第一笔写入起即失败关闭，不能执行旧版确认流程。
func UpgradeV1(directory, installation string) (*Store, error) {
	if !filepath.IsAbs(directory) || !instanceidentity.Valid(installation) {
		return nil, ErrUnavailable
	}
	s := &Store{filepath.Clean(directory), installation}
	err := s.locked(context.Background(), true, func(fd int) error {
		epoch, e := s.readEpochProtocols(fd, true)
		if e != nil {
			return e
		}
		var b binding
		raw, e := read(fd, "storage-recovery-binding.json")
		if e != nil || strict(raw, &b) != nil {
			return ErrUnavailable
		}
		if b.RetentionLedgerID == "" {
			b.RetentionLedgerID, e = prepareRetentionLedger(fd, b)
			if e != nil {
				return e
			}
		} else {
			d, err := readRetentionLedgerVersion(fd, b, b.Protocol != Protocol)
			if err != nil {
				return err
			}
			if d.Protocol != retentionProtocol {
				d.Protocol = retentionProtocol
				if d.Imports == nil {
					d.Imports = map[string]RetentionImport{}
				}
				if err = saveRetentionLedger(fd, d); err != nil {
					return err
				}
			}
		}
		b.Protocol = Protocol
		if e = write(fd, "storage-recovery-binding.json", b); e != nil {
			return e
		}
		epoch.Protocol = Protocol
		return write(fd, "storage-recovery-epoch.json", epoch)
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// WithEpoch 在整个有界受控步骤持有共享文件锁；恢复发布不得越过已进入的步骤。
func (s *Store) WithEpoch(ctx context.Context, fn func(Epoch) error) error {
	if s == nil || fn == nil {
		return ErrUnavailable
	}
	return s.locked(ctx, false, func(fd int) error {
		e, err := s.readEpoch(fd)
		if err != nil {
			return err
		}
		return fn(e)
	})
}
func (s *Store) Read(ctx context.Context) (Epoch, error) {
	var out Epoch
	e := s.WithEpoch(ctx, func(v Epoch) error { out = v; return nil })
	return out, e
}

// Begin 在触碰数据库前提交新代次；同一恢复任务重复进入保持原代次，不回退或清除。
func (s *Store) Begin(ctx context.Context, job, snapshot string) (Epoch, error) {
	return s.BeginChecked(ctx, job, snapshot, nil)
}

// BeginChecked keeps the independent root locked while the maintenance caller
// verifies current SQL coverage. It runs before any database overwrite.
func (s *Store) BeginChecked(ctx context.Context, job, snapshot string, check func(map[string]RetentionRecord) error) (Epoch, error) {
	var out Epoch
	if !validUUID(job) || !validUUID(snapshot) {
		return out, ErrUnavailable
	}
	e := s.locked(ctx, true, func(fd int) error {
		var e error
		out, e = s.readEpoch(fd)
		if e != nil {
			return e
		}
		// A managed SQL overwrite must not proceed with unreadable independent
		// UUID history. Ordinary storage admission does not depend on this ledger.
		var b binding
		raw, e := read(fd, "storage-recovery-binding.json")
		if e != nil || strict(raw, &b) != nil {
			return ErrUnavailable
		}
		d, e := readRetentionLedger(fd, b)
		if e != nil {
			return e
		}
		if check != nil {
			if e = check(d.Records); e != nil {
				return e
			}
		}
		if out.JobID == job {
			if out.SnapshotID != snapshot {
				return ErrUnavailable
			}
			return nil
		}
		out = Epoch{Protocol: Protocol, Installation: s.installation, AnchorID: out.AnchorID, Epoch: uuid.NewString(), JobID: job, SnapshotID: snapshot, CreatedAt: time.Now().UTC()}
		return write(fd, "storage-recovery-epoch.json", out)
	})
	return out, e
}
func (s *Store) locked(ctx context.Context, exclusive bool, fn func(int) error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	fd, e := unix.Open(s.directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return ErrUnavailable
	}
	defer unix.Close(fd)
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Uid != uint32(os.Geteuid()) || stat.Mode&0077 != 0 {
		return ErrUnavailable
	}
	lock, e := unix.Openat(fd, "storage-recovery.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	defer unix.Close(lock)
	if unix.Fstat(lock, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 || stat.Mode&0077 != 0 {
		return ErrUnavailable
	}
	mode := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		mode = unix.LOCK_EX | unix.LOCK_NB
	}
	if e = unix.Flock(lock, mode); e != nil {
		if errors.Is(e, unix.EWOULDBLOCK) {
			return ErrBusy
		}
		return ErrUnavailable
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	return fn(fd)
}
func read(fd int, name string) ([]byte, error) {
	n, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(n), name)
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(n, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0077 != 0 || st.Size < 2 || st.Size > 8192 {
		return nil, ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(f, 8193))
	if e != nil || len(raw) > 8192 {
		return nil, ErrUnavailable
	}
	return raw, nil
}
func write(fd int, name string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || len(raw) > 8192 {
		return ErrUnavailable
	}
	return writeBytes(fd, name, raw)
}
func writeBytes(fd int, name string, raw []byte) error {
	nameTmp := ".storage-recovery-" + uuid.NewString()
	n, e := unix.Openat(fd, nameTmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	f := os.NewFile(uintptr(n), nameTmp)
	defer unix.Unlinkat(fd, nameTmp, 0)
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return ErrUnavailable
	}
	if unix.Renameat(fd, nameTmp, fd, name) != nil || unix.Fsync(fd) != nil {
		return ErrUnavailable
	}
	return nil
}
