package backup_ops

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type RetentionHistoryItem struct {
	RowID      string    `json:"row_id"`
	RequestID  string    `json:"request_id"`
	ArtifactID string    `json:"artifact_id"`
	ActorID    string    `json:"actor_id"`
	State      string    `json:"state"`
	Coverage   string    `json:"coverage"`
	Problem    string    `json:"problem,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
type RetentionHistory struct {
	Snapshot     string                 `json:"snapshot"`
	SourceDigest string                 `json:"source_digest"`
	ObservedAt   time.Time              `json:"observed_at"`
	Total        int                    `json:"total"`
	Foreign      int                    `json:"foreign"`
	Covered      int                    `json:"covered"`
	Missing      int                    `json:"missing"`
	Conflicts    int                    `json:"conflicts"`
	Invalid      int                    `json:"invalid"`
	Unknown      int                    `json:"unknown"`
	Complete     bool                   `json:"complete"`
	RestoreReady bool                   `json:"restore_ready"`
	CanImport    bool                   `json:"can_import"`
	PauseReady   bool                   `json:"pause_ready"`
	Problem      string                 `json:"problem,omitempty"`
	Remaining    int                    `json:"remaining"`
	Page         int                    `json:"page"`
	PageSize     int                    `json:"page_size"`
	Filtered     int                    `json:"filtered"`
	Items        []RetentionHistoryItem `json:"items"`
}
type RetentionImportInput struct {
	RequestID        string `json:"request_id"`
	ExpectedSnapshot string `json:"expected_snapshot"`
	Reason           string `json:"reason"`
	Confirm          bool   `json:"confirm"`
}
type RetentionImportReceipt struct {
	RequestID        string     `json:"request_id"`
	ActorID          string     `json:"actor_id"`
	ExpectedSnapshot string     `json:"expected_snapshot"`
	Reason           string     `json:"reason"`
	State            string     `json:"state"`
	Problem          string     `json:"problem,omitempty"`
	SourceDigest     string     `json:"source_digest,omitempty"`
	Total            int        `json:"total"`
	Covered          int        `json:"covered"`
	Imported         int        `json:"imported"`
	Unknown          int        `json:"unknown"`
	Complete         bool       `json:"complete"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	CanResume        bool       `json:"can_resume"`
}
type retentionImportAuthority interface {
	RetentionImportGuard(context.Context, recovery.Epoch, func(context.Context, *gorm.DB) error) error
	RetentionImportPaused(context.Context) bool
}
type historyAnalysis struct {
	View           RetentionHistory
	Changes        []recovery.RetentionRecord
	CoveredUnknown int
}

