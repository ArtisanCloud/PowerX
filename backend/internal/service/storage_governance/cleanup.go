package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"time"
)

var ErrCleanupBlocked = errors.New("storage cleanup blocked")
var ErrCleanupConflict = errors.New("storage cleanup preview changed or expired")
var ErrCleanupNotFound = errors.New("storage cleanup record not found")

type cleanupStore interface {
	CreateRun(context.Context, *m.CleanupRun) error
	GetRun(context.Context, string, string) (*m.CleanupRun, error)
	UpdateRun(context.Context, *m.CleanupRun, int64) error
	ListRuns(context.Context, string) ([]m.CleanupRun, error)
}
type CleanupItem struct {
	Name    string `json:"name"`
	Bytes   int64  `json:"bytes"`
	Outcome string `json:"outcome"`
}
type CleanupRecord struct {
	ID               string         `json:"id"`
	Resource         ResourceRef    `json:"resource"`
	State            string         `json:"state"`
	CreatedAt        time.Time      `json:"created_at"`
	ExpiresAt        time.Time      `json:"expires_at"`
	TargetDays       int            `json:"target_days"`
	ActorID          uint64         `json:"actor_id"`
	CanExecute       bool           `json:"can_execute"`
	BlockedReason    string         `json:"blocked_reason,omitempty"`
	Items            []CleanupItem  `json:"items"`
	Skipped          map[string]int `json:"skipped"`
	DeletedFiles     int            `json:"deleted_files"`
	RemovedFileBytes int64          `json:"removed_file_bytes"`
	ReclaimedBytes   *int64         `json:"reclaimed_bytes"`
}
type cleanupDocument struct {
	Record         CleanupRecord `json:"record"`
	Identity       Identity      `json:"identity"`
	Scan           LogScan       `json:"scan"`
	PolicyRevision int64         `json:"policy_revision"`
}

