package storagerecovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

const retentionFile = "storage-retention-ledger.json"
const retentionProtocol = "powerx.backup-retention-ledger/v4"
const MaxRetentionRecords = 4096
const maxRetentionBytes = 16 << 20

var ErrRetentionNotFound = errors.New("retention recovery record not found")
var ErrRetentionConflict = errors.New("retention recovery identity conflict")
var ErrRetentionFull = errors.New("retention recovery ledger full")
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var problemPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,95}$`)

// RetentionRecord 仅保存请求身份与历史事实，不包含路径、凭据、命令或当前执行授权。
type RetentionRecord struct {
	Operation         string     `json:"operation,omitempty"`
	ExpectedDiagnosis string     `json:"expected_diagnosis,omitempty"`
	DiagnosticDigest  string     `json:"diagnostic_digest,omitempty"`
	HistoryDigest     string     `json:"history_digest,omitempty"`
	JobID             uint64     `json:"job_id,omitempty"`
	PolicyID          uint64     `json:"policy_id,omitempty"`
	RequestID         string     `json:"request_id"`
	Scope             string     `json:"scope"`
	InputDigest       string     `json:"input_digest"`
	ReviewVersion     string     `json:"review_version"`
	Generation        string     `json:"generation"`
	ArtifactID        uint64     `json:"artifact_id"`
	ActorID           uint64     `json:"actor_id"`
	Token             string     `json:"token"`
	Reason            string     `json:"reason"`
	CreatedAt         time.Time  `json:"created_at"`
	State             string     `json:"state"`
	Problem           string     `json:"problem,omitempty"`
	CompletedAt       *time.Time `json:"completed_at"`
}
type retentionLedger struct {
	Protocol     string                     `json:"protocol"`
	Installation string                     `json:"installation"`
	AnchorID     string                     `json:"anchor_id"`
	ID           string                     `json:"id"`
	Records      map[string]RetentionRecord `json:"records"`
	Imports      map[string]RetentionImport `json:"imports,omitempty"`
	Digest       string                     `json:"digest"`
}

func ledgerDigest(d retentionLedger) string {
	d.Digest = ""
	raw, _ := json.Marshal(d)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validRetention(r RetentionRecord) bool {
	terminal := "retained"
	if r.Operation == "quarantine_cleanup" {
		terminal = "quarantined"
		if !ValidQuarantineDiagnosis(r.ExpectedDiagnosis) || !digestPattern.MatchString(r.DiagnosticDigest) {
			return false
		}
	} else if r.ExpectedDiagnosis != "" || r.DiagnosticDigest != "" {
		return false
	}
	if r.Operation == "" {
		if r.HistoryDigest != "" || r.JobID != 0 || r.PolicyID != 0 {
			return false
		}
	} else if (r.Operation != "retain_cleanup" && r.Operation != "quarantine_cleanup") || !digestPattern.MatchString(r.HistoryDigest) || r.JobID == 0 || r.PolicyID == 0 || !digestPattern.MatchString(r.Generation) {
		return false
	}
	return validUUID(r.RequestID) && r.RequestID != uuid.Nil.String() && validUUID(r.Token) && r.Token != uuid.Nil.String() && r.ArtifactID > 0 && r.ActorID > 0 && digestPattern.MatchString(r.Scope) && digestPattern.MatchString(r.InputDigest) && digestPattern.MatchString(r.ReviewVersion) && (r.Generation == "" || digestPattern.MatchString(r.Generation)) && strings.TrimSpace(r.Reason) == r.Reason && utf8.RuneCountInString(r.Reason) >= 3 && utf8.RuneCountInString(r.Reason) <= 256 && !r.CreatedAt.IsZero() && (r.State == "checking" && r.CompletedAt == nil && r.Problem == "" || (r.State == terminal && r.Problem == "" || r.State == "rejected" && problemPattern.MatchString(r.Problem)) && r.CompletedAt != nil && !r.CompletedAt.Before(r.CreatedAt))
}

func ValidQuarantineDiagnosis(s string) bool {
	switch s {
	case "missing", "size_changed", "checksum_mismatch", "sidecars_missing", "sidecar_mismatch", "metadata_invalid":
		return true
	}
	return false
}
func readRetentionLedger(fd int, b binding) (retentionLedger, error) {
	return readRetentionLedgerVersion(fd, b, false)
}
func readRetentionLedgerVersion(fd int, b binding, legacy bool) (retentionLedger, error) {
	var d retentionLedger
	n, e := unix.Openat(fd, retentionFile, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return d, ErrUnavailable
	}
	f := os.NewFile(uintptr(n), retentionFile)
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(n, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || st.Mode&0077 != 0 || st.Size < 2 || st.Size > maxRetentionBytes {
		return d, ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(f, maxRetentionBytes+1))
	if e != nil || len(raw) > maxRetentionBytes || strict(raw, &d) != nil || (d.Protocol != retentionProtocol && !(legacy && (d.Protocol == "powerx.backup-retention-ledger/v1" || d.Protocol == "powerx.backup-retention-ledger/v2" || d.Protocol == "powerx.backup-retention-ledger/v3"))) || d.Installation != b.Installation || d.AnchorID != b.AnchorID || !validUUID(d.ID) || b.RetentionLedgerID != "" && d.ID != b.RetentionLedgerID || d.Records == nil || len(d.Records) > MaxRetentionRecords || len(d.Imports) > MaxRetentionImports || !digestPattern.MatchString(d.Digest) || ledgerDigest(d) != d.Digest {
		return d, ErrUnavailable
	}
	for key, r := range d.Records {
		if key != r.RequestID || !validRetention(r) || d.Protocol != retentionProtocol && r.Operation != "" && !(d.Protocol == "powerx.backup-retention-ledger/v3" && r.Operation == "retain_cleanup" && r.ExpectedDiagnosis == "" && r.DiagnosticDigest == "") {
			return d, ErrUnavailable
		}
	}
	for key, r := range d.Imports {
		if key != r.RequestID || !validRetentionImport(r) {
			return d, ErrUnavailable
		}
	}
	if d.Protocol == "powerx.backup-retention-ledger/v1" && len(d.Imports) != 0 {
		return d, ErrUnavailable
	}
	return d, nil
}
func saveRetentionLedger(fd int, d retentionLedger) error {
	d.Digest = ledgerDigest(d)
	raw, e := json.Marshal(d)
	if e != nil || len(raw) > maxRetentionBytes {
		return ErrRetentionFull
	}
	return writeBytes(fd, retentionFile, raw)
}
func initializeRetentionLedger(fd int, b binding) error {
	// No overwrite, even for an interrupted installation. Reentry needs the
	// existing complete anchor identity, never a newly generated replacement.
	n, e := unix.Openat(fd, retentionFile, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e == nil {
		unix.Close(n)
		return ErrUnavailable
	}
	if !errors.Is(e, unix.ENOENT) {
		return ErrUnavailable
	}
	return saveRetentionLedger(fd, retentionLedger{Protocol: retentionProtocol, Installation: b.Installation, AnchorID: b.AnchorID, ID: b.RetentionLedgerID, Records: map[string]RetentionRecord{}})
}
func prepareRetentionLedger(fd int, b binding) (string, error) {
	d, e := readRetentionLedgerVersion(fd, b, true)
	if e == nil {
		if d.Protocol != retentionProtocol {
			d.Protocol = retentionProtocol
			if e = saveRetentionLedger(fd, d); e != nil {
				return "", e
			}
		}
		return d.ID, nil
	}
	// Only a pre-v4 binding can prepare its first ledger. A partial/corrupt
	// existing file is not missing and cannot be silently replaced.
	if b.Protocol == Protocol || b.Protocol == ResolutionProtocol || b.Protocol == ImportProtocol || b.Protocol == PreviousProtocol || b.RetentionLedgerID != "" {
		return "", ErrUnavailable
	}
	b.RetentionLedgerID = uuid.NewString()
	if e = initializeRetentionLedger(fd, b); e != nil {
		return "", e
	}
	return b.RetentionLedgerID, nil
}
func ValidRetentionRecord(r RetentionRecord) bool { return validRetention(r) }
func (s *Store) retention(ctx context.Context, write bool, fn func(int, *retentionLedger) error) error {
	if s == nil || fn == nil {
		return ErrUnavailable
	}
	return s.locked(ctx, write, func(fd int) error {
		if write {
			if _, e := s.readEpoch(fd); e != nil {
				return e
			}
		}
		var b binding
		raw, e := read(fd, "storage-recovery-binding.json")
		if e != nil || strict(raw, &b) != nil || b.Protocol != Protocol || b.Installation != s.installation || !validUUID(b.AnchorID) || !validUUID(b.RetentionLedgerID) {
			return ErrUnavailable
		}
		d, e := readRetentionLedger(fd, b)
		if e != nil {
			return e
		}
		return fn(fd, &d)
	})
}
func (s *Store) RetentionReady(ctx context.Context) error {
	return s.retention(ctx, false, func(int, *retentionLedger) error { return nil })
}
func (s *Store) Retention(ctx context.Context, key string) (RetentionRecord, error) {
	var out RetentionRecord
	if !validUUID(key) {
		return out, ErrUnavailable
	}
	e := s.retention(ctx, false, func(_ int, d *retentionLedger) error {
		var ok bool
		out, ok = d.Records[key]
		if !ok {
			return ErrRetentionNotFound
		}
		return nil
	})
	return out, e
}
func (s *Store) Retentions(ctx context.Context, scope string, artifact uint64, token string) ([]RetentionRecord, error) {
	out := []RetentionRecord{}
	e := s.retention(ctx, false, func(_ int, d *retentionLedger) error {
		for _, r := range d.Records {
			if r.Operation == "" && r.Scope == scope && r.ArtifactID == artifact && r.Token == token {
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
func sameRetentionIdentity(a, b RetentionRecord) bool {
	a.State, b.State = "", ""
	a.Problem, b.Problem = "", ""
	a.CompletedAt, b.CompletedAt = nil, nil
	return reflect.DeepEqual(a, b)
}

// CleanupResolutions 保留独立结果读取，不要求数据库或当前 epoch 可用。
func (s *Store) CleanupResolutions(ctx context.Context, scope string, artifact uint64, token string) ([]RetentionRecord, error) {
	out := []RetentionRecord{}
	e := s.retention(ctx, false, func(_ int, d *retentionLedger) error {
		for _, r := range d.Records {
			if (r.Operation == "retain_cleanup" || r.Operation == "quarantine_cleanup") && r.Scope == scope && r.ArtifactID == artifact && r.Token == token {
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
	return out, e
}

// ReserveRetention fsyncs identity before SQL admission. Existing UUID input is
// immutable, including after SQL rollback. Terminal records are never pruned.
func (s *Store) ReserveRetention(ctx context.Context, in RetentionRecord) (RetentionRecord, error) {
	r, _, e := s.ReserveRetentionAdmission(ctx, in)
	return r, e
}
func (s *Store) ReserveRetentionAdmission(ctx context.Context, in RetentionRecord) (RetentionRecord, bool, error) {
	var out RetentionRecord
	created := false
	if !validRetention(in) || in.State != "checking" {
		return out, false, ErrUnavailable
	}
	e := s.retention(ctx, true, func(fd int, d *retentionLedger) error {
		if old, ok := d.Records[in.RequestID]; ok {
			// Caller does not choose a different admission timestamp on retry.
			in.CreatedAt = old.CreatedAt
			if !sameRetentionIdentity(old, in) {
				return ErrRetentionConflict
			}
			out = old
			return nil
		}
		if len(d.Records) >= MaxRetentionRecords {
			return ErrRetentionFull
		}
		d.Records[in.RequestID] = in
		if e := saveRetentionLedger(fd, *d); e != nil {
			return e
		}
		out = in
		created = true
		return nil
	})
	return out, created, e
}
func (s *Store) CompleteRetention(ctx context.Context, in RetentionRecord) error {
	if !validRetention(in) || in.State == "checking" {
		return ErrUnavailable
	}
	return s.retention(ctx, true, func(fd int, d *retentionLedger) error {
		old, ok := d.Records[in.RequestID]
		if !ok || !sameRetentionIdentity(old, in) {
			return ErrRetentionConflict
		}
		if old.State != "checking" {
			if !reflect.DeepEqual(old, in) {
				return ErrRetentionConflict
			}
			return nil
		}
		d.Records[in.RequestID] = in
		return saveRetentionLedger(fd, *d)
	})
}