func historyHash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func validHistoryRow(r m.BackupCleanupRetention) bool {
	if r.DeletedAt.Valid || r.RequestID == uuid.Nil {
		return false
	}
	in := CleanupRetainInput{r.RequestID.String(), r.CleanupToken, r.ReviewVersion, r.Reason, true}
	return validateRetainInput(r.ArtifactID, in) && retainInputDigest(r.ArtifactID, in) == r.InputDigest && recovery.ValidRetentionRecord(retentionRecord(r, ""))
}
func historyCoverage(row m.BackupCleanupRetention, records map[string]recovery.RetentionRecord) (string, string, recovery.RetentionRecord) {
	in := retentionRecord(row, "")
	if !validHistoryRow(row) {
		return "invalid", "BACKUP_HISTORY_RECORD_INVALID", in
	}
	old, ok := records[in.RequestID]
	if !ok {
		return "missing", "", in
	}
	if !recordMatchesRow(old, row) {
		return "conflict", "BACKUP_HISTORY_IDENTITY_CONFLICT", in
	}
	if old.State == "checking" && in.State != "checking" {
		in.Generation = old.Generation
		return "missing", "", in
	}
	if old.State != "checking" && in.State != "checking" {
		if old.State != in.State || old.Problem != in.Problem || old.CompletedAt == nil || in.CompletedAt == nil || !old.CompletedAt.Equal(*in.CompletedAt) {
			return "conflict", "BACKUP_HISTORY_RESULT_CONFLICT", in
		}
	}
	return "covered", "", old
}
func analyzeHistory(rows []m.BackupCleanupRetention, total, foreign int64, scope, source string, inv recovery.RetentionInventory) historyAnalysis {
	a := historyAnalysis{View: RetentionHistory{Total: int(total), Foreign: int(foreign), ObservedAt: time.Now().UTC(), Items: []RetentionHistoryItem{}, Remaining: recovery.MaxRetentionRecords - len(inv.Records)}}
	type sourceRow struct {
		ID        uint64
		UUID      string
		UpdatedAt time.Time
		Deleted   bool
		Record    recovery.RetentionRecord
	}
	canonical := []sourceRow{}
	newCount := 0
	for _, row := range rows {
		record := retentionRecord(row, "")
		canonical = append(canonical, sourceRow{row.ID, row.UUID.String(), row.UpdatedAt.UTC(), row.DeletedAt.Valid, record})
		coverage, problem, next := historyCoverage(row, inv.Records)
		switch coverage {
		case "covered":
			a.View.Covered++
			if next.State == "checking" {
				a.CoveredUnknown++
				a.View.Unknown++
			}
		case "missing":
			a.View.Missing++
			a.Changes = append(a.Changes, next)
			if _, ok := inv.Records[next.RequestID]; !ok {
				newCount++
			}
			if next.State == "checking" {
				a.View.Unknown++
			}
		case "conflict":
			a.View.Conflicts++
		case "invalid":
			a.View.Invalid++
		}
		a.View.Items = append(a.View.Items, RetentionHistoryItem{strconv.FormatUint(row.ID, 10), row.RequestID.String(), strconv.FormatUint(row.ArtifactID, 10), strconv.FormatUint(row.ActorID, 10), row.State, coverage, problem, row.CreatedAt})
	}
	a.View.SourceDigest = historyHash(struct {
		Scope, Source  string
		Total, Foreign int64
		Rows           []sourceRow
	}{scope, source, total, foreign, canonical})
	a.View.Snapshot = historyHash(struct{ Source, Records, Generation string }{a.View.SourceDigest, inv.RecordDigest, recovery.RetentionGeneration(scope, inv.Epoch)})
	switch {
	case total > recovery.MaxRetentionRecords:
		a.View.Problem = "BACKUP_HISTORY_SOURCE_LIMIT"
	case a.View.Invalid > 0:
		a.View.Problem = "BACKUP_HISTORY_RECORD_INVALID"
	case a.View.Conflicts > 0:
		a.View.Problem = "BACKUP_HISTORY_CONFLICT"
	case newCount > a.View.Remaining:
		a.View.Problem = "BACKUP_HISTORY_LEDGER_FULL"
	}
	a.View.Complete = a.View.Problem == "" && a.View.Covered == a.View.Total
	a.View.RestoreReady = a.View.Complete && foreign == 0
	return a
}
func (s *JobService) RetentionHistory(ctx context.Context, page, size int, filter string) (RetentionHistory, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return RetentionHistory{}, e
	}
	if page < 1 || page > 10000 || size < 1 || size > 50 || filter != "all" && filter != "covered" && filter != "missing" && filter != "conflict" && filter != "invalid" {
		return RetentionHistory{}, controlFailure(400, "BACKUP_HISTORY_QUERY_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if e := s.requireRetainSchema(run); e != nil {
		return RetentionHistory{}, e
	}
	store, e := s.retentionLedger()
	if e != nil {
		return RetentionHistory{}, e
	}
	inv, e := store.RetentionInventory(run)
	if e != nil {
		return RetentionHistory{}, ledgerError(e)
	}
	var rows []m.BackupCleanupRetention
	var total, foreign int64
	e = s.db.WithContext(run).Transaction(func(tx *gorm.DB) error {
		var err error
		rows, total, foreign, err = s.retainRepo.HistorySnapshot(run, tx, s.retentionScope(), false, recovery.MaxRetentionRecords+1)
		return err
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return RetentionHistory{}, controlFailure(503, "BACKUP_HISTORY_SOURCE_UNAVAILABLE")
	}
	epoch, e := store.Read(run)
	if e != nil || epoch != inv.Epoch {
		return RetentionHistory{}, controlFailure(409, "BACKUP_HISTORY_RECOVERY_CHANGED")
	}
	a := analyzeHistory(rows, total, foreign, s.retentionScope(), s.ExecutionFingerprint(), inv)
	if authority, ok := s.authority.(retentionImportAuthority); ok {
		a.View.PauseReady = authority.RetentionImportPaused(run)
	}
	a.View.CanImport = a.View.Problem == "" && a.View.PauseReady && inv.ImportCount < recovery.MaxRetentionImports
	if inv.ImportCount >= recovery.MaxRetentionImports {
		a.View.Problem = "BACKUP_HISTORY_IMPORT_LIMIT"
	}
	items := []RetentionHistoryItem{}
	for _, r := range a.View.Items {
		if filter == "all" || r.Coverage == filter {
			items = append(items, r)
		}
	}
	a.View.Filtered = len(items)
	a.View.Page = page
	a.View.PageSize = size
	start := (page - 1) * size
	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	a.View.Items = items[start:end]
	return a.View, nil
}
func importReceipt(ctx context.Context, r recovery.RetentionImport) RetentionImportReceipt {
	return RetentionImportReceipt{r.RequestID, strconv.FormatUint(r.ActorID, 10), r.ExpectedSnapshot, r.Reason, r.State, r.Problem, r.SourceDigest, r.Total, r.Covered, r.Imported, r.Unknown, r.Complete, r.CreatedAt, r.CompletedAt, r.State == "pending" && r.ActorID == reqctx.GetClaims(ctx).UserID}
}
func (s *JobService) RetentionImportRequest(ctx context.Context, key string) (RetentionImportReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return RetentionImportReceipt{}, e
	}
	if !validCleanupToken(key) {
		return RetentionImportReceipt{}, controlFailure(400, "BACKUP_HISTORY_INPUT_INVALID")
	}
	store, e := s.retentionLedger()
	if e != nil {
		return RetentionImportReceipt{}, e
	}
	r, e := store.RetentionImport(ctx, key)
	if errors.Is(e, recovery.ErrRetentionNotFound) {
		return RetentionImportReceipt{}, controlFailure(404, "BACKUP_HISTORY_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return RetentionImportReceipt{}, ledgerError(e)
	}
	if r.Scope != s.retentionScope() {
		return RetentionImportReceipt{}, controlFailure(403, "BACKUP_RETAIN_SCOPE_MISMATCH")
	}
	return importReceipt(ctx, r), nil
}
func (s *JobService) RetentionImportRequests(ctx context.Context) ([]RetentionImportReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return nil, e
	}
	store, e := s.retentionLedger()
	if e != nil {
		return nil, e
	}
	rows, e := store.RetentionImports(ctx, s.retentionScope())
	if e != nil {
		return nil, ledgerError(e)
	}
	out := []RetentionImportReceipt{}
	for _, r := range rows {
		out = append(out, importReceipt(ctx, r))
	}
	return out, nil
}
func (s *JobService) ImportRetentionHistory(ctx context.Context, in RetentionImportInput, send func(string, RetentionImportReceipt) error) (RetentionImportReceipt, error) {
	if e := s.requireRetainRoot(ctx); e != nil {
		return RetentionImportReceipt{}, e
	}
	if !validCleanupToken(in.RequestID) || len(in.ExpectedSnapshot) != 64 || !cleanupReviewChecksum.MatchString(in.ExpectedSnapshot) || !in.Confirm || send == nil || strings.ToLower(in.ExpectedSnapshot) != in.ExpectedSnapshot || strings.TrimSpace(in.Reason) != in.Reason || utf8.RuneCountInString(in.Reason) < 3 || utf8.RuneCountInString(in.Reason) > 256 {
		return RetentionImportReceipt{}, controlFailure(400, "BACKUP_HISTORY_INPUT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	store, e := s.retentionLedger()
	if e != nil {
		return RetentionImportReceipt{}, e
	}
	actor := reqctx.GetClaims(ctx).UserID
	digest := historyHash(struct {
		Scope string
		Actor uint64
		Input RetentionImportInput
	}{s.retentionScope(), actor, in})
	old, e := store.RetentionImport(run, in.RequestID)
	if e == nil {
		if old.ActorID != actor {
			return RetentionImportReceipt{}, controlFailure(403, "BACKUP_RETAIN_ACTOR_MISMATCH")
		}
		if old.InputDigest != digest || old.Scope != s.retentionScope() {
			return RetentionImportReceipt{}, controlFailure(409, "BACKUP_RETAIN_IDEMPOTENCY_CONFLICT")
		}
		if old.State != "pending" {
			return importReceipt(ctx, old), nil
		}
	} else if !errors.Is(e, recovery.ErrRetentionNotFound) {
		return RetentionImportReceipt{}, ledgerError(e)
	}
	if e = s.requireRetainSchema(run); e != nil {
		return RetentionImportReceipt{}, e
	}
	generation, e := s.retentionReviewBinding(run)
	if e != nil {
		return RetentionImportReceipt{}, e
	}
	r := recovery.RetentionImport{RequestID: in.RequestID, Scope: s.retentionScope(), ActorID: actor, InputDigest: digest, ExpectedSnapshot: in.ExpectedSnapshot, Generation: generation, Reason: in.Reason, CreatedAt: time.Now().UTC().Truncate(time.Microsecond), State: "pending"}
	r, e = store.ReserveRetentionImport(run, r)
	if e != nil {
		return RetentionImportReceipt{}, ledgerError(e)
	}
	if r.State != "pending" {
		return importReceipt(ctx, r), nil
	}
	s.cleanupSvc.notifyChanged()
	if e = send("accepted", importReceipt(ctx, r)); e != nil {
		return RetentionImportReceipt{}, controlFailure(503, "BACKUP_HISTORY_RESULT_UNKNOWN")
	}
	authority, ok := s.authority.(retentionImportAuthority)
	if !ok {
		return RetentionImportReceipt{}, controlFailure(503, "BACKUP_HISTORY_CONTROL_UNAVAILABLE")
	}
	r, e = store.ApplyRetentionImport(run, r, func(epoch recovery.Epoch, records map[string]recovery.RetentionRecord) (recovery.RetentionImportPlan, error) {
		var plan recovery.RetentionImportPlan
		e := authority.RetentionImportGuard(run, epoch, func(c context.Context, tx *gorm.DB) error {
			rows, total, foreign, e := s.retainRepo.HistorySnapshot(c, tx, s.retentionScope(), true, recovery.MaxRetentionRecords+1)
			if e != nil {
				return e
			}
			a := analyzeHistory(rows, total, foreign, s.retentionScope(), s.ExecutionFingerprint(), recovery.RetentionInventory{Epoch: epoch, Records: records, RecordDigest: recovery.RecordsDigest(records)})
			plan = recovery.RetentionImportPlan{SourceDigest: a.View.SourceDigest, Total: a.View.Total, Covered: a.View.Covered, Unknown: a.CoveredUnknown, Problem: a.View.Problem}
			if a.View.Snapshot != in.ExpectedSnapshot {
				plan.Problem = "BACKUP_HISTORY_SNAPSHOT_CHANGED"
			}
			if plan.Problem == "" {
				plan.Records = a.Changes
				plan.Covered = plan.Total
				plan.Unknown = a.View.Unknown
			}
			return nil
		})
		return plan, e
	})
	if e != nil {
		return RetentionImportReceipt{}, controlFailure(503, "BACKUP_HISTORY_RESULT_UNKNOWN")
	}
	s.cleanupSvc.notifyChanged()
	return importReceipt(ctx, r), nil
}