func rootActor(ctx context.Context) uint64 {
	c := reqctx.GetClaims(ctx)
	if c == nil || !reqctx.IsRoot(ctx) || c.UserID == 0 || c.Issuer == "powerx-sts" {
		return 0
	}
	for _, p := range c.Platforms {
		if p == "api_key" || p == "service_actor" {
			return 0
		}
	}
	return c.UserID
}
func (s *Service) cleanupReady(ctx context.Context) error {
	if rootActor(ctx) == 0 {
		return ErrRootRequired
	}
	if !s.stateReady() || s.runs == nil || s.opts.Identity.HostID == "" || s.opts.Identity.FilesystemID == "" {
		return ErrStateUnavailable
	}
	return nil
}
func (s *Service) logResource(id string) (Resource, error) {
	for _, r := range s.opts.Resources {
		if r.ID == id && r.Kind == "application_logs" && r.InstanceID == s.opts.Identity.InstanceID && r.Scope == s.opts.Identity.Environment && s.opts.LogLifecycle != nil && s.opts.LogLifecycle.Control(id) != nil {
			return r, nil
		}
	}
	return Resource{}, ErrCleanupBlocked
}
func (s *Service) PreviewCleanup(ctx context.Context, id string, days int) (CleanupRecord, error) {
	if err := s.cleanupReady(ctx); err != nil {
		return CleanupRecord{}, err
	}
	if days < 1 || days > 3650 {
		return CleanupRecord{}, ErrPolicyInvalid
	}
	r, err := s.logResource(id)
	if err != nil {
		return CleanupRecord{}, err
	}
	p, err := s.Policy(ctx)
	if err != nil {
		return CleanupRecord{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	scan, err := s.opts.LogLifecycle.Inspect(bounded, id, s.opts.Now().Add(-time.Duration(days)*24*time.Hour))
	if err != nil {
		return CleanupRecord{}, ErrStateUnavailable
	}
	now := s.opts.Now().UTC()
	out := CleanupRecord{ID: uuid.NewString(), Resource: refFor(r), State: "preview", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute), TargetDays: days, ActorID: rootActor(ctx), CanExecute: true, Skipped: scan.Skipped, Items: []CleanupItem{}}
	for _, c := range scan.Candidates {
		out.Items = append(out.Items, CleanupItem{Name: c.Name, Bytes: c.Size, Outcome: "candidate"})
	}
	out.BlockedReason = s.opts.LogLifecycle.Control(id).BlockedReason
	if !scan.Complete {
		out.BlockedReason = "scan_incomplete"
	}
	if len(scan.Candidates) == 0 {
		out.BlockedReason = "no_candidates"
	}
	if r.Scope == "prod" || r.Scope == "staging" {
		out.BlockedReason = "archive_verification_required"
	}
	for _, rp := range p.Resources {
		if rp.ResourceID == id && rp.RequireVerifiedArchive {
			out.BlockedReason = "archive_verification_required"
		}
	}
	out.CanExecute = out.BlockedReason == ""
	doc := cleanupDocument{Record: out, Identity: s.opts.Identity, Scan: scan, PolicyRevision: p.Revision}
	raw, _ := json.Marshal(doc)
	if len(raw) > 65536 {
		return CleanupRecord{}, ErrStateUnavailable
	}
	row := &m.CleanupRun{TenantID: 0, InstanceID: s.opts.Identity.InstanceID, RunID: out.ID, Version: 1, State: "preview", ActorID: out.ActorID, ExpiresAt: out.ExpiresAt, Document: raw}
	if err = s.runs.CreateRun(ctx, row); err != nil {
		return CleanupRecord{}, ErrStateUnavailable
	}
	return out, nil
}
func decodeCleanup(row *m.CleanupRun) (cleanupDocument, error) {
	var d cleanupDocument
	if json.Unmarshal(row.Document, &d) != nil || d.Record.ID != row.RunID || d.Record.State != row.State || len(d.Scan.Candidates) > 32 || len(d.Record.Items) != len(d.Scan.Candidates) {
		return d, ErrStateUnavailable
	}
	return d, nil
}
func publicCleanup(d cleanupDocument) CleanupRecord {
	out := d.Record
	if out.State == "executing" {
		out.State = "unconfirmed"
		out.CanExecute = false
	}
	return out
}
func (s *Service) CleanupRun(ctx context.Context, id string) (CleanupRecord, error) {
	if err := s.cleanupReady(ctx); err != nil {
		return CleanupRecord{}, err
	}
	if _, err := uuid.Parse(id); err != nil {
		return CleanupRecord{}, ErrCleanupNotFound
	}
	row, err := s.runs.GetRun(ctx, s.opts.Identity.InstanceID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CleanupRecord{}, ErrCleanupNotFound
	}
	if err != nil {
		return CleanupRecord{}, ErrStateUnavailable
	}
	d, err := decodeCleanup(row)
	if err != nil || d.Identity != s.opts.Identity {
		return CleanupRecord{}, ErrStateUnavailable
	}
	return publicCleanup(d), nil
}
func (s *Service) CleanupRuns(ctx context.Context) ([]CleanupRecord, error) {
	if err := s.cleanupReady(ctx); err != nil {
		return nil, err
	}
	rows, err := s.runs.ListRuns(ctx, s.opts.Identity.InstanceID)
	if err != nil {
		return nil, ErrStateUnavailable
	}
	out := []CleanupRecord{}
	for i := range rows {
		d, e := decodeCleanup(&rows[i])
		if e != nil {
			return nil, e
		}
		if d.Identity == s.opts.Identity {
			out = append(out, publicCleanup(d))
		}
	}
	return out, nil
}
func (s *Service) ExecuteCleanup(ctx context.Context, id string) (CleanupRecord, error) {
	if err := s.cleanupReady(ctx); err != nil {
		return CleanupRecord{}, err
	}
	s.policyMu.Lock()
	defer s.policyMu.Unlock()
	if _, err := uuid.Parse(id); err != nil {
		return CleanupRecord{}, ErrCleanupNotFound
	}
	row, err := s.runs.GetRun(ctx, s.opts.Identity.InstanceID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CleanupRecord{}, ErrCleanupNotFound
	}
	if err != nil {
		return CleanupRecord{}, ErrStateUnavailable
	}
	d, err := decodeCleanup(row)
	if err != nil {
		return CleanupRecord{}, err
	}
	if row.State != "preview" {
		return publicCleanup(d), nil
	} // 幂等重放只回读，不再次删除。
	if d.Identity != s.opts.Identity || row.ActorID != rootActor(ctx) || !s.opts.Now().Before(d.Record.ExpiresAt) {
		return CleanupRecord{}, ErrCleanupConflict
	}
	if !d.Record.CanExecute {
		return CleanupRecord{}, ErrCleanupBlocked
	}
	if _, err = s.logResource(d.Record.Resource.ID); err != nil {
		return CleanupRecord{}, err
	}
	p, err := s.Policy(ctx)
	if err != nil {
		return CleanupRecord{}, err
	}
	if p.Revision != d.PolicyRevision {
		return CleanupRecord{}, ErrCleanupConflict
	}
	if c := s.opts.LogLifecycle.Control(d.Record.Resource.ID); c == nil || c.BlockedReason != "" {
		return CleanupRecord{}, ErrCleanupBlocked
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	persist := func(writeCtx context.Context) error {
		raw, e := json.Marshal(d)
		if e != nil {
			return ErrStateUnavailable
		}
		expected := row.Version
		row.Version++
		row.State = d.Record.State
		row.Document = datatypes.JSON(raw)
		if e = s.runs.UpdateRun(writeCtx, row, expected); e != nil {
			if errors.Is(e, repo.ErrRevisionConflict) {
				return ErrCleanupConflict
			}
			return ErrStateUnavailable
		}
		return nil
	}
	err = s.opts.LogLifecycle.WithLock(bounded, d.Record.Resource.ID, func() error {
		fresh, e := s.opts.LogLifecycle.Inspect(bounded, d.Record.Resource.ID, d.Record.CreatedAt.Add(-time.Duration(d.Record.TargetDays)*24*time.Hour))
		if e != nil {
			return ErrStateUnavailable
		}
		if !fresh.Complete || fresh.Fingerprint != d.Scan.Fingerprint {
			return ErrCleanupConflict
		}
		d.Record.State = "executing"
		d.Record.CanExecute = false
		if e = persist(bounded); e != nil {
			return e
		}
		eligible := map[string]LogCandidate{}
		for _, c := range fresh.Candidates {
			eligible[c.Name] = c
		}
		partial := false
		for i, c := range d.Scan.Candidates {
			if bounded.Err() != nil {
				partial = true
				break
			}
			if control := s.opts.LogLifecycle.Control(d.Record.Resource.ID); control == nil || control.BlockedReason != "" {
				partial = true
				d.Record.BlockedReason = "legacy_retention_enabled"
				break
			}
			if _, ok := eligible[c.Name]; !ok {
				d.Record.Items[i].Outcome = "protected_after_preview"
				partial = true
				continue
			}
			d.Record.Items[i].Outcome = "deleting"
			if e = persist(bounded); e != nil {
				return e
			}
			outcome, removeErr := s.opts.LogLifecycle.Remove(bounded, d.Record.Resource.ID, c)
			d.Record.Items[i].Outcome = outcome
			if outcome == "deleted" {
				d.Record.DeletedFiles++
				d.Record.RemovedFileBytes += c.Size
			} else {
				partial = true
			}
			if e = persist(bounded); e != nil {
				return e
			}
			if removeErr != nil {
				partial = true
				break
			}
		}
		d.Record.State = "completed"
		if partial {
			d.Record.State = "partial"
		}
		for i := range d.Record.Items {
			if d.Record.Items[i].Outcome == "candidate" {
				d.Record.Items[i].Outcome = "not_attempted"
			}
		}
		finalCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer stop()
		return persist(finalCtx)
	})
	s.mu.Lock()
	s.cached = nil
	s.mu.Unlock()
	if err != nil {
		return CleanupRecord{}, err
	}
	return publicCleanup(d), nil
}
