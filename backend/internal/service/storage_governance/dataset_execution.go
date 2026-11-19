package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DatasetExecutionCapability = "com.corex.storage.datasets.execution_worker"
const datasetExecutionContract = "powerx.storage-dataset-execution/v1"
const datasetWriteAuthorization = 15 * time.Second
const datasetWriteBatchLimit = 128

type DatasetPolicyChange struct {
	ExpectedRevision   *int64 `json:"expected_revision"`
	AcceptanceRevision int64  `json:"acceptance_revision"`
	Enabled            *bool  `json:"enabled"`
	Action             string `json:"action"`
	RetentionDays      int    `json:"retention_days"`
	ArchiveTargetID    string `json:"archive_target_uuid,omitempty"`
}

type DatasetExecutionPolicy struct {
	DatasetUUID        string `json:"dataset_uuid"`
	TenantUUID         string `json:"tenant_uuid"`
	Revision           int64  `json:"revision"`
	AcceptanceRevision int64  `json:"acceptance_revision"`
	Enabled            bool   `json:"enabled"`
	Action             string `json:"action,omitempty"`
	RetentionDays      int    `json:"retention_days,omitempty"`
	ArchiveTargetID    string `json:"archive_target_uuid,omitempty"`
	TargetRevision     int64  `json:"target_revision,omitempty"`
	ActiveRequest      string `json:"active_request_uuid,omitempty"`
	State              string `json:"state"`
}

type datasetPolicyDocument struct {
	Policy            DatasetExecutionPolicy
	TargetFingerprint string
}

type DatasetRunInput struct {
	RequestUUID    string `json:"request_uuid"`
	TenantUUID     string `json:"tenant_uuid"`
	PreviewUUID    string `json:"preview_request_uuid"`
	PolicyRevision int64  `json:"expected_policy_revision"`
}

type DatasetExecutionReceipt struct {
	RequestUUID      string     `json:"request_uuid"`
	DatasetUUID      string     `json:"dataset_uuid"`
	TenantUUID       string     `json:"tenant_uuid"`
	PreviewUUID      string     `json:"preview_request_uuid"`
	PolicyRevision   int64      `json:"policy_revision"`
	Action           string     `json:"action"`
	State            string     `json:"state"`
	CutoffAt         time.Time  `json:"cutoff_at"`
	GrantExpiresAt   *time.Time `json:"grant_expires_at,omitempty"`
	AffectedCount    *int       `json:"affected_count,omitempty"`
	SourceReceipt    string     `json:"source_receipt_uuid,omitempty"`
	ArchiveAvailable bool       `json:"archive_available,omitempty"`
}

type datasetRunDocument struct {
	Run                  DatasetExecutionReceipt
	Deployment           string
	Owner                string
	TargetSubject        string
	Installation         string
	AcceptanceRevision   int64
	RequestDigest        string
	PreviewExpires       time.Time
	GrantUUID            string
	GrantIssuedAt        time.Time
	GrantSnapshot        *datasetExecutionGrant
	LeaseReceipt         string
	LeaseFence           uint64
	ReportDigest         string
	ArchiveTargetID      string
	ArchiveTargetRev     int64
	ArchiveTargetDigest  string
	ArchiveContentDigest string
	ArchiveReportDigest  string
	ArchiveObject        *archive.DatasetArchiveObject
	CancelledBy          uint64
}

func (s *Service) datasetWriteNamespace(category string) string {
	return "dataset-" + category + ":" + controlDigest([]byte(s.opts.Identity.InstanceID))
}

func (s *Service) datasetWriteReady() bool {
	return s != nil && s.opts.DB != nil && s.opts.Identity.InstanceID != ""
}

