package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DatasetPreviewCapability = "com.corex.storage.datasets.preview_worker"
const datasetHostContract = "powerx.storage-dataset-host/v1"
const datasetPreviewLifetime = 5 * time.Minute

type DatasetPreviewInput struct {
	RequestUUID        string `json:"request_uuid"`
	TenantUUID         string `json:"tenant_uuid"`
	AcceptanceRevision int64  `json:"expected_acceptance_revision"`
	RetentionDays      int    `json:"retention_days"`
}

type DatasetPreview struct {
	RequestUUID        string                `json:"request_uuid"`
	DatasetUUID        uuid.UUID             `json:"dataset_uuid"`
	TenantUUID         string                `json:"tenant_uuid"`
	AcceptanceRevision int64                 `json:"acceptance_revision"`
	RetentionDays      int                   `json:"retention_days"`
	CutoffAt           time.Time             `json:"cutoff_at"`
	ExpiresAt          time.Time             `json:"expires_at"`
	State              string                `json:"state"`
	StatusReason       string                `json:"status_reason,omitempty"`
	Result             *DatasetPreviewCounts `json:"result,omitempty"`
}

type datasetPreviewDocument struct {
	Preview              DatasetPreview
	Deployment           string
	Owner                string
	Installation         string
	TargetSubject        string
	RequestDigest        string
	ClaimReceipt         string
	ClaimFence           uint64
	ClaimDeadline        time.Time
	MaxExamined          int
	ReportDigest         string
	ExecutionRequestUUID string
}

type datasetPreviewGrant struct {
	RequestUUID           string    `json:"request_uuid"`
	DatasetUUID           uuid.UUID `json:"dataset_uuid"`
	TenantUUID            string    `json:"tenant_uuid"`
	SourceBindingUUID     uuid.UUID `json:"source_binding_uuid"`
	SourceFingerprint     string    `json:"source_fingerprint"`
	AcceptanceRevision    int64     `json:"acceptance_revision"`
	DefinitionDigest      string    `json:"definition_digest"`
	SchemaContractVersion int64     `json:"schema_contract_version"`
	AdapterKey            string    `json:"adapter_key"`
	AdapterVersion        int64     `json:"adapter_version"`
	CutoffAt              time.Time `json:"cutoff_at"`
	DeadlineAt            time.Time `json:"deadline_at"`
	MaxExamined           int       `json:"max_examined"`
}

func (s *Service) datasetPreviewNamespace() string {
	return "dataset-preview:" + controlDigest([]byte(s.opts.Identity.InstanceID))
}

func (s *Service) loadDatasetPreview(ctx context.Context, db *gorm.DB, id string, lock bool) (model.CleanupRun, datasetPreviewDocument, error) {
	var row model.CleanupRun
	var doc datasetPreviewDocument
	q := db.WithContext(ctx).Where("tenant_id = 0 AND instance_id = ? AND run_id = ?", s.datasetPreviewNamespace(), id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, doc, controlError(404, "DATASET_PREVIEW_NOT_FOUND")
		}
		return row, doc, err
	}
	if json.Unmarshal(row.Document, &doc) != nil || row.RunID != doc.Preview.RequestUUID || doc.Deployment != s.opts.Identity.InstanceID {
		return row, doc, ErrStateUnavailable
	}
	return row, doc, nil
}

func saveDatasetPreview(tx *gorm.DB, row model.CleanupRun, doc datasetPreviewDocument) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	out := tx.Model(&row).Updates(map[string]any{"version": row.Version + 1, "state": doc.Preview.State, "document": datatypes.JSON(raw)})
	if out.Error != nil {
		return out.Error
	}
	if out.RowsAffected != 1 {
		return ErrStateUnavailable
	}
	return nil
}

