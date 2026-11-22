package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type datasetExecutionReport struct {
	RequestUUID    string    `json:"request_uuid"`
	GrantUUID      string    `json:"grant_uuid"`
	ReceiptUUID    string    `json:"source_receipt_uuid"`
	ReceiptDigest  string    `json:"source_receipt_digest"`
	Outcome        string    `json:"outcome"`
	AffectedCount  *int      `json:"affected_count"`
	SourceSealedAt time.Time `json:"source_sealed_at"`
}

func (r datasetExecutionReport) valid() bool {
	for _, value := range []string{r.RequestUUID, r.GrantUUID, r.ReceiptUUID} {
		if _, err := parseDatasetUUID(value); err != nil {
			return false
		}
	}
	if !datasetDigestPattern.MatchString(r.ReceiptDigest) || r.SourceSealedAt.IsZero() || r.AffectedCount == nil ||
		*r.AffectedCount < 0 || *r.AffectedCount > datasetWriteBatchLimit {
		return false
	}
	return r.Outcome == "committed" || r.Outcome == "aborted" && *r.AffectedCount == 0
}

type datasetExecutionGrant struct {
	datasetPreviewGrant
	GrantUUID      string    `json:"grant_uuid"`
	Action         string    `json:"action"`
	PolicyRevision int64     `json:"policy_revision"`
	IssuedAt       time.Time `json:"issued_at"`
	MaxAffected    int       `json:"max_affected"`
}

func datasetErrorCode(err error, code string) bool { return dto.CodeOf(err) == code }