func (s *Service) withDatasetWriteAdmission(ctx context.Context, tx *gorm.DB, fn func() error) error {
	if !s.opts.DatasetExecutionEnabled || s.opts.DatasetTasks == nil || s.opts.Control == nil ||
		s.opts.Control.executionFence == nil || s.opts.Control.executionGuard == nil || s.opts.Control.ConsumerExited() {
		return controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	c := s.opts.Control
	err := c.executionGuard(ctx, func() error {
		return c.repo.WithExecutionTx(ctx, tx, c.opts.Scope.ID(), func(row *model.ControlConfiguration) error {
			if err := c.executionFence(ctx, tx, row); err != nil {
				return err
			}
			return fn()
		})
	})
	if datasetErrorCode(err, "STORAGE_EXECUTION_PAUSED") {
		return controlError(423, "DATASET_EXECUTION_PAUSED")
	}
	return err
}

func loadDatasetWriteDocument[T any](ctx context.Context, db *gorm.DB, namespace, id string, lock bool) (model.CleanupRun, T, error) {
	var row model.CleanupRun
	var doc T
	q := db.WithContext(ctx).Where("tenant_id = 0 AND instance_id = ? AND run_id = ?", namespace, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Take(&row).Error; err != nil {
		return row, doc, err
	}
	if json.Unmarshal(row.Document, &doc) != nil {
		return row, doc, ErrStateUnavailable
	}
	return row, doc, nil
}

func storeDatasetWriteDocument(ctx context.Context, tx *gorm.DB, row model.CleanupRun, state string, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	row.Document, row.State, row.Version = datatypes.JSON(raw), state, row.Version+1
	if row.ID == 0 {
		return tx.WithContext(ctx).Create(&row).Error
	}
	result := tx.WithContext(ctx).Model(&row).Updates(map[string]any{"document": row.Document, "state": row.State, "version": row.Version, "actor_id": row.ActorID})
	if result.Error == nil && result.RowsAffected != 1 {
		return ErrStateUnavailable
	}
	return result.Error
}

func (s *Service) datasetPolicyLock(ctx context.Context, tx *gorm.DB, ds, tenant string) error {
	return tx.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", s.datasetWriteNamespace("policy")+":"+ds+":"+tenant).Error
}

func (s *Service) loadDatasetPolicy(ctx context.Context, db *gorm.DB, ds, tenant string) (model.CleanupRun, datasetPolicyDocument, error) {
	namespace := s.datasetWriteNamespace("policy")
	key := uuid.NewSHA1(uuid.NameSpaceOID, []byte(ds+":"+tenant)).String()
	row, doc, err := loadDatasetWriteDocument[datasetPolicyDocument](ctx, db, namespace, key, false)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.CleanupRun{InstanceID: namespace, RunID: key, ExpiresAt: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)},
			datasetPolicyDocument{Policy: DatasetExecutionPolicy{DatasetUUID: ds, TenantUUID: tenant, State: "paused"}}, nil
	}
	if err == nil && (doc.Policy.DatasetUUID != ds || doc.Policy.TenantUUID != tenant) {
		return row, doc, ErrStateUnavailable
	}
	return row, doc, err
}

