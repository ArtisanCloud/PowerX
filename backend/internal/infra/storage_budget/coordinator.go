// Package storagebudget 提供独立于业务数据库的同盘空间预留与崩溃恢复账本。
package storagebudget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var ErrInvalid = errors.New("storage budget invalid")
var ErrUnavailable = errors.New("storage budget unavailable")
var ErrInsufficient = errors.New("storage budget insufficient")
var ErrBusy = errors.New("storage budget busy")

// ErrDeviceChanged 表示持久账本与当前设备不同，且不能安全核验为同一文件系统。
var ErrDeviceChanged = errors.New("storage budget device changed; verified filesystem rebind required")

type Config struct {
	Directory         string           `yaml:"directory" json:"directory"`
	FilesystemPath    string           `yaml:"filesystem_path" json:"filesystem_path"`
	HostID            string           `yaml:"host_id" json:"host_id"`
	FilesystemID      string           `yaml:"filesystem_id" json:"filesystem_id"`
	InstanceID        string           `yaml:"instance_id" json:"instance_id"`
	MinFreeBytes      int64            `yaml:"min_free_bytes" json:"min_free_bytes"`
	CriticalFreeBytes int64            `yaml:"critical_free_bytes" json:"critical_free_bytes"`
	MaxUsedBytes      int64            `yaml:"max_used_bytes" json:"max_used_bytes"`
	MaxConcurrent     int              `yaml:"max_concurrent" json:"max_concurrent"`
	Now               func() time.Time `yaml:"-" json:"-"`
}
type Reservation struct {
	ID           string    `json:"id"`
	InstanceID   string    `json:"instance_id"`
	Operation    string    `json:"operation"`
	PeakBytes    int64     `json:"peak_bytes"`
	WrittenBytes int64     `json:"written_bytes"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type Snapshot struct {
	HostID            string        `json:"host_id"`
	FilesystemID      string        `json:"filesystem_id"`
	Revision          int64         `json:"revision"`
	AvailableBytes    int64         `json:"available_bytes"`
	UsedBytes         int64         `json:"used_bytes"`
	ReservedBytes     int64         `json:"reserved_bytes"`
	MinFreeBytes      int64         `json:"min_free_bytes"`
	CriticalFreeBytes int64         `json:"critical_free_bytes"`
	MaxUsedBytes      int64         `json:"max_used_bytes"`
	Active            []Reservation `json:"active"`
}
type ledger struct {
	Control          *ControlProjection     `json:"control,omitempty"`
	Version          int                    `json:"version"`
	HostID           string                 `json:"host_id"`
	FilesystemID     string                 `json:"filesystem_id"`
	Device           uint64                 `json:"device"`
	Revision         int64                  `json:"revision"`
	Reservations     map[string]Reservation `json:"reservations"`
	Policy           *BudgetPolicy          `json:"policy,omitempty"`
	DeviceRebindings []deviceRebinding      `json:"device_rebindings,omitempty"`
}

// BudgetPolicy 在所有同盘实例之间共享，不能各自解释运行余量。
type BudgetPolicy struct {
	Revision          int64 `json:"revision"`
	MinFreeBytes      int64 `json:"min_free_bytes"`
	CriticalFreeBytes int64 `json:"critical_free_bytes"`
	MaxUsedBytes      int64 `json:"max_used_bytes"`
	MaxConcurrent     int   `json:"max_concurrent"`
}
type Coordinator struct {
	authorityMu      sync.RWMutex
	authority        BudgetAuthority
	cfg              Config
	device           uint64
	inode            uint64
	initializing     bool
	verifyFilesystem func(string, string) bool
}
type Lease struct {
	c        *Coordinator
	id       string
	mu       sync.Mutex
	released bool
}

func New(c Config) (*Coordinator, error) {
	return newCoordinator(c, stableFilesystemMatches)
}

func newCoordinator(c Config, verify func(string, string) bool) (*Coordinator, error) {
	if !filepath.IsAbs(c.Directory) || !filepath.IsAbs(c.FilesystemPath) || c.HostID == "" || c.FilesystemID == "" || c.InstanceID == "" {
		return nil, ErrUnavailable
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.MinFreeBytes <= 0 {
		c.MinFreeBytes = 20 << 30
	}
	if c.CriticalFreeBytes <= 0 {
		c.CriticalFreeBytes = 10 << 30
	}
	if c.MaxConcurrent <= 0 {
		c.MaxConcurrent = 1
	}
	if c.CriticalFreeBytes >= c.MinFreeBytes || c.MaxConcurrent > 16 {
		return nil, ErrUnavailable
	}
	if err := os.MkdirAll(c.Directory, 0700); err != nil {
		return nil, ErrUnavailable
	}
	canonical, err := filepath.EvalSymlinks(c.Directory)
	if err != nil {
		return nil, ErrUnavailable
	}
	c.Directory = canonical
	var dir, root unix.Stat_t
	if unix.Lstat(c.Directory, &dir) != nil || dir.Mode&unix.S_IFMT != unix.S_IFDIR || dir.Mode&0077 != 0 || int(dir.Uid) != os.Geteuid() {
		return nil, ErrUnavailable
	}
	if unix.Stat(c.FilesystemPath, &root) != nil || root.Dev != dir.Dev {
		return nil, ErrUnavailable
	}
	for parent := c.Directory; parent != "/"; parent = filepath.Dir(parent) {
		var st unix.Stat_t
		if unix.Lstat(parent, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
			return nil, ErrUnavailable
		}
	}
	coordinator := &Coordinator{cfg: c, device: uint64(dir.Dev), inode: dir.Ino, initializing: true, verifyFilesystem: verify}
	// 首次注册即持久化权威策略，避免尚未执行任务时各实例读取不同默认值。
	if err := coordinator.locked(context.Background(), true, func(*ledger) error { return nil }); err != nil {
		return nil, err
	}
	coordinator.initializing = false
	return coordinator, nil
}
func (c *Coordinator) locked(ctx context.Context, write bool, fn func(*ledger) error) error {
	directoryFD, err := c.openDirectory()
	if err != nil {
		return err
	}
	defer unix.Close(directoryFD)
	fd, err := unix.Openat(directoryFD, "budget.lock", unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return ErrUnavailable
	}
	defer unix.Close(fd)
	var lockStat unix.Stat_t
	if unix.Fstat(fd, &lockStat) != nil || lockStat.Mode&unix.S_IFMT != unix.S_IFREG || lockStat.Nlink != 1 || int(lockStat.Uid) != os.Geteuid() {
		return ErrUnavailable
	}
	if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return ErrBusy
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	state := ledger{Version: 1, HostID: c.cfg.HostID, FilesystemID: c.cfg.FilesystemID, Device: c.device, Reservations: map[string]Reservation{}}
	raw, readErr := c.readPrivate("budget.json", 256<<10)
	if readErr == nil {
		if json.Unmarshal(raw, &state) != nil {
			return ErrUnavailable
		}
		if state.Version != 1 || state.HostID != c.cfg.HostID || state.FilesystemID != c.cfg.FilesystemID || state.Reservations == nil || len(state.Reservations) > 64 || len(state.DeviceRebindings) > 8 {
			return ErrUnavailable
		}
		if state.Device != c.device {
			if !c.initializing || !write || len(state.Reservations) != 0 || c.verifyFilesystem == nil || !c.verifyFilesystem(c.cfg.FilesystemPath, c.cfg.FilesystemID) || !c.rebindPoliciesSafe() {
				return ErrDeviceChanged
			}
			state.DeviceRebindings = append(state.DeviceRebindings, deviceRebinding{PreviousDevice: state.Device, Device: c.device, InstanceID: c.cfg.InstanceID, At: c.cfg.Now().UTC(), Reason: "same_stable_filesystem_no_reservations_cleanup_disabled"})
			if len(state.DeviceRebindings) > 8 {
				state.DeviceRebindings = state.DeviceRebindings[len(state.DeviceRebindings)-8:]
			}
			state.Device = c.device
		}
	} else if !os.IsNotExist(readErr) {
		return ErrUnavailable
	}
	for id, r := range state.Reservations {
		if !c.cfg.Now().Before(r.ExpiresAt) {
			delete(state.Reservations, id)
		}
	}
	if state.Policy == nil {
		state.Policy = &BudgetPolicy{Revision: 1, MinFreeBytes: c.cfg.MinFreeBytes, CriticalFreeBytes: c.cfg.CriticalFreeBytes, MaxUsedBytes: c.cfg.MaxUsedBytes, MaxConcurrent: c.cfg.MaxConcurrent}
	}
	if !validBudget(*state.Policy) {
		return ErrUnavailable
	}
	if err = fn(&state); err != nil {
		return err
	}
	if !write {
		return nil
	}
	state.Revision++
	encoded, err := json.Marshal(state)
	if err != nil {
		return ErrUnavailable
	}
	return c.writePrivate("budget.json", encoded)
}
func (c *Coordinator) snapshot(state *ledger) (Snapshot, error) {
	var fs unix.Statfs_t
	if unix.Statfs(c.cfg.FilesystemPath, &fs) != nil || fs.Bsize <= 0 {
		return Snapshot{}, ErrUnavailable
	}
	out := Snapshot{HostID: c.cfg.HostID, FilesystemID: c.cfg.FilesystemID, Revision: state.Revision, AvailableBytes: int64(fs.Bavail) * int64(fs.Bsize), UsedBytes: int64(fs.Blocks-fs.Bfree) * int64(fs.Bsize), MinFreeBytes: state.Policy.MinFreeBytes, CriticalFreeBytes: state.Policy.CriticalFreeBytes, MaxUsedBytes: state.Policy.MaxUsedBytes, Active: []Reservation{}}
	for _, r := range state.Reservations {
		if r.PeakBytes < 0 || r.WrittenBytes < 0 || r.WrittenBytes > r.PeakBytes {
			return out, ErrUnavailable
		}
		out.ReservedBytes += r.PeakBytes - r.WrittenBytes
		out.Active = append(out.Active, r)
	}
	return out, nil
}
func (c *Coordinator) Status(ctx context.Context) (Snapshot, error) {
	var out Snapshot
	err := c.withAuthorizedBudget(ctx, false, func(s *ledger) error { var e error; out, e = c.snapshot(s); return e })
	return out, err
}
func (c *Coordinator) Reserve(ctx context.Context, operation string, peak int64, ttl time.Duration) (*Lease, error) {
	if peak < 0 || peak > 1<<50 || ttl <= 0 || ttl > 24*time.Hour || len(operation) > 80 || operation == "" {
		return nil, ErrUnavailable
	}
	id := uuid.NewString()
	err := c.withAuthorizedBudget(ctx, true, func(s *ledger) error {
		current, e := c.snapshot(s)
		if e != nil {
			return e
		}
		if len(s.Reservations) >= s.Policy.MaxConcurrent {
			return ErrBusy
		}
		if current.AvailableBytes < peak+current.ReservedBytes+s.Policy.MinFreeBytes || current.AvailableBytes < s.Policy.CriticalFreeBytes {
			return ErrInsufficient
		}
		if s.Policy.MaxUsedBytes > 0 && current.UsedBytes+current.ReservedBytes+peak > s.Policy.MaxUsedBytes {
			return ErrInsufficient
		}
		now := c.cfg.Now().UTC()
		s.Reservations[id] = Reservation{ID: id, InstanceID: c.cfg.InstanceID, Operation: operation, PeakBytes: peak, CreatedAt: now, ExpiresAt: now.Add(ttl)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Lease{c: c, id: id}, nil
}

// Progress 将已经占用文件系统的部分转为实占，不能重复扣减预留；扩张需重新准入。
func (l *Lease) Progress(ctx context.Context, written, peak int64, ttl time.Duration) error {
	if l == nil {
		return nil
	}
	if written < 0 || peak < written || peak > 1<<50 || ttl <= 0 || ttl > 24*time.Hour {
		return ErrUnavailable
	}
	return l.c.withAuthorizedBudget(ctx, true, func(s *ledger) error {
		r, ok := s.Reservations[l.id]
		if !ok || r.InstanceID != l.c.cfg.InstanceID {
			return ErrUnavailable
		}
		delete(s.Reservations, l.id)
		current, e := l.c.snapshot(s)
		if e != nil {
			return e
		}
		if current.AvailableBytes < current.ReservedBytes+peak-written+s.Policy.MinFreeBytes || (s.Policy.MaxUsedBytes > 0 && current.UsedBytes+current.ReservedBytes+peak-written > s.Policy.MaxUsedBytes) {
			return ErrInsufficient
		}
		r.PeakBytes = peak
		r.WrittenBytes = written
		r.ExpiresAt = l.c.cfg.Now().Add(ttl)
		s.Reservations[l.id] = r
		return nil
	})
}
func (l *Lease) Release(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	err := l.c.locked(ctx, true, func(s *ledger) error {
		if r, ok := s.Reservations[l.id]; ok && r.InstanceID != l.c.cfg.InstanceID {
			return ErrUnavailable
		}
		delete(s.Reservations, l.id)
		return nil
	})
	if err == nil {
		l.released = true
	}
	return err
}

// Document 在同一个主机锁内读改写有界运行元数据，不依赖业务数据库。
func (c *Coordinator) Document(ctx context.Context, name string, update func([]byte) ([]byte, error)) ([]byte, error) {
	return c.DocumentTransaction(ctx, name, func(raw []byte, _ func([]byte) error) ([]byte, error) {
		if update == nil {
			return nil, nil
		}
		return update(raw)
	})
}

// DocumentTransaction 在持锁期间先持久化意图，再提交外部有界事务及结果。
// persist 只在回调执行期间有效，不得留存或跨 goroutine 使用。
func (c *Coordinator) DocumentTransaction(ctx context.Context, name string, update func([]byte, func([]byte) error) ([]byte, error)) ([]byte, error) {
	if name != "execution-control-binding" && name != "execution-control-snapshot" && name != "system-retention-probe-binding" && name != "system-retention-probe-control" && name != "system-retention-discovery-binding" && name != "system-retention-discovery-control" && name != "database-retention-binding" && name != "database-retention-control" && name != "snapshot-policy-control" && name != "backup-policy-control" && name != "archive-policy-control" && name != "archive-target-control" && name != "lifecycle" && name != "alerts" && name != "ownership" && name != "database-retention" && name != "table-monitoring" && name != "table-monitoring-control" && name != "cleanup-health" && name != "database-backlog" && name != "system-table-retention" {
		return nil, ErrUnavailable
	}
	var out []byte
	err := c.locked(ctx, false, func(_ *ledger) error {
		filename, err := c.documentNameLocked(name)
		if err != nil {
			return err
		}
		raw, err := c.readPrivate(filename, 1<<20)
		if err != nil && !os.IsNotExist(err) {
			return ErrUnavailable
		}
		active := true
		defer func() { active = false }()
		persist := func(next []byte) error {
			if !active || len(next) > 1<<20 || !json.Valid(next) {
				return ErrUnavailable
			}
			return c.writePrivate(filename, next)
		}
		next, e := update(raw, persist)
		if e != nil {
			return e
		}
		if next == nil {
			out = raw
			return nil
		}
		if len(next) == 0 && len(raw) == 0 {
			out = raw
			return nil
		}
		if e = persist(next); e != nil {
			return e
		}
		out = next
		return nil
	})
	return out, err
}

func validBudget(p BudgetPolicy) bool {
	return p.Revision >= 1 && p.MinFreeBytes > 0 && p.MinFreeBytes <= 1<<50 && p.CriticalFreeBytes > 0 && p.CriticalFreeBytes < p.MinFreeBytes && p.MaxUsedBytes >= 0 && p.MaxUsedBytes <= 1<<50 && p.MaxConcurrent >= 1 && p.MaxConcurrent <= 16
}
func (c *Coordinator) Policy(ctx context.Context) (BudgetPolicy, error) {
	var out BudgetPolicy
	err := c.withAuthorizedBudget(ctx, false, func(s *ledger) error { out = *s.Policy; return nil })
	return out, err
}
func (c *Coordinator) SavePolicy(ctx context.Context, expected int64, p BudgetPolicy) (BudgetPolicy, error) {
	if c.authorityFence() != nil {
		return p, ErrControlRequired
	}
	binding, e := c.readControlBinding()
	if e != nil || binding != nil {
		return p, ErrControlRequired
	}
	p.Revision = expected + 1
	if !validBudget(p) {
		return p, ErrInvalid
	}
	err := c.locked(ctx, true, func(s *ledger) error {
		binding, e := c.readControlBinding()
		if e != nil || binding != nil {
			return ErrControlRequired
		}
		if s.Control != nil {
			return ErrControlRequired
		}
		if s.Policy.Revision != expected {
			return ErrBusy
		}
		s.Policy = &p
		return nil
	})
	return p, err
}

// ReservePathDefault 拒绝在未登记的另一块盘沿用当前盘预算。
func ReservePathDefault(ctx context.Context, path, operation string, peak int64) (*Lease, error) {
	defaultMu.RLock()
	c := defaultCoordinator
	defaultMu.RUnlock()
	if c == nil {
		return nil, nil
	}
	parent := filepath.Clean(path)
	for {
		var st unix.Stat_t
		err := unix.Stat(parent, &st)
		if err == nil {
			if uint64(st.Dev) != c.device {
				return nil, ErrUnavailable
			}
			break
		}
		if !os.IsNotExist(err) || parent == filepath.Dir(parent) {
			return nil, ErrUnavailable
		}
		parent = filepath.Dir(parent)
	}
	return c.Reserve(ctx, operation, peak, 2*time.Minute)
}

var defaultMu sync.RWMutex
var defaultCoordinator *Coordinator

func SetDefault(c *Coordinator) { defaultMu.Lock(); defaultCoordinator = c; defaultMu.Unlock() }

// ReserveDefault 仅在部署明确开启准入后使用；旧部署不静默改变行为。
func ReserveDefault(ctx context.Context, operation string, peak int64) (*Lease, error) {
	defaultMu.RLock()
	c := defaultCoordinator
	defaultMu.RUnlock()
	if c == nil {
		return nil, nil
	}
	lease, err := c.Reserve(ctx, operation, peak, 2*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, operation)
	}
	return lease, nil
}

func (c *Coordinator) ChangeDirectory() string { return c.cfg.Directory }

func DefaultConfig() *Config {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	if defaultCoordinator == nil {
		return nil
	}
	c := defaultCoordinator.cfg
	c.Now = nil
	return &c
}