func (s *Service) currentDatasetPreviewSource(ctx context.Context, db *gorm.DB, id uuid.UUID, revision int64, lock bool) (model.DatasetRegistration, installedDataset, error) {
	var row model.DatasetRegistration
	var source installedDataset
	q := db.WithContext(ctx).Where("tenant_id = 0 AND deployment_id = ? AND dataset_uuid = ? AND is_current AND status = 1", s.opts.Identity.InstanceID, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, source, controlError(404, "DATASET_NOT_FOUND")
		}
		return row, source, err
	}
	if row.State != "accepted" || revision != row.AcceptanceRevision {
		return row, source, controlError(409, "DATASET_PREVIEW_INVALIDATED")
	}
	source, e := s.resolveInstalledDataset(ctx, row.OwnerID, row.DatasetKey)
	if e != nil || row.SourceFingerprint == "" || row.BindingUUID != source.binding || row.Installation != source.plugin.Revision ||
		row.DefinitionDigest != source.definitionFingerprint() {
		return row, source, controlError(409, "DATASET_PREVIEW_INVALIDATED")
	}
	if source.declaration.RetentionBounds == nil {
		return row, source, controlError(422, "DATASET_PREVIEW_NOT_ALLOWED")
	}
	var sameDB *gorm.DB
	if lock {
		sameDB = db
	}
	_, fingerprint, e := s.sampleInstalledDatasetTx(ctx, source, sameDB)
	if e != nil || row.SourceFingerprint != fingerprint {
		return row, source, controlError(409, "DATASET_PREVIEW_INVALIDATED")
	}
	return row, source, nil
}

func (s *Service) datasetPreviewTarget(ctx context.Context, db *gorm.DB, tenant, plugin string) (string, error) {
	return s.datasetWorkerTarget(ctx, db, tenant, plugin, DatasetPreviewCapability)
}

func (s *Service) datasetWorkerTarget(ctx context.Context, db *gorm.DB, tenant, plugin, capability string) (string, error) {
	var target tenantmodel.Tenant
	err := db.WithContext(ctx).Where("uuid = ? AND status = ?", tenant, tenantmodel.TenantStatusActive).Take(&target).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", controlError(403, "DATASET_TENANT_FORBIDDEN")
	}
	if err != nil {
		return "", err
	}
	access, err := cap.NewGrantStatusService(db).PluginSTSAccess(ctx, tenant, plugin)
	if err == nil {
		err = access.Require(capability)
	}
	if err == nil {
		err = access.Require(taskqueue.ConsumeCapability)
	}
	if err != nil {
		var denied *cap.DirectGrantError
		if errors.As(err, &denied) && denied.Status < 500 {
			return "", controlError(403, "DATASET_PREVIEW_WORKER_FORBIDDEN")
		}
		return "", err
	}
	if err := pluginservice.NewPluginDrainJobService(db).EnsurePluginAcceptsNewUsage(ctx, plugin); err != nil {
		return "", err
	}
	return access.Subject, nil
}

func (s *Service) previewRuntimeScope(doc datasetPreviewDocument) taskqueue.RuntimeScope {
	return taskqueue.RuntimeScope{TenantUUID: doc.Preview.TenantUUID, SourceService: doc.TargetSubject, EnvironmentID: doc.Deployment}
}