func (s *Service) DatasetPolicy(ctx context.Context, dataset, tenant string) (DatasetExecutionPolicy, error) {
	if _, err := parseDatasetUUID(dataset); err != nil {
		return DatasetExecutionPolicy{}, controlError(400, "DATASET_POLICY_INVALID")
	}
	if _, err := parseDatasetUUID(tenant); err != nil {
		return DatasetExecutionPolicy{}, controlError(400, "DATASET_POLICY_INVALID")
	}
	if !s.datasetWriteReady() {
		return DatasetExecutionPolicy{}, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	_, doc, err := s.loadDatasetPolicy(ctx, s.opts.DB, dataset, tenant)
	return doc.Policy, err
}

func (s *Service) ChangeDatasetPolicy(ctx context.Context, actor uint64, dataset, tenant string, in DatasetPolicyChange) (DatasetExecutionPolicy, error) {
	id, e := parseDatasetUUID(dataset)
	_, tenantErr := parseDatasetUUID(tenant)
	if e != nil || tenantErr != nil || actor == 0 || in.ExpectedRevision == nil || *in.ExpectedRevision < 0 || in.Enabled == nil {
		return DatasetExecutionPolicy{}, controlError(400, "DATASET_POLICY_INVALID")
	}
	if !s.datasetWriteReady() {
		return DatasetExecutionPolicy{}, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	var target archive.Target
	if *in.Enabled && in.Action == "archive" {
		if !validTargetUUID(in.ArchiveTargetID) {
			return DatasetExecutionPolicy{}, controlError(400, "DATASET_ARCHIVE_TARGET_INVALID")
		}
		var err error
		target, err = s.datasetArchiveTarget(ctx, in.ArchiveTargetID)
		if err != nil {
			return DatasetExecutionPolicy{}, err
		}
	}
	var out DatasetExecutionPolicy
	err := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		change := func() error {
			if err := s.datasetPolicyLock(ctx, tx, dataset, tenant); err != nil {
				return err
			}
			row, doc, err := s.loadDatasetPolicy(ctx, tx, dataset, tenant)
			if err != nil {
				return err
			}
			p := &doc.Policy
			if p.Revision != *in.ExpectedRevision {
				return controlError(409, "DATASET_POLICY_REVISION_CONFLICT")
			}
			if *in.Enabled {
				if p.ActiveRequest != "" {
					return controlError(409, "DATASET_EXECUTION_RECONCILIATION_REQUIRED")
				}
				head, source, err := s.currentDatasetPreviewSource(ctx, tx, id, in.AcceptanceRevision, true)
				if err != nil {
					return err
				}
				if err := allowedDatasetWrite(source.declaration, in.Action, in.RetentionDays, in.ArchiveTargetID); err != nil {
					return err
				}
				if _, err := s.datasetWorkerTarget(ctx, tx, tenant, head.OwnerID, DatasetExecutionCapability); err != nil {
					return err
				}
				p.AcceptanceRevision, p.Action, p.RetentionDays = in.AcceptanceRevision, in.Action, in.RetentionDays
				p.ArchiveTargetID, p.TargetRevision, doc.TargetFingerprint = "", 0, ""
				if in.Action == "archive" {
					p.ArchiveTargetID, p.TargetRevision, doc.TargetFingerprint = target.ID, target.Revision, target.VerifiedFingerprint
				}
			}
			if !datasetRevisionOK(p.Revision + 1) {
				return ErrStateUnavailable
			}
			p.Revision++
			p.Enabled = *in.Enabled
			p.State = "paused"
			if p.Enabled {
				p.State = "enabled"
			} else if p.ActiveRequest != "" {
				p.State = "draining"
			}
			row.ActorID = actor
			out = *p
			return storeDatasetWriteDocument(ctx, tx, row, p.State, doc)
		}
		if *in.Enabled {
			return s.withDatasetWriteAdmission(ctx, tx, change)
		}
		return change()
	})
	if err == nil {
		s.PublishChange("dataset_policy_changed")
	}
	return out, err
}

func allowedDatasetWrite(decl DatasetDeclaration, action string, days int, target string) error {
	if (action != "delete" && action != "archive") || !slices.Contains(decl.AllowedActions, action) ||
		action == "delete" && (decl.ArchiveBeforeDelete || target != "") {
		return controlError(422, "DATASET_ACTION_NOT_ALLOWED")
	}
	bounds := decl.RetentionBounds
	if bounds == nil || days < bounds.MinimumDays || days > bounds.MaximumDays {
		return controlError(422, "DATASET_RETENTION_OUT_OF_BOUNDS")
	}
	if action == "archive" && !validTargetUUID(target) {
		return controlError(400, "DATASET_ARCHIVE_TARGET_INVALID")
	}
	return nil
}

func (s *Service) loadDatasetRun(ctx context.Context, tx *gorm.DB, id string, lock bool) (model.CleanupRun, datasetRunDocument, error) {
	row, doc, err := loadDatasetWriteDocument[datasetRunDocument](ctx, tx, s.datasetWriteNamespace("execution"), id, lock)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return row, doc, controlError(404, "DATASET_EXECUTION_NOT_FOUND")
	}
	if err == nil && (doc.Deployment != s.opts.Identity.InstanceID || doc.Run.RequestUUID != id) {
		return row, doc, ErrStateUnavailable
	}
	return row, doc, err
}

