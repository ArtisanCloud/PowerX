package storagerecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxRetentionImports = 1024

// RetentionImport 是独立凭据的操作审计，不是备份保护命令。
type RetentionImport struct {
	RequestID        string     `json:"request_id"`
	Scope            string     `json:"scope"`
	ActorID          uint64     `json:"actor_id"`
	InputDigest      string     `json:"input_digest"`
	ExpectedSnapshot string     `json:"expected_snapshot"`
	Generation       string     `json:"generation"`
	Reason           string     `json:"reason"`
	CreatedAt        time.Time  `json:"created_at"`
	State            string     `json:"state"`
	Problem          string     `json:"problem,omitempty"`
	SourceDigest     string     `json:"source_digest,omitempty"`
	RecordDigest     string     `json:"record_digest,omitempty"`
	Total            int        `json:"total"`
	Covered          int        `json:"covered"`
	Imported         int        `json:"imported"`
	Unknown          int        `json:"unknown"`
	Complete         bool       `json:"complete"`
	CompletedAt      *time.Time `json:"completed_at"`
}
type RetentionInventory struct {
	Epoch        Epoch
	Records      map[string]RetentionRecord
	RecordDigest string
	ImportCount  int
}
type RetentionImportPlan struct {
	Records                 []RetentionRecord
	Problem                 string
	SourceDigest            string
	Total, Covered, Unknown int
}

func RetentionGeneration(scope string, e Epoch) string {
	s := sha256.Sum256([]byte(scope + "/" + e.Protocol + "/" + e.Installation + "/" + e.AnchorID + "/" + e.Epoch))
	return hex.EncodeToString(s[:])
}
func RecordsDigest(records map[string]RetentionRecord) string {
	raw, _ := json.Marshal(records)
	s := sha256.Sum256(raw)
	return hex.EncodeToString(s[:])
}
func validRetentionImport(r RetentionImport) bool {
	if !validUUID(r.RequestID) || r.RequestID == "00000000-0000-0000-0000-000000000000" || r.ActorID == 0 || !digestPattern.MatchString(r.Scope) || !digestPattern.MatchString(r.InputDigest) || !digestPattern.MatchString(r.ExpectedSnapshot) || !digestPattern.MatchString(r.Generation) || r.CreatedAt.IsZero() || strings.TrimSpace(r.Reason) != r.Reason || utf8.RuneCountInString(r.Reason) < 3 || utf8.RuneCountInString(r.Reason) > 256 || r.Total < 0 || r.Covered < 0 || r.Covered > r.Total || r.Unknown < 0 || r.Unknown > r.Covered || r.Imported < 0 || r.Imported > r.Covered {
		return false
	}
	if r.State == "pending" {
		return r.CompletedAt == nil && !r.Complete && r.Problem == "" && r.SourceDigest == "" && r.RecordDigest == "" && r.Total == 0 && r.Covered == 0 && r.Imported == 0 && r.Unknown == 0
	}
	if r.CompletedAt == nil || r.CompletedAt.Before(r.CreatedAt) {
		return false
	}
	if r.State == "rejected" {
		return !r.Complete && problemPattern.MatchString(r.Problem) && (r.SourceDigest == "" || digestPattern.MatchString(r.SourceDigest))
	}
	return r.State == "completed" && r.Complete && r.Problem == "" && r.Total == r.Covered && digestPattern.MatchString(r.SourceDigest) && digestPattern.MatchString(r.RecordDigest)
}
func (s *Store) RetentionInventory(ctx context.Context) (RetentionInventory, error) {
	var out RetentionInventory
	e := s.retention(ctx, false, func(fd int, d *retentionLedger) error {
		epoch, e := s.readEpoch(fd)
		if e != nil {
			return e
		}
		out = RetentionInventory{epoch, d.Records, RecordsDigest(d.Records), len(d.Imports)}
		return nil
	})
	return out, e
}
func (s *Store) RetentionImport(ctx context.Context, key string) (RetentionImport, error) {
	var out RetentionImport
	e := s.retention(ctx, false, func(_ int, d *retentionLedger) error {
		var ok bool
		out, ok = d.Imports[key]
		if !ok {
			return ErrRetentionNotFound
		}
		return nil
	})
	return out, e
}
func sameImportInput(a, b RetentionImport) bool {
	return a.RequestID == b.RequestID && a.Scope == b.Scope && a.ActorID == b.ActorID && a.InputDigest == b.InputDigest && a.ExpectedSnapshot == b.ExpectedSnapshot && a.Reason == b.Reason
}
func (s *Store) ReserveRetentionImport(ctx context.Context, in RetentionImport) (RetentionImport, error) {
	var out RetentionImport
	if !validRetentionImport(in) || in.State != "pending" {
		return out, ErrUnavailable
	}
	e := s.retention(ctx, true, func(fd int, d *retentionLedger) error {
		if old, ok := d.Imports[in.RequestID]; ok {
			if !sameImportInput(old, in) {
				return ErrRetentionConflict
			}
			out = old
			return nil
		}
		if len(d.Imports) >= MaxRetentionImports {
			return ErrRetentionFull
		}
		if d.Imports == nil {
			d.Imports = map[string]RetentionImport{}
		}
		d.Imports[in.RequestID] = in
		if e := saveRetentionLedger(fd, *d); e != nil {
			return e
		}
		out = in
		return nil
	})
	return out, e
}