func (i *DatasetRegisterInvoker) invokeExecution(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	bad := controlError(400, "DATASET_EXECUTION_OPERATION_INVALID")
	if in.Method != "INVOKE" || in.Endpoint != datasetEndpoint || len(in.Query) != 0 || len(in.Context) != 0 || len(in.Payload) != 1 {
		return nil, bad
	}
	if _, ok := in.Payload["body"]; !ok {
		return nil, bad
	}
	raw, err := json.Marshal(in.Body)
	var op struct {
		ContractVersion string          `json:"contract_version"`
		Operation       string          `json:"operation"`
		Input           json.RawMessage `json:"input"`
	}
	if err != nil || len(raw) > datasetExecutionMaxRequestBytes || !decodeDatasetHost(raw, &op) || op.ContractVersion != datasetExecutionContract {
		return nil, bad
	}
	claims := reqctx.GetClaims(ctx)
	tid := reqctx.GetTenantUUID(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.PluginID == "" {
		return nil, controlError(403, "DATASET_STS_REQUIRED")
	}
	if _, err := parseDatasetUUID(tid); err != nil || tid != claims.TenantUUID || tid != in.TenantUUID {
		return nil, controlError(403, "DATASET_TENANT_FORBIDDEN")
	}
	if i.Service == nil {
		return nil, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	s := i.Service()
	if !s.datasetWriteReady() || s.opts.DatasetTasks == nil {
		return nil, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
	}
	access, err := cap.NewGrantStatusService(s.opts.DB).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(in.CapabilityID)
	}
	if err != nil || !access.OwnsPlugin(claims.PluginID) {
		var denied *cap.DirectGrantError
		if err != nil && (!errors.As(err, &denied) || denied.Status >= 500) {
			return nil, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
		}
		return nil, controlError(403, "DATASET_EXECUTION_WORKER_FORBIDDEN")
	}

	work, err := s.applyDatasetExecutionOperation(ctx, op.Operation, op.Input, tid, claims.PluginID, access.Subject)
	if err != nil {
		if dto.CodeOf(err) == "" {
			return nil, controlError(503, "DATASET_EXECUTION_UNAVAILABLE")
		}
		return nil, err
	}
	return work, nil
}

func (s *Service) applyDatasetExecutionOperation(ctx context.Context, op string, raw json.RawMessage, tenant, plugin, subject string) (map[string]any, error) {
	bad := controlError(400, "DATASET_EXECUTION_OPERATION_INVALID")
	switch op {
	case "claim_execution":
		var ref datasetPreviewLeaseRef
		if !decodeDatasetHost(raw, &ref) || !ref.valid() {
			return nil, bad
		}
		doc, err := s.ownedDatasetRun(ctx, ref.RequestUUID, tenant, plugin, subject)
		if err != nil {
			return nil, err
		}
		return s.claimDatasetExecution(ctx, doc, ref)
	case "reconcile_execution":
		var ref struct {
			RequestUUID string `json:"request_uuid"`
		}
		if !decodeDatasetHost(raw, &ref) {
			return nil, bad
		}
		if _, err := parseDatasetUUID(ref.RequestUUID); err != nil {
			return nil, bad
		}
		doc, err := s.ownedDatasetRun(ctx, ref.RequestUUID, tenant, plugin, subject)
		if err != nil {
			return nil, err
		}
		if doc.GrantUUID == "" || doc.Run.GrantExpiresAt == nil {
			return nil, controlError(409, "DATASET_EXECUTION_GRANT_STALE")
		}
		now := s.opts.Now()
		if now.Before(*doc.Run.GrantExpiresAt) {
			return nil, controlError(409, "DATASET_EXECUTION_GRANT_ACTIVE")
		}
		return doc.reconciliationResult(now), nil
	case "report_execution":
		var report datasetExecutionReport
		if !decodeDatasetHost(raw, &report) || !report.valid() {
			return nil, bad
		}
		doc, err := s.ownedDatasetRun(ctx, report.RequestUUID, tenant, plugin, subject)
		if err != nil {
			return nil, err
		}
		return s.settleDatasetExecution(ctx, doc, report)
	case "archive_execution":
		return s.archiveDatasetExecution(ctx, raw, tenant, plugin, subject)
	default:
		return nil, bad
	}
}

func (s *Service) ownedDatasetRun(ctx context.Context, id, tenant, plugin, subject string) (datasetRunDocument, error) {
	_, doc, err := s.loadDatasetRun(ctx, s.opts.DB, id, false)
	if err != nil {
		return doc, err
	}
	if doc.Run.TenantUUID != tenant || doc.Owner != plugin || doc.TargetSubject != subject || subject == "" {
		return doc, controlError(403, "DATASET_EXECUTION_WORKER_FORBIDDEN")
	}
	return doc, nil
}

func (doc datasetRunDocument) originalGrant() *datasetExecutionGrant {
	grant := doc.GrantSnapshot
	if grant == nil || doc.Run.GrantExpiresAt == nil ||
		grant.GrantUUID != doc.GrantUUID || grant.RequestUUID != doc.Run.RequestUUID ||
		grant.DatasetUUID.String() != doc.Run.DatasetUUID || grant.TenantUUID != doc.Run.TenantUUID ||
		grant.Action != doc.Run.Action || grant.PolicyRevision != doc.Run.PolicyRevision ||
		grant.AcceptanceRevision != doc.AcceptanceRevision || !grant.CutoffAt.Equal(doc.Run.CutoffAt) ||
		!grant.DeadlineAt.Equal(*doc.Run.GrantExpiresAt) || !grant.IssuedAt.Equal(doc.GrantIssuedAt) {
		return nil
	}
	return grant
}

func (doc datasetRunDocument) reconciliationResult(now time.Time) map[string]any {
	result := map[string]any{"state": "reconcile_only", "request_uuid": doc.Run.RequestUUID, "grant_uuid": doc.GrantUUID}
	if grant := doc.originalGrant(); grant != nil && !now.Before(grant.DeadlineAt) {
		result["expired_grant"] = grant
	}
	return result
}

func (s *Service) claimDatasetExecution(ctx context.Context, doc datasetRunDocument, ref datasetPreviewLeaseRef) (map[string]any, error) {
	var result map[string]any
	issued := false
	err := s.opts.DatasetTasks.UseDatasetExecutionLease(ctx, s.executionRuntimeScope(doc), ref.RequestUUID, ref.ReceiptUUID, ref.Fence, func(tx *gorm.DB, leaseUntil time.Time) error {
		// The locked TaskBus record serializes authorization, cancellation and
		// settlement. Read the durable grant before consulting write admission.
		_, current, err := s.loadDatasetRun(ctx, tx, ref.RequestUUID, false)
		if err != nil {
			return err
		}
		if current.GrantUUID != "" && (current.originalGrant() == nil || current.Run.State != "authorized" ||
			!s.opts.Now().Before(*current.Run.GrantExpiresAt) || current.LeaseReceipt != ref.ReceiptUUID || current.LeaseFence != ref.Fence) {
			result = current.reconciliationResult(s.opts.Now())
			return nil
		}
		err = s.withDatasetWriteAdmission(ctx, tx, func() error {
			if err := s.datasetPolicyLock(ctx, tx, current.Run.DatasetUUID, current.Run.TenantUUID); err != nil {
				return err
			}
			row, current, err := s.loadDatasetRun(ctx, tx, ref.RequestUUID, true)
			if err != nil {
				return err
			}
			_, policy, err := s.loadDatasetPolicy(ctx, tx, current.Run.DatasetUUID, current.Run.TenantUUID)
			if err != nil {
				return err
			}
			now := s.opts.Now().UTC()
			active := policy.Policy.Enabled && policy.Policy.Revision == current.Run.PolicyRevision && policy.Policy.ActiveRequest == ref.RequestUUID
			if current.GrantUUID != "" && (!active || !now.Before(*current.Run.GrantExpiresAt)) {
				result = current.reconciliationResult(now)
				return nil
			}
			if !active || !now.Before(current.PreviewExpires) || current.Run.State != "queued" && current.Run.State != "authorized" {
				return controlError(409, "DATASET_EXECUTION_STALE")
			}
			grant, err := s.authorizeDatasetGrant(ctx, tx, row, current, ref, now, leaseUntil)
			if err != nil {
				return err
			}
			issued = current.GrantUUID == ""
			result = map[string]any{"state": "authorized", "grant": grant}
			return nil
		})
		switch dto.CodeOf(err) {
		case "DATASET_EXECUTION_PAUSED", "DATASET_EXECUTION_UNAVAILABLE", "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE":
			if current.GrantUUID != "" {
				result = current.reconciliationResult(s.opts.Now())
				return nil
			}
		}
		return err
	})
	if errors.Is(err, event_bus.ErrLeaseStale) || errors.Is(err, taskqueue.ErrTaskTerminal) {
		return nil, controlError(409, "DATASET_EXECUTION_LEASE_STALE")
	}
	if err == nil && issued {
		s.PublishChange("dataset_execution_changed")
	}
	return result, err
}

func (s *Service) authorizeDatasetGrant(ctx context.Context, tx *gorm.DB, row model.CleanupRun, current datasetRunDocument, ref datasetPreviewLeaseRef, now, leaseUntil time.Time) (*datasetExecutionGrant, error) {
	stale := controlError(409, "DATASET_EXECUTION_STALE")
	dataset, _ := parseDatasetUUID(current.Run.DatasetUUID)
	head, source, err := s.currentDatasetPreviewSource(ctx, tx, dataset, current.AcceptanceRevision, true)
	if err != nil {
		return nil, err
	}
	if head.Installation != current.Installation || head.OwnerID != current.Owner {
		return nil, stale
	}
	subject, err := s.datasetWorkerTarget(ctx, tx, current.Run.TenantUUID, current.Owner, DatasetExecutionCapability)
	if err != nil {
		return nil, err
	}
	if subject != current.TargetSubject {
		return nil, stale
	}
	if current.GrantUUID != "" {
		grant := current.originalGrant()
		if grant == nil || grant.SourceBindingUUID != head.BindingUUID || grant.SourceFingerprint != head.SourceFingerprint ||
			grant.DefinitionDigest != source.spec.SHA256 || !s.opts.Now().Before(grant.DeadlineAt) {
			return nil, stale
		}
		return grant, nil
	}

	deadline := now.Add(datasetWriteAuthorization)
	if deadline.After(leaseUntil) {
		deadline = leaseUntil
	}
	if deadline.After(current.PreviewExpires) {
		deadline = current.PreviewExpires
	}
	if !s.opts.Now().Before(deadline) {
		return nil, stale
	}
	grant := &datasetExecutionGrant{
		GrantUUID: uuid.NewString(), Action: current.Run.Action, PolicyRevision: current.Run.PolicyRevision,
		IssuedAt: now, MaxAffected: datasetWriteBatchLimit,
		datasetPreviewGrant: datasetPreviewGrant{RequestUUID: ref.RequestUUID, DatasetUUID: dataset,
			TenantUUID: current.Run.TenantUUID, SourceBindingUUID: head.BindingUUID, SourceFingerprint: head.SourceFingerprint,
			AcceptanceRevision: head.AcceptanceRevision, DefinitionDigest: source.spec.SHA256,
			SchemaContractVersion: source.declaration.SchemaContractVersion, AdapterKey: source.declaration.AdapterKey,
			AdapterVersion: source.declaration.AdapterVersion, CutoffAt: current.Run.CutoffAt,
			DeadlineAt: deadline, MaxExamined: datasetMaxExamined},
	}
	current.GrantUUID, current.GrantIssuedAt, current.GrantSnapshot = grant.GrantUUID, grant.IssuedAt, grant
	current.Run.GrantExpiresAt = &deadline
	current.LeaseReceipt, current.LeaseFence = ref.ReceiptUUID, ref.Fence
	current.Run.State = "authorized"
	if err := storeDatasetWriteDocument(ctx, tx, row, current.Run.State, current); err != nil {
		return nil, err
	}
	return grant, nil
}

func (s *Service) settleDatasetExecution(ctx context.Context, doc datasetRunDocument, report datasetExecutionReport) (map[string]any, error) {
	if doc.Run.Action != "delete" && !(doc.Run.Action == "archive" && report.Outcome == "aborted") {
		return nil, controlError(422, "DATASET_EXECUTION_ACTION_MISMATCH")
	}
	return s.recordDatasetResult(ctx, doc, report, nil)
}

func (s *Service) recordDatasetResult(ctx context.Context, doc datasetRunDocument, report datasetExecutionReport, object *archive.DatasetArchiveObject) (map[string]any, error) {
	raw, _ := json.Marshal(report)
	digest := controlDigest(raw)
	var output DatasetExecutionReceipt
	changed := false
	err := s.opts.DatasetTasks.SettleDatasetExecution(ctx, s.executionRuntimeScope(doc), report.RequestUUID, func(tx *gorm.DB) error {
		if err := s.datasetPolicyLock(ctx, tx, doc.Run.DatasetUUID, doc.Run.TenantUUID); err != nil {
			return err
		}
		row, current, err := s.loadDatasetRun(ctx, tx, report.RequestUUID, true)
		if err != nil {
			return err
		}
		if err := s.checkDatasetResultGrant(current, report); err != nil {
			return err
		}
		if current.ReportDigest != "" {
			if current.ReportDigest != digest || current.Run.ArchiveAvailable != (object != nil) ||
				object != nil && (current.ArchiveObject == nil || *current.ArchiveObject != *object) {
				return controlError(409, "DATASET_EXECUTION_REPORT_CONFLICT")
			}
			output = current.Run
			return nil
		}
		if current.ArchiveContentDigest != "" && (object == nil || object.SHA256 != current.ArchiveContentDigest ||
			digest != current.ArchiveReportDigest) {
			return controlError(409, "DATASET_EXECUTION_REPORT_CONFLICT")
		}
		policyRow, policy, err := s.loadDatasetPolicy(ctx, tx, doc.Run.DatasetUUID, doc.Run.TenantUUID)
		if err != nil {
			return err
		}
		if policy.Policy.ActiveRequest != report.RequestUUID {
			return controlError(409, "DATASET_EXECUTION_STALE")
		}
		current.ReportDigest, current.Run.SourceReceipt, current.Run.AffectedCount = digest, report.ReceiptUUID, report.AffectedCount
		current.ArchiveObject, current.Run.ArchiveAvailable = object, object != nil
		current.Run.State = report.Outcome
		if err := storeDatasetWriteDocument(ctx, tx, row, current.Run.State, current); err != nil {
			return err
		}
		policy.Policy.ActiveRequest = ""
		if !policy.Policy.Enabled {
			policy.Policy.State = "paused"
		}
		if err := storeDatasetWriteDocument(ctx, tx, policyRow, policy.Policy.State, policy); err != nil {
			return err
		}
		output, changed = current.Run, true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.PublishChange("dataset_execution_changed")
	}
	return map[string]any{"request_uuid": output.RequestUUID, "state": output.State}, nil
}

func (s *Service) checkDatasetResultGrant(current datasetRunDocument, report datasetExecutionReport) error {
	if current.GrantUUID == "" || report.GrantUUID != current.GrantUUID || current.Run.GrantExpiresAt == nil ||
		report.SourceSealedAt.Before(current.GrantIssuedAt) || report.SourceSealedAt.After(s.opts.Now()) ||
		report.Outcome == "committed" && !report.SourceSealedAt.Before(*current.Run.GrantExpiresAt) {
		return controlError(409, "DATASET_EXECUTION_GRANT_STALE")
	}
	return nil
}
