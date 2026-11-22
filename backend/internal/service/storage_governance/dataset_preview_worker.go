package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

type datasetPreviewLeaseRef struct {
	RequestUUID string `json:"request_uuid"`
	ReceiptUUID string `json:"receipt_uuid"`
	Fence       uint64 `json:"fence"`
}

type datasetPreviewReport struct {
	datasetPreviewLeaseRef
	AcceptanceRevision int64                 `json:"acceptance_revision"`
	Outcome            string                `json:"outcome"`
	Result             *DatasetPreviewCounts `json:"result,omitempty"`
	StatusReason       string                `json:"status_reason,omitempty"`
}

func decodeDatasetHost(raw []byte, dst any) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst) == nil && dec.Decode(&struct{}{}) == io.EOF
}

func (r datasetPreviewLeaseRef) valid() bool {
	_, e := parseDatasetUUID(r.RequestUUID)
	_, leaseErr := parseDatasetUUID(r.ReceiptUUID)
	return e == nil && leaseErr == nil && r.Fence > 0 && r.Fence <= 1<<53-1
}

func (p datasetPreviewReport) valid() bool {
	if !p.datasetPreviewLeaseRef.valid() || !datasetRevisionOK(p.AcceptanceRevision) {
		return false
	}
	switch p.Outcome {
	case "completed":
		return p.Result != nil && p.StatusReason == "" && p.Result.Validate(datasetMaxExamined) == nil
	case "failed":
		return p.Result == nil && p.StatusReason != "" && datasetFilterValue(p.StatusReason,
			"source_unavailable", "schema_mismatch", "adapter_unavailable", "scan_timeout", "protection_unverifiable")
	}
	return false
}

func (i *DatasetRegisterInvoker) invokePreview(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	bad := controlError(400, "DATASET_PREVIEW_OPERATION_INVALID")
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
	if err != nil || len(raw) > 4096 || !decodeDatasetHost(raw, &op) || op.ContractVersion != datasetHostContract {
		return nil, bad
	}
	var ref datasetPreviewLeaseRef
	var report *datasetPreviewReport
	switch op.Operation {
	case "claim_preview":
		if !decodeDatasetHost(op.Input, &ref) || !ref.valid() {
			return nil, bad
		}
	case "report_preview":
		report = &datasetPreviewReport{}
		if !decodeDatasetHost(op.Input, report) || !report.valid() {
			return nil, bad
		}
		ref = report.datasetPreviewLeaseRef
	default:
		return nil, bad
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.PluginID == "" {
		return nil, controlError(403, "DATASET_STS_REQUIRED")
	}
	tid := reqctx.GetTenantUUID(ctx)
	if _, err := parseDatasetUUID(tid); err != nil || tid != claims.TenantUUID || tid != in.TenantUUID {
		return nil, controlError(403, "DATASET_TENANT_FORBIDDEN")
	}
	if i.Service == nil {
		return nil, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
	}
	s := i.Service()
	if s == nil || s.opts.DB == nil || s.opts.DatasetTasks == nil {
		return nil, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
	}
	access, err := cap.NewGrantStatusService(s.opts.DB).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(in.CapabilityID)
	}
	if err != nil || !access.OwnsPlugin(claims.PluginID) {
		var denied *cap.DirectGrantError
		if err != nil && (!errors.As(err, &denied) || denied.Status >= 500) {
			return nil, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
		}
		return nil, controlError(403, "DATASET_PREVIEW_WORKER_FORBIDDEN")
	}
	out, err := s.previewWorker(ctx, claims.PluginID, tid, access.Subject, ref, report)
	if err != nil {
		if dto.CodeOf(err) == "" {
			return nil, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
		}
		return nil, err
	}
	return out, nil
}