// ApplyRetentionImport holds the existing independent lock once. The callback
// reads a bounded SQL snapshot; one checkpoint commits records and receipt.
func (s *Store) ApplyRetentionImport(ctx context.Context, in RetentionImport, plan func(Epoch, map[string]RetentionRecord) (RetentionImportPlan, error)) (RetentionImport, error) {
	var out RetentionImport
	e := s.retention(ctx, true, func(fd int, d *retentionLedger) error {
		old, ok := d.Imports[in.RequestID]
		if !ok || !sameImportInput(old, in) {
			return ErrRetentionConflict
		}
		if old.State != "pending" {
			out = old
			return nil
		}
		epoch, e := s.readEpoch(fd)
		if e != nil {
			return e
		}
		p := RetentionImportPlan{Problem: "BACKUP_HISTORY_RECOVERY_CHANGED"}
		if RetentionGeneration(old.Scope, epoch) == old.Generation {
			frozen := map[string]RetentionRecord{}
			for k, v := range d.Records {
				frozen[k] = v
			}
			p, e = plan(epoch, frozen)
			if e != nil {
				return e
			}
		}
		if p.Total < 0 || p.Covered < 0 || p.Covered > p.Total || p.Unknown < 0 || p.Unknown > p.Covered {
			return ErrUnavailable
		}
		if p.Problem == "" && p.Covered != p.Total {
			return ErrUnavailable
		}
		if p.Problem == "" {
			if len(d.Records)+len(p.Records) > MaxRetentionRecords {
				newCount := len(d.Records)
				for _, r := range p.Records {
					if _, ok := d.Records[r.RequestID]; !ok {
						newCount++
					}
				}
				if newCount > MaxRetentionRecords {
					p.Problem = "BACKUP_HISTORY_LEDGER_FULL"
				}
			}
		}
		if p.Problem == "" {
			for _, r := range p.Records {
				if !validRetention(r) || r.Scope != old.Scope {
					return ErrUnavailable
				}
				if before, exists := d.Records[r.RequestID]; exists {
					if !sameRetentionIdentity(before, r) || before.State != "checking" && !reflect.DeepEqual(before, r) {
						return ErrRetentionConflict
					}
				}
				d.Records[r.RequestID] = r
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		old.CompletedAt = &now
		old.SourceDigest = p.SourceDigest
		old.Total = p.Total
		old.Covered = p.Covered
		old.Unknown = p.Unknown
		if p.Problem != "" {
			old.State = "rejected"
			old.Problem = p.Problem
		} else {
			old.State = "completed"
			old.Complete = true
			old.Imported = len(p.Records)
			old.RecordDigest = RecordsDigest(d.Records)
		}
		if !validRetentionImport(old) {
			return ErrUnavailable
		}
		d.Imports[old.RequestID] = old
		if e = saveRetentionLedger(fd, *d); e != nil {
			return e
		}
		out = old
		return nil
	})
	return out, e
}

func (s *Store) RetentionImports(ctx context.Context, scope string) ([]RetentionImport, error) {
	out := []RetentionImport{}
	e := s.retention(ctx, false, func(_ int, d *retentionLedger) error {
		for _, r := range d.Imports {
			if r.Scope == scope {
				out = append(out, r)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].RequestID > out[j].RequestID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > 20 {
		out = out[:20]
	}
	return out, e
}