func (s *Service) StartDatasetExecution(ctx context.Context, actor uint64, dataset string, in DatasetRunInput) (DatasetExecutionReceipt, bool, error) {
	id, e := parseDatasetUUID(dataset)
	_, runErr := parseDatasetUUID(in.RequestUUID)
	_, previewErr := parseDatasetUUID(in.PreviewUUID)
	_, tenantErr := parseDatasetUUID(in.TenantUUID)
	if e != nil || runErr != nil || previewErr != nil || tenantErr != nil || !datasetRevisionOK(in.PolicyRevision) || actor == 0 {
		return DatasetExecutionReceipt{}, false, controlError(400, "DATASET_EXECUTION_INVALID")
	}
	if !s.datasetWriteReady() {
		return DatasetExecutionReceipt{}, false, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	raw, _ := json.Marshal(in)
	digest := controlDigest(raw)
	var out DatasetExecutionReceipt
	var msg event_bus.TaskMessage
	err := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ns := s.datasetWriteNamespace("execution")
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", ns+":"+in.RequestUUID).Error; err != nil {
			return err
		}
		row, old, err := s.loadDatasetRun(ctx, tx, in.RequestUUID, false)
		if err == nil {
			if row.ActorID != actor || old.Run.DatasetUUID != dataset || old.RequestDigest != digest {
				return controlError(409, "DATASET_REQUEST_CONFLICT")
			}
			out = s.projectDatasetRun(old)
			return nil
		}
		if row.ID != 0 || !datasetErrorCode(err, "DATASET_EXECUTION_NOT_FOUND") {
			return err
		}
		return s.withDatasetWriteAdmission(ctx, tx, func() error {
			if err := s.datasetPolicyLock(ctx, tx, dataset, in.TenantUUID); err != nil {
				return err
			}
			policyRow, doc, err := s.loadDatasetPolicy(ctx, tx, dataset, in.TenantUUID)
			if err != nil {
				return err
			}
			p := doc.Policy
			if !p.Enabled || p.Revision != in.PolicyRevision || p.ActiveRequest != "" {
				return controlError(409, "DATASET_POLICY_UNAVAILABLE")
			}
			previewRow, preview, err := s.loadDatasetPreview(ctx, tx, in.PreviewUUID, true)
			now := s.opts.Now().UTC()
			if err != nil || previewRow.ActorID != actor || preview.Preview.DatasetUUID != id ||
				preview.Preview.TenantUUID != in.TenantUUID || preview.Preview.AcceptanceRevision != p.AcceptanceRevision ||
				preview.Preview.RetentionDays != p.RetentionDays || preview.Preview.State != "completed" ||
				preview.Preview.Result == nil || preview.Preview.Result.CandidateCount < 1 ||
				!now.Before(preview.Preview.ExpiresAt) || preview.ExecutionRequestUUID != "" {
				return controlError(409, "DATASET_EXECUTION_PREVIEW_REQUIRED")
			}
			head, source, err := s.currentDatasetPreviewSource(ctx, tx, id, p.AcceptanceRevision, true)
			if err != nil {
				return err
			}
			if err := allowedDatasetWrite(source.declaration, p.Action, p.RetentionDays, p.ArchiveTargetID); err != nil {
				return err
			}
			subject, err := s.datasetWorkerTarget(ctx, tx, in.TenantUUID, head.OwnerID, DatasetExecutionCapability)
			if err != nil {
				return err
			}
			out = DatasetExecutionReceipt{RequestUUID: in.RequestUUID, DatasetUUID: dataset, TenantUUID: in.TenantUUID,
				PreviewUUID: in.PreviewUUID, PolicyRevision: in.PolicyRevision, Action: p.Action, State: "queued", CutoffAt: preview.Preview.CutoffAt}
			run := datasetRunDocument{Run: out, Deployment: s.opts.Identity.InstanceID, Owner: head.OwnerID, Installation: head.Installation,
				TargetSubject: subject, AcceptanceRevision: p.AcceptanceRevision, RequestDigest: digest, PreviewExpires: preview.Preview.ExpiresAt,
				ArchiveTargetID: p.ArchiveTargetID, ArchiveTargetRev: p.TargetRevision, ArchiveTargetDigest: doc.TargetFingerprint}
			if err := storeDatasetWriteDocument(ctx, tx, model.CleanupRun{InstanceID: ns, RunID: in.RequestUUID, ActorID: actor,
				ExpiresAt: now.AddDate(0, 0, 30)}, out.State, run); err != nil {
				return err
			}
			preview.ExecutionRequestUUID = in.RequestUUID
			if err := saveDatasetPreview(tx, previewRow, preview); err != nil {
				return err
			}
			doc.Policy.ActiveRequest = in.RequestUUID
			if err := storeDatasetWriteDocument(ctx, tx, policyRow, doc.Policy.State, doc); err != nil {
				return err
			}
			notice, _ := json.Marshal(map[string]string{"contract_version": datasetExecutionContract, "request_uuid": in.RequestUUID})
			msg, err = s.opts.DatasetTasks.RegisterDatasetExecution(ctx, tx, s.executionRuntimeScope(run), in.RequestUUID, notice)
			return err
		})
	})
	if err != nil {
		return DatasetExecutionReceipt{}, false, err
	}
	if msg.ID != "" {
		run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = s.opts.DatasetTasks.DeliverDatasetTask(run, msg)
		s.PublishChange("dataset_execution_changed")
	}
	return out, msg.ID != "", nil
}