func (s *Service) previewWorker(ctx context.Context, plugin, tenant, subject string, ref datasetPreviewLeaseRef, report *datasetPreviewReport) (map[string]any, error) {
	_, doc, err := s.loadDatasetPreview(ctx, s.opts.DB, ref.RequestUUID, false)
	if dto.CodeOf(err) == "DATASET_PREVIEW_NOT_FOUND" {
		return nil, controlError(409, "DATASET_PREVIEW_LEASE_STALE")
	}
	if err != nil {
		return nil, err
	}
	if doc.Owner != plugin || doc.Preview.TenantUUID != tenant || doc.TargetSubject != subject {
		return nil, controlError(403, "DATASET_PREVIEW_WORKER_FORBIDDEN")
	}
	if state, done, err := replayDatasetPreviewReport(doc, ref, report); done {
		return state, err
	}
	var out map[string]any
	var update bool
	err = s.opts.DatasetTasks.UseDatasetPreviewLease(ctx, s.previewRuntimeScope(doc), ref.RequestUUID, ref.ReceiptUUID, ref.Fence, report != nil, func(tx *gorm.DB, expires time.Time) error {
		row, current, err := s.loadDatasetPreview(ctx, tx, ref.RequestUUID, true)
		if err != nil {
			return err
		}
		now := s.opts.Now()
		if !now.Before(current.Preview.ExpiresAt) {
			return controlError(409, "DATASET_PREVIEW_EXPIRED")
		}
		head, source, err := s.currentDatasetPreviewSource(ctx, tx, current.Preview.DatasetUUID, current.Preview.AcceptanceRevision, true)
		if err != nil {
			return err
		}
		target, err := s.datasetPreviewTarget(ctx, tx, tenant, plugin)
		if err != nil {
			return err
		}
		if head.Installation != current.Installation || target != current.TargetSubject {
			return controlError(409, "DATASET_PREVIEW_INVALIDATED")
		}
		if report != nil {
			if current.ClaimReceipt != ref.ReceiptUUID || current.ClaimFence != ref.Fence || !now.Before(current.ClaimDeadline) ||
				report.AcceptanceRevision != current.Preview.AcceptanceRevision {
				return controlError(409, "DATASET_PREVIEW_LEASE_STALE")
			}
			if report.Result != nil && report.Result.Validate(current.MaxExamined) != nil {
				return controlError(400, "DATASET_PREVIEW_RESULT_INVALID")
			}
			current.ReportDigest = datasetReportDigest(report)
			current.Preview.State, current.Preview.StatusReason, current.Preview.Result = report.Outcome, report.StatusReason, report.Result
			out = datasetWorkerResult(current)
			update = true
			return saveDatasetPreview(tx, row, current)
		}
		if current.ClaimReceipt != ref.ReceiptUUID || current.ClaimFence != ref.Fence {
			current.ClaimReceipt, current.ClaimFence = ref.ReceiptUUID, ref.Fence
			current.ClaimDeadline = current.Preview.ExpiresAt
			current.MaxExamined = datasetMaxExamined
			if expires.Before(current.ClaimDeadline) {
				current.ClaimDeadline = expires
			}
			current.Preview.State = "running"
			update = true
			if err := saveDatasetPreview(tx, row, current); err != nil {
				return err
			}
		}
		grant := datasetPreviewGrant{
			RequestUUID: ref.RequestUUID, DatasetUUID: head.DatasetUUID, TenantUUID: tenant,
			SourceBindingUUID: head.BindingUUID, SourceFingerprint: head.SourceFingerprint,
			AcceptanceRevision: head.AcceptanceRevision, DefinitionDigest: source.spec.SHA256,
			SchemaContractVersion: source.declaration.SchemaContractVersion, AdapterKey: source.declaration.AdapterKey,
			AdapterVersion: source.declaration.AdapterVersion, CutoffAt: current.Preview.CutoffAt,
			DeadlineAt: current.ClaimDeadline, MaxExamined: current.MaxExamined,
		}
		raw, err := json.Marshal(grant)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &out)
	})
	if errors.Is(err, event_bus.ErrLeaseStale) || errors.Is(err, taskqueue.ErrTaskTerminal) {
		// Another report may have committed between the first read and the
		// locked delivery. Check for exact response-loss reconciliation.
		_, latest, readErr := s.loadDatasetPreview(ctx, s.opts.DB, ref.RequestUUID, false)
		if readErr == nil {
			if result, done, replayErr := replayDatasetPreviewReport(latest, ref, report); done {
				return result, replayErr
			}
		}
	}
	if errors.Is(err, event_bus.ErrLeaseStale) || errors.Is(err, taskqueue.ErrTaskTerminal) {
		return nil, controlError(409, "DATASET_PREVIEW_LEASE_STALE")
	}
	if err != nil {
		return nil, err
	}
	if update {
		s.PublishChange("dataset_preview_changed")
	}
	return out, nil
}

func datasetReportDigest(report *datasetPreviewReport) string {
	raw, _ := json.Marshal(report)
	return controlDigest(raw)
}

func datasetWorkerResult(doc datasetPreviewDocument) map[string]any {
	return map[string]any{"request_uuid": doc.Preview.RequestUUID, "state": doc.Preview.State}
}

func replayDatasetPreviewReport(doc datasetPreviewDocument, ref datasetPreviewLeaseRef, report *datasetPreviewReport) (map[string]any, bool, error) {
	if doc.ReportDigest == "" {
		return nil, false, nil
	}
	if report == nil || doc.ClaimReceipt != ref.ReceiptUUID || doc.ClaimFence != ref.Fence || doc.ReportDigest != datasetReportDigest(report) {
		return nil, true, controlError(409, "DATASET_PREVIEW_REPORT_CONFLICT")
	}
	return datasetWorkerResult(doc), true, nil
}