func (s *Service) StartDatasetPreview(ctx context.Context, actor uint64, datasetID string, in DatasetPreviewInput) (DatasetPreview, bool, error) {
	id, e := parseDatasetUUID(datasetID)
	_, reqErr := parseDatasetUUID(in.RequestUUID)
	_, tenantErr := parseDatasetUUID(in.TenantUUID)
	if e != nil || reqErr != nil || tenantErr != nil || actor == 0 || !datasetRevisionOK(in.AcceptanceRevision) || in.RetentionDays < 1 || in.RetentionDays > 3650 {
		return DatasetPreview{}, false, controlError(400, "DATASET_PREVIEW_INVALID")
	}
	if s == nil || s.opts.DB == nil || s.opts.DatasetTasks == nil || s.opts.Identity.InstanceID == "" {
		return DatasetPreview{}, false, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
	}
	encoded, _ := json.Marshal(in)
	digest := controlDigest(encoded)
	var doc datasetPreviewDocument
	var msg event_bus.TaskMessage
	created := false
	err := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", s.datasetPreviewNamespace()+":"+in.RequestUUID).Error; err != nil {
			return err
		}
		row, orig, err := s.loadDatasetPreview(ctx, tx, in.RequestUUID, false)
		if err == nil {
			if row.ActorID != actor || orig.Preview.DatasetUUID != id || orig.RequestDigest != digest {
				return controlError(409, "DATASET_REQUEST_CONFLICT")
			}
			doc = orig
			return nil
		}
		if dto.CodeOf(err) != "DATASET_PREVIEW_NOT_FOUND" {
			return err
		}
		head, source, err := s.currentDatasetPreviewSource(ctx, tx, id, in.AcceptanceRevision, true)
		if err != nil {
			return err
		}
		bounds := source.declaration.RetentionBounds
		if in.RetentionDays < bounds.MinimumDays || in.RetentionDays > bounds.MaximumDays {
			return controlError(422, "DATASET_RETENTION_OUT_OF_BOUNDS")
		}
		subject, err := s.datasetPreviewTarget(ctx, tx, in.TenantUUID, head.OwnerID)
		if err != nil {
			return err
		}
		now := s.opts.Now().UTC()
		doc = datasetPreviewDocument{
			Deployment: s.opts.Identity.InstanceID, Owner: head.OwnerID, Installation: head.Installation,
			TargetSubject: subject, RequestDigest: digest,
			Preview: DatasetPreview{RequestUUID: in.RequestUUID, DatasetUUID: id, TenantUUID: in.TenantUUID,
				AcceptanceRevision: in.AcceptanceRevision, RetentionDays: in.RetentionDays,
				CutoffAt: now.Add(-time.Duration(in.RetentionDays) * 24 * time.Hour), ExpiresAt: now.Add(datasetPreviewLifetime), State: "queued"},
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		row = model.CleanupRun{InstanceID: s.datasetPreviewNamespace(), RunID: in.RequestUUID, ActorID: actor,
			Version: 1, State: doc.Preview.State, ExpiresAt: doc.Preview.ExpiresAt, Document: raw}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		notice, _ := json.Marshal(map[string]string{"contract_version": datasetHostContract, "request_uuid": in.RequestUUID})
		msg, err = s.opts.DatasetTasks.RegisterDatasetPreview(ctx, tx, s.previewRuntimeScope(doc), in.RequestUUID, notice)
		created = err == nil
		return err
	})
	if err != nil {
		return DatasetPreview{}, false, err
	}
	if created {
		publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		// A committed queue row is recoverable when Redis is unavailable.
		_ = s.opts.DatasetTasks.DeliverDatasetTask(publishCtx, msg)
		s.PublishChange("dataset_preview_changed")
		return doc.Preview, true, nil
	}
	return s.projectDatasetPreview(ctx, doc), false, nil
}

func (s *Service) GetDatasetPreview(ctx context.Context, actor uint64, datasetID, requestID string) (DatasetPreview, error) {
	id, e := parseDatasetUUID(datasetID)
	_, reqErr := parseDatasetUUID(requestID)
	if e != nil || reqErr != nil || actor == 0 {
		return DatasetPreview{}, controlError(400, "DATASET_PREVIEW_INVALID")
	}
	if s == nil || s.opts.DB == nil || s.opts.Identity.InstanceID == "" {
		return DatasetPreview{}, controlError(503, "DATASET_PREVIEW_UNAVAILABLE")
	}
	row, doc, err := s.loadDatasetPreview(ctx, s.opts.DB, requestID, false)
	if err != nil {
		return DatasetPreview{}, err
	}
	if doc.Preview.DatasetUUID != id || row.ActorID != actor {
		return DatasetPreview{}, controlError(404, "DATASET_PREVIEW_NOT_FOUND")
	}
	return s.projectDatasetPreview(ctx, doc), nil
}

func (s *Service) projectDatasetPreview(ctx context.Context, doc datasetPreviewDocument) DatasetPreview {
	view := doc.Preview
	_, _, err := s.currentDatasetPreviewSource(ctx, s.opts.DB, view.DatasetUUID, view.AcceptanceRevision, false)
	subject, targetErr := s.datasetPreviewTarget(ctx, s.opts.DB, view.TenantUUID, doc.Owner)
	if err != nil || targetErr != nil || subject != doc.TargetSubject {
		view.State, view.StatusReason, view.Result = "invalidated", "dataset_source_or_worker_changed", nil
	} else if !s.opts.Now().Before(view.ExpiresAt) {
		view.State, view.StatusReason, view.Result = "expired", "preview_expired", nil
	}
	return view
}