func (s *Service) DatasetExecution(ctx context.Context, actor uint64, dataset, request string) (DatasetExecutionReceipt, error) {
	if _, err := parseDatasetUUID(dataset); err != nil || actor == 0 {
		return DatasetExecutionReceipt{}, controlError(400, "DATASET_EXECUTION_INVALID")
	}
	if _, err := parseDatasetUUID(request); err != nil {
		return DatasetExecutionReceipt{}, controlError(400, "DATASET_EXECUTION_INVALID")
	}
	if !s.datasetWriteReady() {
		return DatasetExecutionReceipt{}, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	_, doc, err := s.loadDatasetRun(ctx, s.opts.DB, request, false)
	if err != nil {
		return DatasetExecutionReceipt{}, err
	}
	if doc.Run.DatasetUUID != dataset {
		return DatasetExecutionReceipt{}, controlError(404, "DATASET_EXECUTION_NOT_FOUND")
	}
	return s.projectDatasetRun(doc), nil
}

func (s *Service) projectDatasetRun(doc datasetRunDocument) DatasetExecutionReceipt {
	r := doc.Run
	now := s.opts.Now()
	if r.State == "authorized" && r.GrantExpiresAt != nil && !now.Before(*r.GrantExpiresAt) {
		r.State = "needs_reconciliation"
	}
	return r
}

func (s *Service) executionRuntimeScope(doc datasetRunDocument) taskqueue.RuntimeScope {
	return taskqueue.RuntimeScope{TenantUUID: doc.Run.TenantUUID, SourceService: doc.TargetSubject, EnvironmentID: doc.Deployment}
}

// CancelDatasetExecution releases an intent only when no source write grant
// could have been issued. An authorized batch must be reconciled at its source.
func (s *Service) CancelDatasetExecution(ctx context.Context, actor uint64, dataset, request string) (DatasetExecutionReceipt, error) {
	out, err := s.DatasetExecution(ctx, actor, dataset, request)
	if err != nil {
		return out, err
	}
	if s.opts.DatasetTasks == nil {
		return out, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	_, doc, err := s.loadDatasetRun(ctx, s.opts.DB, request, false)
	if err != nil {
		return out, err
	}
	err = s.opts.DatasetTasks.SettleDatasetExecution(ctx, s.executionRuntimeScope(doc), request, func(tx *gorm.DB) error {
		if err := s.datasetPolicyLock(ctx, tx, dataset, doc.Run.TenantUUID); err != nil {
			return err
		}
		row, run, err := s.loadDatasetRun(ctx, tx, request, true)
		if err != nil {
			return err
		}
		if run.Run.State == "cancelled" {
			out = run.Run
			return nil
		}
		if run.Run.State != "queued" || run.GrantUUID != "" {
			return controlError(409, "DATASET_EXECUTION_RECONCILIATION_REQUIRED")
		}
		policyRow, policy, err := s.loadDatasetPolicy(ctx, tx, dataset, run.Run.TenantUUID)
		if err != nil {
			return err
		}
		if policy.Policy.ActiveRequest != request {
			return controlError(409, "DATASET_EXECUTION_STALE")
		}
		policy.Policy.ActiveRequest = ""
		if !policy.Policy.Enabled {
			policy.Policy.State = "paused"
		}
		run.Run.State = "cancelled"
		out = run.Run
		run.CancelledBy = actor
		if err := storeDatasetWriteDocument(ctx, tx, row, out.State, run); err != nil {
			return err
		}
		return storeDatasetWriteDocument(ctx, tx, policyRow, policy.Policy.State, policy)
	})
	if err == nil {
		s.PublishChange("dataset_execution_changed")
	}
	return out, err
}
