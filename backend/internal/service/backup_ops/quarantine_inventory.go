package backup_ops

import (
	"context"
	"database/sql"
	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// QuarantineInventoryOptions 只接受固定过滤与排序；查询不修改任何回执或当前状态。
type QuarantineInventoryOptions struct {
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
	Current   string `form:"current"`
	Result    string `form:"result"`
	Diagnosis string `form:"diagnosis"`
	Query     string `form:"q"`
	Sort      string `form:"sort"`
	Direction string `form:"direction"`
}

// QuarantineInventoryItem 将历史事实和当前 SQL 观察分开，Snapshot 只有只读复查用途。
type QuarantineInventoryItem struct {
	RequestID        string     `json:"request_id"`
	ArtifactID       string     `json:"artifact_id"`
	JobID            string     `json:"job_id"`
	PolicyID         string     `json:"policy_id"`
	Token            string     `json:"token"`
	Result           string     `json:"result"`
	Diagnosis        string     `json:"diagnosis"`
	Reason           string     `json:"reason"`
	ActorID          string     `json:"actor_id"`
	CreatedAt        time.Time  `json:"created_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	HistoryStatus    string     `json:"history_status"`
	Current          string     `json:"current"`
	CleanupState     string     `json:"cleanup_state"`
	SizeBytes        *int64     `json:"size_bytes"`
	CanInspect       bool       `json:"can_inspect"`
	Snapshot         string     `json:"snapshot"`
	ValidBackupProof bool       `json:"valid_backup_proof"`
}
type QuarantineInventoryPage struct {
	Items                            []QuarantineInventoryItem `json:"items"`
	Total                            int                       `json:"total"`
	Page                             int                       `json:"page"`
	PageSize                         int                       `json:"page_size"`
	UnattributedQuarantinedArtifacts int                       `json:"unattributed_quarantined_artifacts"`
	ObservedAt                       time.Time                 `json:"observed_at"`
}
type quarantineInventory struct {
	items        []QuarantineInventoryItem
	rows         map[uint64]repo.CleanupReviewRow
	unattributed int
}

func quarantineInventoryError() error {
	return controlFailure(503, "BACKUP_QUARANTINE_INVENTORY_UNAVAILABLE")
}
func member(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func validInventoryOptions(o QuarantineInventoryOptions) bool {
	return o.Page > 0 && o.Page <= 100000 && o.PageSize > 0 && o.PageSize <= 100 && member(o.Current, "all", "quarantined", "not_quarantined", "missing", "unavailable", "identity_changed") && member(o.Result, "all", "checking", "quarantined", "rejected") && (o.Diagnosis == "all" || recovery.ValidQuarantineDiagnosis(o.Diagnosis)) && len(o.Query) <= 64 && o.Query == strings.TrimSpace(o.Query) && member(o.Sort, "time", "size", "artifact") && member(o.Direction, "asc", "desc")
}
func inventoryItem(r recovery.RetentionRecord, sqlRow *m.BackupCleanupResolution, current *repo.CleanupReviewRow, generation string, independent bool) QuarantineInventoryItem {
	out := QuarantineInventoryItem{RequestID: r.RequestID, ArtifactID: strconv.FormatUint(r.ArtifactID, 10), JobID: strconv.FormatUint(r.JobID, 10), PolicyID: strconv.FormatUint(r.PolicyID, 10), Token: r.Token, Result: r.State, Diagnosis: r.ExpectedDiagnosis, Reason: r.Reason, ActorID: strconv.FormatUint(r.ActorID, 10), CreatedAt: r.CreatedAt, CompletedAt: r.CompletedAt, Current: "missing", HistoryStatus: "covered"}
	switch {
	case !independent:
		out.HistoryStatus = "independent_missing"
	case sqlRow == nil:
		out.HistoryStatus = "sql_missing"
	case !resolutionMatches(r, *sqlRow):
		out.HistoryStatus = "conflict"
	case r.State == "checking" && sqlRow.State != "checking":
		out.HistoryStatus = "checkpoint_required"
	case r.State != "checking" && sqlRow.State == "checking":
		out.HistoryStatus = "sql_replayed"
	case !reflect.DeepEqual(r, resolutionRecord(*sqlRow)):
		out.HistoryStatus = "conflict"
	}
	if current != nil {
		size := current.SizeBytes
		out.SizeBytes = &size
		switch {
		case current.CleanupToken != r.Token || current.JobID != r.JobID:
			out.Current = "identity_changed"
		case current.ArtifactDeleted || !current.JobFound || current.JobDeleted:
			out.Current = "unavailable"
		case current.PolicyID != r.PolicyID:
			out.Current = "identity_changed"
		case current.CleanupState == "quarantined":
			out.Current = "quarantined"
		default:
			out.Current = "not_quarantined"
		}
		if member(current.CleanupState, "pending", "unconfirmed", "retained", "quarantined", "") {
			out.CleanupState = current.CleanupState
		} else {
			out.CleanupState = "other"
		}
		out.CanInspect = member(out.Current, "quarantined", "not_quarantined") && out.HistoryStatus != "conflict" && generation != ""
		if out.CanInspect {
			out.Snapshot = historyHash(struct {
				Contract, Generation, History string
				Record                        recovery.RetentionRecord
				Current                       repo.CleanupReviewRow
			}{"powerx.quarantine-inspection/v1", generation, out.HistoryStatus, r, *current})
		}
	}
	return out
}

// loadQuarantineInventory 有界独立凭据与一个只读 SQL 快照合并，绝不修补缺失来源。
func (s *JobService) loadQuarantineInventory(ctx context.Context) (quarantineInventory, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return quarantineInventory{}, e
	}
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if s.resolutionRepo == nil {
		return quarantineInventory{}, quarantineInventoryError()
	}
	store, e := s.retentionLedger()
	if e != nil {
		return quarantineInventory{}, e
	}
	before, e := store.RetentionInventory(run)
	if e != nil {
		return quarantineInventory{}, ledgerError(e)
	}
	records := map[string]recovery.RetentionRecord{}
	independent := map[string]bool{}
	for key, r := range before.Records {
		if r.Scope == s.retentionScope() && r.Operation == cleanupQuarantineOperation {
			records[key] = r
			independent[key] = true
		}
	}
	audits := map[string]m.BackupCleanupResolution{}
	out := quarantineInventory{items: []QuarantineInventoryItem{}, rows: map[uint64]repo.CleanupReviewRow{}}
	e = s.db.WithContext(run).Transaction(func(tx *gorm.DB) error {
		rows, e := s.resolutionRepo.QuarantineInventory(run, tx, s.retentionScope(), recovery.MaxRetentionRecords+1)
		if e != nil {
			return quarantineInventoryError()
		}
		if len(rows) > recovery.MaxRetentionRecords {
			return controlFailure(423, "BACKUP_QUARANTINE_INVENTORY_LIMIT")
		}
		for _, row := range rows {
			key := row.RequestID.String()
			audits[key] = row
			if !independent[key] {
				if !validResolutionRow(row) {
					return controlFailure(503, "BACKUP_QUARANTINE_INVENTORY_CONFLICT")
				}
				records[key] = resolutionRecord(row)
			}
		}
		if len(records) > recovery.MaxRetentionRecords {
			return controlFailure(423, "BACKUP_QUARANTINE_INVENTORY_LIMIT")
		}
		ids := []uint64{}
		seen := map[uint64]bool{}
		for _, r := range records {
			if !seen[r.ArtifactID] {
				ids = append(ids, r.ArtifactID)
				seen[r.ArtifactID] = true
			}
		}
		current, e := s.artifactRepo.CleanupInventoryRows(run, tx, ids)
		if e != nil {
			return quarantineInventoryError()
		}
		for _, r := range current {
			out.rows[r.ID] = r
		}
		keys, e := s.artifactRepo.QuarantinedInventoryKeys(run, tx, recovery.MaxRetentionRecords+1)
		if e != nil {
			return quarantineInventoryError()
		}
		if len(keys) > recovery.MaxRetentionRecords {
			return controlFailure(423, "BACKUP_QUARANTINE_INVENTORY_LIMIT")
		}
		attributed := map[uint64]string{}
		generation := recovery.RetentionGeneration(s.retentionScope(), before.Epoch)
		for key, r := range records {
			var row *repo.CleanupReviewRow
			if v, ok := out.rows[r.ArtifactID]; ok {
				row = &v
			}
			var audit *m.BackupCleanupResolution
			if v, ok := audits[key]; ok {
				audit = &v
			}
			item := inventoryItem(r, audit, row, generation, independent[key])
			out.items = append(out.items, item)
			if item.Current == "quarantined" && item.HistoryStatus != "conflict" {
				attributed[r.ArtifactID] = r.Token
			}
		}
		for _, key := range keys {
			if attributed[key.ID] != key.CleanupToken {
				out.unattributed++
			}
		}
		return nil
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return quarantineInventory{}, e
	}
	after, e := store.RetentionInventory(run)
	if e != nil {
		return quarantineInventory{}, ledgerError(e)
	}
	if before.RecordDigest != after.RecordDigest || recovery.RetentionGeneration(s.retentionScope(), before.Epoch) != recovery.RetentionGeneration(s.retentionScope(), after.Epoch) {
		return quarantineInventory{}, controlFailure(409, "BACKUP_QUARANTINE_INVENTORY_CHANGED")
	}
	return out, nil
}

// QuarantineInventory 支持服务端分页和白名单过滤／排序，不读取大文件。
func (s *JobService) QuarantineInventory(ctx context.Context, o QuarantineInventoryOptions) (QuarantineInventoryPage, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return QuarantineInventoryPage{}, e
	}
	if !validInventoryOptions(o) {
		return QuarantineInventoryPage{}, controlFailure(400, "BACKUP_QUARANTINE_INVENTORY_QUERY_INVALID")
	}
	data, e := s.loadQuarantineInventory(ctx)
	if e != nil {
		return QuarantineInventoryPage{}, e
	}
	return quarantineInventoryPage(data, o), nil
}
func quarantineInventoryPage(data quarantineInventory, o QuarantineInventoryOptions) QuarantineInventoryPage {
	out := QuarantineInventoryPage{Items: []QuarantineInventoryItem{}, Page: o.Page, PageSize: o.PageSize, UnattributedQuarantinedArtifacts: data.unattributed, ObservedAt: time.Now().UTC()}
	selected := []QuarantineInventoryItem{}
	for _, r := range data.items {
		if o.Current != "all" && r.Current != o.Current || o.Result != "all" && r.Result != o.Result || o.Diagnosis != "all" && r.Diagnosis != o.Diagnosis {
			continue
		}
		if o.Query != "" && !member(o.Query, r.RequestID, r.ArtifactID, r.JobID, r.Token) {
			continue
		}
		selected = append(selected, r)
	}
	sort.Slice(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		cmp := 0
		switch o.Sort {
		case "size":
			av, bv := int64(-1), int64(-1)
			if a.SizeBytes != nil {
				av = *a.SizeBytes
			}
			if b.SizeBytes != nil {
				bv = *b.SizeBytes
			}
			if av < bv {
				cmp = -1
			} else if av > bv {
				cmp = 1
			}
		case "artifact":
			av, _ := strconv.ParseUint(a.ArtifactID, 10, 64)
			bv, _ := strconv.ParseUint(b.ArtifactID, 10, 64)
			if av < bv {
				cmp = -1
			} else if av > bv {
				cmp = 1
			}
		default:
			av, bv := a.CreatedAt, b.CreatedAt
			if a.CompletedAt != nil {
				av = *a.CompletedAt
			}
			if b.CompletedAt != nil {
				bv = *b.CompletedAt
			}
			cmp = av.Compare(bv)
		}
		if cmp == 0 {
			cmp = strings.Compare(a.RequestID, b.RequestID)
		}
		if o.Direction == "desc" {
			return cmp > 0
		}
		return cmp < 0
	})
	out.Total = len(selected)
	start := (o.Page - 1) * o.PageSize
	if start < out.Total {
		end := min(start+o.PageSize, out.Total)
		out.Items = selected[start:end]
	}
	return out
}

// QuarantineInventoryRequest 返回精确历史 UUID 与当前状态；找不到不证明此前未执行。
func (s *JobService) QuarantineInventoryRequest(ctx context.Context, key string) (QuarantineInventoryItem, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return QuarantineInventoryItem{}, e
	}
	if !validCleanupToken(key) {
		return QuarantineInventoryItem{}, controlFailure(400, "BACKUP_QUARANTINE_INVENTORY_QUERY_INVALID")
	}
	data, e := s.loadQuarantineInventory(ctx)
	if e != nil {
		return QuarantineInventoryItem{}, e
	}
	for _, r := range data.items {
		if r.RequestID == key {
			return r, nil
		}
	}
	return QuarantineInventoryItem{}, controlFailure(404, "BACKUP_QUARANTINE_INVENTORY_NOT_FOUND")
}

// QuarantineInspection 是瞬时只读诊断；即使文件完整也不解除隔离或授予恢复资格。
type QuarantineInspection struct {
	RequestID            string              `json:"request_id"`
	Snapshot             string              `json:"snapshot"`
	Verification         CleanupVerification `json:"verification"`
	CurrentlyQuarantined bool                `json:"currently_quarantined"`
	ReadOnly             bool                `json:"read_only"`
	QuarantineUnchanged  bool                `json:"quarantine_unchanged"`
	ValidBackupProof     bool                `json:"valid_backup_proof"`
}

func (s *JobService) InspectQuarantineArtifact(ctx context.Context, key, snapshot string, progress func(CleanupVerification) error) (QuarantineInspection, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return QuarantineInspection{}, e
	}
	if !validCleanupToken(key) || !recoveryHash(snapshot) || progress == nil {
		return QuarantineInspection{}, controlFailure(400, "BACKUP_QUARANTINE_INVENTORY_QUERY_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	data, e := s.loadQuarantineInventory(run)
	if e != nil {
		return QuarantineInspection{}, e
	}
	var item QuarantineInventoryItem
	for _, r := range data.items {
		if r.RequestID == key {
			item = r
			break
		}
	}
	if item.RequestID == "" {
		return QuarantineInspection{}, controlFailure(404, "BACKUP_QUARANTINE_INVENTORY_NOT_FOUND")
	}
	if !item.CanInspect || item.Snapshot != snapshot {
		return QuarantineInspection{}, controlFailure(409, "BACKUP_QUARANTINE_INVENTORY_CHANGED")
	}
	id, _ := strconv.ParseUint(item.ArtifactID, 10, 64)
	row := data.rows[id]
	pool, e := s.db.DB()
	if e != nil || pool.Stats().MaxOpenConnections == 1 {
		return QuarantineInspection{}, controlFailure(503, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	}
	acquire, done := context.WithTimeout(run, 3*time.Second)
	unlock, e := lockBackupPolicy(acquire, s.db, row.PolicyID)
	done()
	if e != nil {
		return QuarantineInspection{}, controlFailure(409, "BACKUP_QUARANTINE_POLICY_BUSY")
	}
	defer unlock()
	check := func() error {
		fresh, e := s.QuarantineInventoryRequest(run, key)
		if e != nil {
			return e
		}
		if !fresh.CanInspect || fresh.Snapshot != snapshot {
			return controlFailure(409, "BACKUP_QUARANTINE_INVENTORY_CHANGED")
		}
		return nil
	}
	if e = check(); e != nil {
		return QuarantineInspection{}, e
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return QuarantineInspection{}, quarantineInventoryError()
	}
	verification, _, e := probeQuarantine(run, root, row, progress, nil)
	if e != nil {
		return QuarantineInspection{}, e
	}
	if e = check(); e != nil {
		return QuarantineInspection{}, e
	}
	return QuarantineInspection{key, snapshot, verification, item.Current == "quarantined", true, true, false}, nil
}
