// Package storagelifecycle 执行部署登记的封存日志策略；业务数据不进入通用清理器。
package storagelifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var ErrBlocked = errors.New("storage lifecycle blocked")
var ErrConflict = errors.New("storage lifecycle revision conflict")
var ErrInvalid = errors.New("storage lifecycle invalid")

type Source struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	Scope           string   `json:"scope"`
	InstanceID      string   `json:"instance_id"`
	Directory       string   `json:"-"`
	ActiveFiles     []string `json:"-"`
	CurrentLogfiles string   `json:"-"`
	Format          string   `json:"format"`
	RequireArchive  bool     `json:"require_archive"`
	OwnerUID        uint32   `json:"-"`
	Fingerprint     string   `json:"-"`
	Supported       bool     `json:"supported"`
	Problem         string   `json:"problem,omitempty"`
}
type Policy struct {
	CleanupMode    string    `json:"cleanup_mode"`
	SourceID       string    `json:"source_id"`
	Revision       int64     `json:"revision"`
	Enabled        bool      `json:"enabled"`
	RetentionDays  int       `json:"retention_days"`
	MaxBytes       int64     `json:"max_bytes"`
	RequireArchive bool      `json:"require_archive"`
	Fingerprint    string    `json:"fingerprint"`
	OwnerInstance  string    `json:"owner_instance"`
	OperatorID     uint64    `json:"operator_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type Candidate struct {
	Name       string `json:"name"`
	Device     uint64 `json:"device"`
	Inode      uint64 `json:"inode"`
	Bytes      int64  `json:"bytes"`
	Allocated  int64  `json:"allocated"`
	ModifiedNS int64  `json:"modified_ns"`
	ChangedNS  int64  `json:"changed_ns"`
}
type Preview struct {
	SourceID       string         `json:"source_id"`
	PolicyRevision int64          `json:"policy_revision"`
	Candidates     []Candidate    `json:"-"`
	CandidateNames []string       `json:"candidate_names"`
	CandidateBytes int64          `json:"candidate_bytes"`
	Protected      map[string]int `json:"protected"`
	ObservedBytes  int64          `json:"observed_bytes"`
	OverBudget     bool           `json:"over_budget"`
	Complete       bool           `json:"complete"`
	Problem        string         `json:"problem,omitempty"`
}
type Run struct {
	RequestKey            string     `json:"request_key"`
	ID                    string     `json:"id"`
	SourceID              string     `json:"source_id"`
	PolicyRevision        int64      `json:"policy_revision"`
	InstanceID            string     `json:"instance_id"`
	State                 string     `json:"state"`
	StartedAt             time.Time  `json:"started_at"`
	FinishedAt            *time.Time `json:"finished_at"`
	DeletedFiles          int        `json:"deleted_files"`
	RemovedBytes          int64      `json:"removed_bytes"`
	RemovedAllocatedBytes int64      `json:"removed_allocated_bytes"`
	AvailableBefore       int64      `json:"available_before"`
	AvailableAfter        *int64     `json:"available_after"`
	AcknowledgedAt        *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy        uint64     `json:"acknowledged_by,omitempty"`
	PendingQuarantine     string     `json:"pending_quarantine,omitempty"`
	PendingName           string     `json:"pending_name,omitempty"`
	Problem               string     `json:"problem,omitempty"`
}
type Status struct {
	Configured             bool     `json:"configured"`
	AllowEnforce           bool     `json:"allow_enforce"`
	Sources                []Source `json:"sources"`
	Policies               []Policy `json:"policies"`
	Runs                   []Run    `json:"runs"`
	Revision               int64    `json:"revision"`
	Problem                string   `json:"problem,omitempty"`
	DatabaseCleanupEnabled bool     `json:"database_cleanup_enabled"`
}
type document struct {
	Version  int               `json:"version"`
	Revision int64             `json:"revision"`
	Policies map[string]Policy `json:"policies"`
	Runs     []Run             `json:"runs"`
}
type Options struct {
	Coordinator       *budget.Coordinator
	InstanceID        string
	Sources           []Source
	AllowEnforce      bool
	LegacyBlocked     func(context.Context) (bool, error)
	LegacySourceOwned func(context.Context, Source) bool
	VerifyArchive     func(context.Context, Source, Candidate) (bool, error)
	ArchiveReady      func(context.Context, Source) (bool, error)
	Publish           func()
	Now               func() time.Time
}
type Engine struct {
	opts       Options
	sources    map[string]Source
	beforeHold func()
	afterHold  func()
}

func New(opts Options) (*Engine, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Coordinator == nil || opts.InstanceID == "" || len(opts.Sources) > 64 {
		return nil, ErrInvalid
	}
	e := &Engine{opts: opts, sources: map[string]Source{}}
	for _, s := range opts.Sources {
		if s.ID == "" || s.Directory == "" || !filepath.IsAbs(s.Directory) || e.sources[s.ID].ID != "" {
			return nil, ErrInvalid
		}
		for _, name := range s.ActiveFiles {
			if name == "" || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
				return nil, ErrInvalid
			}
		}
		// Audit sources default to verified custody, including dev; only an explicit
		// cleanup mode can select safe expiry deletion instead.
		if s.Kind == "audit_logs" || s.Kind == "postgresql_logs" || s.Kind == "journal" {
			s.RequireArchive = true
		}
		s.Supported = s.Kind == "application_logs" || s.Kind == "audit_logs" || s.Kind == "postgresql_logs" || s.Kind == "system_logs" || s.Kind == "journal"
		if s.InstanceID != "" && s.InstanceID != opts.InstanceID && s.Scope != "shared" {
			s.Supported = false
			s.Problem = "source_owned_by_other_instance"
		}
		if !s.Supported {
			if s.Problem == "" {
				s.Problem = "protected_resource"
			}
		} else {
			if s.Format != "lumberjack" && s.Format != "numeric" && s.Format != "dated" && s.Format != "journal" && s.Format != "postgresql" {
				s.Supported = false
				s.Problem = "rotation_format_required"
			}
			if (s.Format != "journal" && len(s.ActiveFiles) == 0) || len(s.ActiveFiles) > 16 {
				s.Supported = false
				s.Problem = "active_files_required"
			}
			if s.Format == "postgresql" && !validPostgresSource(s) {
				s.Supported = false
				s.Problem = "postgresql_manifest_required"
			}
		}
		if s.Supported && !opts.Coordinator.ContainsPath(s.Directory) {
			s.Supported = false
			s.Problem = "source_filesystem_unregistered"
		}
		var st unix.Stat_t
		if unix.Lstat(s.Directory, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0002 != 0 {
			s.Supported = false
			s.Problem = "source_unavailable"
		} else {
			definition := sourceDefinition(s, st)
			h := sha256.Sum256([]byte(definition))
			s.Fingerprint = hex.EncodeToString(h[:])
		}
		e.sources[s.ID] = s
	}
	return e, nil
}
func decode(raw []byte) (document, error) {
	d := document{Version: 1, Policies: map[string]Policy{}, Runs: []Run{}}
	if len(raw) > 0 && (json.Unmarshal(raw, &d) != nil || d.Version != 1 || d.Policies == nil || len(d.Policies) > 64 || len(d.Runs) > 100) {
		return d, ErrInvalid
	}
	// 旧账本没有显式清理模式；升级只观察，不把旧布尔值迁成直接删除。
	for id, p := range d.Policies {
		if p.CleanupMode == "" {
			p.CleanupMode, p.Enabled = "disabled", false
			d.Policies[id] = p
		}
	}
	return d, nil
}
func (e *Engine) update(ctx context.Context, fn func(*document) error) error {
	_, err := e.opts.Coordinator.Document(ctx, "lifecycle", func(raw []byte) ([]byte, error) {
		d, err := decode(raw)
		if err != nil {
			return nil, err
		}
		if err = fn(&d); err != nil {
			return nil, err
		}
		d.Revision++
		return json.Marshal(d)
	})
	if err == nil && e.opts.Publish != nil {
		e.opts.Publish()
	}
	return err
}
func (e *Engine) load(ctx context.Context) (document, error) {
	raw, err := e.opts.Coordinator.Document(ctx, "lifecycle", nil)
	if err != nil {
		return document{}, err
	}
	return decode(raw)
}
func (e *Engine) Status(ctx context.Context) (Status, error) {
	d, err := e.load(ctx)
	out := Status{Configured: true, AllowEnforce: e.opts.AllowEnforce, Sources: []Source{}, Policies: []Policy{}, Runs: []Run{}}
	if err != nil {
		return out, err
	}
	out.Revision = d.Revision
	if e.opts.LegacyBlocked != nil {
		run, cancel := context.WithTimeout(ctx, 2*time.Second)
		blocked, err := e.opts.LegacyBlocked(run)
		cancel()
		if err != nil {
			out.Problem = "legacy_owner_unknown"
		} else if blocked {
			for _, source := range e.sources {
				if source.Supported && (e.opts.LegacySourceOwned == nil || !e.opts.LegacySourceOwned(ctx, source)) {
					out.Problem = "legacy_owner_active"
					break
				}
			}
		}
	}
	for _, s := range e.sources {
		out.Sources = append(out.Sources, s)
		if p, ok := d.Policies[s.ID]; ok {
			out.Policies = append(out.Policies, p)
		}
	}
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].ID < out.Sources[j].ID })
	for _, r := range d.Runs {
		if _, ok := e.sources[r.SourceID]; ok {
			if r.State == "executing" && e.opts.Now().Sub(r.StartedAt) > 2*time.Minute {
				r.State = "unconfirmed"
				r.Problem = "interrupted_execution"
			}
			out.Runs = append(out.Runs, r)
		}
	}
	if len(out.Runs) > 50 {
		out.Runs = out.Runs[len(out.Runs)-50:]
	}
	return out, nil
}
func (e *Engine) Save(ctx context.Context, actor uint64, id string, expected int64, p Policy) (Policy, error) {
	s, ok := e.sources[id]
	if !ok || actor == 0 || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.MaxBytes < 1 || p.MaxBytes > 1<<50 {
		return Policy{}, ErrInvalid
	}
	// 兼容旧客户端时仍遵守旧来源的归档要求；只有显式模式能选择直接删除。
	if p.CleanupMode == "" {
		p.CleanupMode = "expire_delete"
		if s.RequireArchive || p.RequireArchive {
			p.CleanupMode = "archive_then_delete"
		}
	}
	if p.CleanupMode != "disabled" && p.CleanupMode != "expire_delete" && p.CleanupMode != "archive_then_delete" {
		return Policy{}, ErrInvalid
	}
	if p.Enabled && p.CleanupMode == "disabled" {
		return Policy{}, ErrInvalid
	}
	p.RequireArchive = p.CleanupMode == "archive_then_delete"
	if p.Enabled && p.RequireArchive && e.opts.ArchiveReady != nil {
		ready, err := e.opts.ArchiveReady(ctx, s)
		if err != nil || !ready {
			return Policy{}, ErrBlocked
		}
	}
	if p.Enabled && (!e.opts.AllowEnforce || !s.Supported) {
		return Policy{}, ErrBlocked
	}
	if p.Enabled {
		blocked, err := e.legacyBlocked(ctx, s)
		if err != nil || blocked {
			return Policy{}, ErrBlocked
		}
	}
	p.SourceID = id
	p.Fingerprint = s.Fingerprint
	p.OperatorID = actor
	p.OwnerInstance = e.opts.InstanceID
	p.UpdatedAt = e.opts.Now().UTC()
	err := e.update(ctx, func(d *document) error {
		current := d.Policies[id]
		if !s.Supported && current.Revision == 0 {
			return ErrInvalid
		}
		if current.Revision != expected {
			return ErrConflict
		}
		if current.Enabled && current.OwnerInstance != e.opts.InstanceID {
			return ErrBlocked
		}
		for _, r := range d.Runs {
			if p.Enabled && r.SourceID == id && (r.State == "executing" || (r.State == "unconfirmed" && r.AcknowledgedAt == nil)) {
				return ErrBlocked
			}
		}
		p.Revision = expected + 1
		d.Policies[id] = p
		return nil
	})
	return p, err
}
func (e *Engine) Preview(ctx context.Context, id string) (Preview, error) {
	d, err := e.load(ctx)
	if err != nil {
		return Preview{}, err
	}
	p, ok := d.Policies[id]
	if !ok {
		return Preview{}, ErrInvalid
	}
	return e.inspect(ctx, id, p)
}
func (e *Engine) inspect(ctx context.Context, id string, p Policy) (Preview, error) {
	s, ok := e.sources[id]
	out := Preview{SourceID: id, PolicyRevision: p.Revision, Complete: true, Candidates: []Candidate{}, CandidateNames: []string{}, Protected: map[string]int{}}
	if !ok || !s.Supported || p.Fingerprint != s.Fingerprint {
		return out, ErrBlocked
	}
	fd, err := unix.Open(s.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return out, ErrBlocked
	}
	f := os.NewFile(uintptr(fd), "registered-log-source")
	defer f.Close()
	var root unix.Stat_t
	if unix.Fstat(fd, &root) != nil || root.Mode&0002 != 0 {
		return out, ErrBlocked
	}
	currentDefinition := sourceDefinition(s, root)
	h := sha256.Sum256([]byte(currentDefinition))
	if hex.EncodeToString(h[:]) != s.Fingerprint {
		return out, ErrBlocked
	}
	activity, err := postgresActivity(s)
	if err != nil {
		out.Complete, out.Problem = false, "postgresql_activity_unavailable"
		return out, nil
	}
	names, err := f.Readdirnames(1001)
	if err != nil && err != io.EOF {
		return out, ErrBlocked
	}
	if len(names) > 1000 {
		out.Complete = false
		out.Problem = "scan_incomplete"
		names = names[:1000]
	}
	items := []Candidate{}
	latest := map[string]int64{}
	for _, name := range names {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		var st unix.Stat_t
		if unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
			out.Protected["unreadable"]++
			out.Complete = false
			continue
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Dev != root.Dev || st.Mode&0002 != 0 || st.Uid != s.OwnerUID || (s.Format == "postgresql" && st.Mode&0022 != 0) {
			out.Protected["unsafe_file"]++
			continue
		}
		out.ObservedBytes += st.Blocks * 512
		if activity.active[name] || !sealedName(s, name, e.opts.Now()) {
			out.Protected["active_or_unknown"]++
			continue
		}
		c := Candidate{Name: name, Device: uint64(st.Dev), Inode: st.Ino, Bytes: st.Size, Allocated: st.Blocks * 512, ModifiedNS: st.Mtim.Nano(), ChangedNS: st.Ctim.Nano()}
		if c.ModifiedNS > latest[sealedGroup(s, name)] {
			latest[sealedGroup(s, name)] = c.ModifiedNS
		}
		items = append(items, c)
	}
	cutoff := e.opts.Now().Add(-time.Duration(p.RetentionDays) * 24 * time.Hour).UnixNano()
	stable := e.opts.Now().Add(-5 * time.Minute).UnixNano()
	sort.Slice(items, func(i, j int) bool { return items[i].ModifiedNS < items[j].ModifiedNS })
	for _, c := range items {
		if c.ModifiedNS >= latest[sealedGroup(s, c.Name)] {
			out.Protected["latest_rotation"]++
			continue
		}
		if c.ModifiedNS >= cutoff || c.ModifiedNS >= stable {
			out.Protected["within_retention"]++
			continue
		}
		if len(out.Candidates) >= 32 {
			out.Protected["batch_limit"]++
			continue
		}
		out.Candidates = append(out.Candidates, c)
		out.CandidateNames = append(out.CandidateNames, c.Name)
		out.CandidateBytes += c.Allocated
	}
	if !activity.unchanged(s) {
		out.Complete, out.Problem = false, "postgresql_activity_changed"
		out.Candidates, out.CandidateNames, out.CandidateBytes = []Candidate{}, []string{}, 0
	}
	out.OverBudget = out.ObservedBytes > p.MaxBytes
	return out, nil
}
func sealedName(s Source, name string, now time.Time) bool {
	if filepath.Base(name) != name {
		return false
	}
	if s.Format == "journal" {
		return regexp.MustCompile(`^(system|user-[0-9]+)@[a-f0-9-]+\.journal$`).MatchString(name)
	}
	if s.Format == "postgresql" {
		return postgresLogGroup(s, name) != ""
	}
	for _, active := range s.ActiveFiles {
		if name == active {
			continue
		}
		switch s.Format {
		case "numeric":
			if regexp.MustCompile(`^` + regexp.QuoteMeta(active) + `\.[1-9][0-9]*(\.gz)?$`).MatchString(name) {
				return true
			}
		case "lumberjack":
			ext := filepath.Ext(active)
			prefix := strings.TrimSuffix(active, ext) + "-"
			raw := strings.TrimSuffix(name, ".gz")
			if strings.HasPrefix(raw, prefix) && strings.HasSuffix(raw, ext) {
				_, err := time.Parse("2006-01-02T15-04-05.000", strings.TrimSuffix(strings.TrimPrefix(raw, prefix), ext))
				if err == nil {
					return true
				}
			}
		case "dated":
			if strings.HasPrefix(name, active) && len(name) >= len(active)+10 {
				day := name[len(active) : len(active)+10]
				_, err := time.Parse("2006-01-02", day)
				if err == nil && day < now.Format("2006-01-02") && regexp.MustCompile(`^`+regexp.QuoteMeta(active)+`[0-9]{4}-[0-9]{2}-[0-9]{2}(-[0-9T:.\-]+)?\.log(\.gz)?$`).MatchString(name) {
					return true
				}
			}
		}
	}
	return false
}
func (e *Engine) Run(ctx context.Context, id string) (Run, error) {
	key := fmt.Sprintf("scheduled:%s:%d", id, e.opts.Now().Unix()/900)
	return e.RunWithKey(ctx, id, key)
}
func (e *Engine) RunWithKey(ctx context.Context, id, key string) (Run, error) {
	return e.runWithKey(ctx, id, key, 0)
}
func (e *Engine) RunWithRevision(ctx context.Context, id, key string, revision int64) (Run, error) {
	if revision < 1 {
		return Run{}, ErrInvalid
	}
	return e.runWithKey(ctx, id, key, revision)
}
func (e *Engine) runWithKey(ctx context.Context, id, key string, expected int64) (Run, error) {
	if len(key) < 1 || len(key) > 128 || !regexp.MustCompile(`^[A-Za-z0-9:._-]+$`).MatchString(key) {
		return Run{}, ErrInvalid
	}
	d, err := e.load(ctx)
	if err != nil {
		return Run{}, err
	}
	for _, previous := range d.Runs {
		if previous.SourceID == id && previous.RequestKey == key {
			return previous, nil
		}
	}
	p, ok := d.Policies[id]
	if expected > 0 && p.Revision != expected {
		return Run{}, ErrConflict
	}
	if !ok || !p.Enabled || !e.opts.AllowEnforce || p.OwnerInstance != e.opts.InstanceID {
		return Run{}, ErrBlocked
	}
	if blocked, err := e.legacyBlocked(ctx, e.sources[id]); err != nil || blocked {
		return Run{}, ErrBlocked
	}
	preview, err := e.inspect(ctx, id, p)
	if err != nil {
		return Run{}, err
	}
	if !preview.Complete {
		return Run{}, ErrBlocked
	}
	source := e.sources[id]
	space, err := e.opts.Coordinator.Status(ctx)
	if err != nil {
		return Run{}, err
	}
	run := Run{ID: uuid.NewString(), RequestKey: key, SourceID: id, PolicyRevision: p.Revision, InstanceID: e.opts.InstanceID, State: "executing", StartedAt: e.opts.Now().UTC(), AvailableBefore: space.AvailableBytes}
	err = e.update(ctx, func(d *document) error {
		current := d.Policies[id]
		if !current.Enabled || current.Revision != p.Revision {
			return ErrConflict
		}
		for _, r := range d.Runs {
			if r.SourceID == id && r.RequestKey == key {
				return ErrConflict
			}
			if r.SourceID == id && (r.State == "executing" || (r.State == "unconfirmed" && r.AcknowledgedAt == nil)) {
				return ErrBlocked
			}
		}
		// 不自动移除未确认意图；满额时拒绝新任务。
		for len(d.Runs) >= 100 {
			removed := false
			for i, r := range d.Runs {
				if r.State == "completed" || r.State == "partial" || r.State == "blocked" || r.AcknowledgedAt != nil {
					d.Runs = append(d.Runs[:i], d.Runs[i+1:]...)
					removed = true
					break
				}
			}
			if !removed {
				return ErrBlocked
			}
		}
		d.Runs = append(d.Runs, run)
		return nil
	})
	if err != nil {
		return Run{}, err
	}
	persist := func() error {
		return e.update(context.WithoutCancel(ctx), func(d *document) error {
			if run.PendingName != "" {
				pNow := d.Policies[id]
				if !pNow.Enabled || pNow.Revision != p.Revision {
					return ErrConflict
				}
			}
			for i, r := range d.Runs {
				if r.ID == run.ID {
					d.Runs[i] = run
					return nil
				}
			}
			return ErrInvalid
		})
	}
	for _, candidate := range preview.Candidates {
		if ctx.Err() != nil {
			run.Problem = "cancelled"
			break
		}
		current, err := e.load(ctx)
		if err != nil {
			run.Problem = "state_unavailable"
			break
		}
		if latest := current.Policies[id]; !latest.Enabled || latest.Revision != p.Revision {
			run.Problem = "policy_changed"
			break
		}
		if p.CleanupMode == "archive_then_delete" || p.RequireArchive {
			if e.opts.VerifyArchive == nil {
				run.Problem = "archive_verification_required"
				continue
			}
			verified, err := e.opts.VerifyArchive(ctx, source, candidate)
			if err != nil || !verified {
				run.Problem = "archive_verification_required"
				continue
			}
		}
		run.PendingName = candidate.Name
		run.PendingQuarantine = uuid.NewString()
		if err := persist(); err != nil {
			if errors.Is(err, ErrConflict) {
				run.PendingName = ""
				run.PendingQuarantine = ""
				run.Problem = "policy_changed"
				break
			}
			return run, ErrBlocked
		}
		// 主机账本锁覆盖最终策略检查和 unlink；暂停成功后不会继续删下一文件。
		err = e.update(ctx, func(d *document) error {
			latest := d.Policies[id]
			if !latest.Enabled || latest.Revision != p.Revision {
				run.PendingName = ""
				run.PendingQuarantine = ""
				run.Problem = "policy_changed"
			} else {
				removed, problem, held := e.remove(ctx, source, candidate, run.PendingQuarantine)
				if removed {
					run.DeletedFiles++
					run.RemovedBytes += candidate.Bytes
					run.RemovedAllocatedBytes += candidate.Allocated
				} else {
					run.Problem = problem
				}
				if held {
					run.State = "unconfirmed"
				} else {
					run.PendingName = ""
					run.PendingQuarantine = ""
				}
			}
			for i, r := range d.Runs {
				if r.ID == run.ID {
					d.Runs[i] = run
					return nil
				}
			}
			return ErrInvalid
		})
		if err != nil {
			return run, ErrBlocked
		}
		if run.State == "unconfirmed" {
			return run, nil
		}
		if run.Problem == "policy_changed" {
			break
		}
	}
	if run.Problem == "" && preview.ObservedBytes-run.RemovedAllocatedBytes > p.MaxBytes {
		run.Problem = "protected_budget_exceeded"
	}
	if run.Problem != "" {
		run.State = "partial"
		if run.DeletedFiles == 0 {
			run.State = "blocked"
		}
	} else {
		run.State = "completed"
	}
	now := e.opts.Now().UTC()
	run.FinishedAt = &now
	if space, err := e.opts.Coordinator.Status(context.WithoutCancel(ctx)); err == nil {
		run.AvailableAfter = &space.AvailableBytes
	}
	if err = persist(); err != nil {
		return run, err
	}
	return run, nil
}
func (e *Engine) remove(ctx context.Context, s Source, c Candidate, privateName string) (bool, string, bool) {
	if ctx.Err() != nil {
		return false, "cancelled", false
	}
	if !sealedName(s, c.Name, e.opts.Now()) {
		return false, "unsafe_file", false
	}
	activity, err := postgresActivity(s)
	if err != nil || activity.active[c.Name] {
		return false, "postgresql_activity_unavailable", false
	}
	fd, err := unix.Open(s.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, "source_unavailable", false
	}
	defer unix.Close(fd)
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return false, "source_busy", false
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	var root, st unix.Stat_t
	if unix.Fstat(fd, &root) != nil || uint64(root.Dev) != c.Device {
		return false, "source_changed", false
	}
	definition := sourceDefinition(s, root)
	h := sha256.Sum256([]byte(definition))
	if hex.EncodeToString(h[:]) != s.Fingerprint {
		return false, "source_changed", false
	}
	if unix.Fstatat(fd, c.Name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
		return false, "candidate_missing", false
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != s.OwnerUID || st.Ino != c.Inode || uint64(st.Dev) != c.Device || st.Size != c.Bytes || st.Blocks*512 != c.Allocated || st.Mtim.Nano() != c.ModifiedNS || st.Ctim.Nano() != c.ChangedNS {
		return false, "candidate_changed", false
	}
	// 重新扫描同一日志流，防止并发轮转或其他清理使候选成为最后一份。
	dup, err := unix.Dup(fd)
	if err != nil {
		return false, "source_unavailable", false
	}
	listing := os.NewFile(uintptr(dup), "latest-rotation-check")
	names, err := listing.Readdirnames(1001)
	listing.Close()
	if (err != nil && err != io.EOF) || len(names) > 1000 {
		return false, "scan_incomplete", false
	}
	newer := false
	for _, name := range names {
		if name == c.Name || activity.active[name] || !sealedName(s, name, e.opts.Now()) || sealedGroup(s, name) != sealedGroup(s, c.Name) {
			continue
		}
		var other unix.Stat_t
		if unix.Fstatat(fd, name, &other, unix.AT_SYMLINK_NOFOLLOW) == nil && other.Mode&unix.S_IFMT == unix.S_IFREG && other.Nlink == 1 && other.Uid == s.OwnerUID && other.Mtim.Nano() > c.ModifiedNS {
			newer = true
			break
		}
	}
	if !newer {
		return false, "latest_rotation", false
	}
	// rename 原子捕获实际路径对象，再在私有目录核对，避免 logrotate 在检查后替换文件。
	if e.beforeHold != nil {
		e.beforeHold()
	}
	if !activity.unchanged(s) {
		return false, "postgresql_activity_changed", false
	}
	removed, held, problem := false, false, "quarantine_unavailable"
	err = e.opts.Coordinator.WithQuarantine(ctx, func(qfd int) error {
		placeholder, err := unix.Openat(qfd, privateName, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			return err
		}
		unix.Close(placeholder)
		if err = unix.Renameat(fd, c.Name, qfd, privateName); err != nil {
			unix.Unlinkat(qfd, privateName, 0)
			problem = "candidate_changed"
			return err
		}
		held = true
		if e.afterHold != nil {
			e.afterHold()
		}
		if unix.Fsync(qfd) != nil || unix.Fsync(fd) != nil {
			problem = "quarantine_requires_review"
			return ErrBlocked
		}
		var captured unix.Stat_t
		same := unix.Fstatat(qfd, privateName, &captured, unix.AT_SYMLINK_NOFOLLOW) == nil && captured.Mode&unix.S_IFMT == unix.S_IFREG && captured.Nlink == 1 && captured.Uid == s.OwnerUID && captured.Ino == c.Inode && uint64(captured.Dev) == c.Device && captured.Size == c.Bytes && captured.Mtim.Nano() == c.ModifiedNS
		if same && s.Format == "postgresql" {
			if !activity.unchanged(s) {
				same, problem = false, "postgresql_activity_changed"
			} else if closed := postgresFileClosed(ctx, s, filepath.Join(e.opts.Coordinator.ChangeDirectory(), "quarantine", privateName)); !closed {
				same, problem = false, "postgresql_file_open_or_unknown"
			}
			if same && (!activity.unchanged(s) || unix.Fstatat(qfd, privateName, &captured, unix.AT_SYMLINK_NOFOLLOW) != nil || captured.Ino != c.Inode || captured.Size != c.Bytes || captured.Mtim.Nano() != c.ModifiedNS || captured.Nlink != 1) {
				same, problem = false, "candidate_changed"
			}
		}
		if !same {
			if problem == "quarantine_unavailable" {
				problem = "candidate_changed"
			}
			// Linkat 不覆盖新出现的文件；若原位置已占用就保持隔离，等待人工核对。
			if unix.Linkat(qfd, privateName, fd, c.Name, 0) == nil {
				if unix.Unlinkat(qfd, privateName, 0) == nil && unix.Fsync(qfd) == nil && unix.Fsync(fd) == nil {
					held = false
					return nil
				}
			}
			problem = "quarantine_requires_review"
			return ErrBlocked
		}
		if unix.Unlinkat(qfd, privateName, 0) != nil {
			problem = "quarantine_requires_review"
			return ErrBlocked
		}
		held = false
		removed = true
		problem = ""
		if unix.Fsync(qfd) != nil {
			held = true
			removed = false
			problem = "quarantine_requires_review"
			return ErrBlocked
		}
		return nil
	})
	if err != nil && problem == "" {
		problem = "quarantine_requires_review"
	}
	return removed, problem, held
}

func sourceDefinition(s Source, st unix.Stat_t) string {
	definition := fmt.Sprintf("%s:%s:%d:%d:%d:%v:%s:%s:%s:%s:%d:%t", s.ID, s.Directory, st.Dev, st.Ino, st.Uid, s.ActiveFiles, s.Format, s.Kind, s.Scope, s.InstanceID, s.OwnerUID, s.RequireArchive)
	if s.Format == "postgresql" {
		definition += ":" + s.CurrentLogfiles
	}
	return definition
}
func sealedGroup(s Source, name string) string {
	if s.Format == "postgresql" {
		return postgresLogGroup(s, name)
	}
	if s.Format == "journal" {
		return strings.SplitN(name, "@", 2)[0]
	}
	for _, active := range s.ActiveFiles {
		single := s
		single.ActiveFiles = []string{active}
		if sealedName(single, name, time.Now().AddDate(100, 0, 0)) {
			return active
		}
	}
	return name
}

// AcknowledgeInterrupted 只解除暂停来源的陈旧执行锁；未知删除结果继续保留为未知。
func (e *Engine) AcknowledgeInterrupted(ctx context.Context, actor uint64, id string) (Run, error) {
	var out Run
	if actor == 0 {
		return out, ErrInvalid
	}
	err := e.update(ctx, func(d *document) error {
		for i, r := range d.Runs {
			if r.ID != id {
				continue
			}
			if r.AcknowledgedAt != nil {
				out = r
				return nil
			}
			p := d.Policies[r.SourceID]
			if p.Enabled || r.InstanceID != e.opts.InstanceID || (r.State != "executing" && r.State != "unconfirmed") || e.opts.Now().Sub(r.StartedAt) < 2*time.Minute {
				return ErrBlocked
			}
			if r.PendingQuarantine != "" {
				err := e.opts.Coordinator.WithQuarantine(ctx, func(fd int) error {
					var st unix.Stat_t
					if err := unix.Fstatat(fd, r.PendingQuarantine, &st, unix.AT_SYMLINK_NOFOLLOW); err == unix.ENOENT {
						return nil
					}
					return ErrBlocked
				})
				if err != nil {
					return ErrBlocked
				}
			}
			now := e.opts.Now().UTC()
			r.State = "unconfirmed"
			r.Problem = "interrupted_execution_acknowledged"
			r.AcknowledgedAt = &now
			r.AcknowledgedBy = actor
			d.Runs[i] = r
			out = r
			return nil
		}
		return ErrInvalid
	})
	return out, err
}
