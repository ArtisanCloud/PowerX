package capability_registry

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/eventbus"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RedisClient exposes the subset used by capability record repositories.
type RedisClient interface {
	redis.UniversalClient
}

// SyncWorkerConfig captures dependencies required by the Capability Sync Worker.
type SyncWorkerConfig struct {
	DB              *gorm.DB
	Redis           RedisClient
	EventBus        event_bus.EventBus
	Logger          *pxlog.Logger
	Clock           func() time.Time
	Audit           *AuditService
	Alerting        CapabilityAlerting
	WorkflowCatalog *WorkflowCatalog
}

// SyncWorker ingests plugin artifacts and persists CapabilityRecords.
type SyncWorker struct {
	recordRepo      *repo.CapabilityRecordRepository
	templateRepo    *repo.WorkflowTemplateRepository
	jobRepo         *repo.CapabilitySyncJobRepository
	eventBus        event_bus.EventBus
	logger          *pxlog.Logger
	now             func() time.Time
	audit           *AuditService
	alerting        CapabilityAlerting
	workflowCatalog *WorkflowCatalog
}

// NewSyncWorker constructs a new worker instance.
func NewSyncWorker(cfg SyncWorkerConfig) *SyncWorker {
	if cfg.DB == nil {
		panic("capability registry sync worker requires DB")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	clock := cfg.Clock
	if clock == nil {
		clock = time.Now
	}

	recordRepo := repo.NewCapabilityRecordRepository(cfg.DB, cfg.Redis)
	templateRepo := repo.NewWorkflowTemplateRepository(cfg.DB)
	jobRepo := repo.NewCapabilitySyncJobRepository(cfg.DB)

	audit := cfg.Audit
	if audit == nil {
		audit = NewAuditService(AuditServiceOptions{
			JobRepo:  jobRepo,
			EventBus: cfg.EventBus,
			Clock:    clock,
		})
	}

	return &SyncWorker{
		recordRepo:      recordRepo,
		templateRepo:    templateRepo,
		jobRepo:         jobRepo,
		eventBus:        cfg.EventBus,
		logger:          logger,
		now:             clock,
		audit:           audit,
		alerting:        cfg.Alerting,
		workflowCatalog: cfg.WorkflowCatalog,
	}
}

// ProcessArtifact processes a plugin artifact located at path (directory or .pxp archive).
func (w *SyncWorker) ProcessArtifact(ctx context.Context, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat artifact: %w", err)
	}

	artifactPath := path
	root := path
	cleanup := func() {}
	if !info.IsDir() {
		root, cleanup, err = extractArchive(path)
		if err != nil {
			return fmt.Errorf("extract artifact: %w", err)
		}
	}
	defer cleanup()

	manifest, err := loadManifest(root)
	if err != nil {
		return fmt.Errorf("load plugin manifest: %w", err)
	}

	catalog, err := loadCatalog(root)
	if err != nil {
		return fmt.Errorf("load capability catalog: %w", err)
	}
	if len(catalog.Capabilities) == 0 {
		return errors.New("capability catalog is empty")
	}

	pluginMeta := catalog.Plugin
	if pluginMeta.ID == "" {
		pluginMeta.ID = manifest.ID
	}
	if pluginMeta.Version == "" {
		pluginMeta.Version = manifest.Version
	}
	if pluginMeta.Name == "" {
		pluginMeta.Name = manifest.Name
	}

	if err := ensureExposureDir(root); err != nil {
		alertErr := &AssetAlertError{
			PluginID:      pluginMeta.ID,
			PluginName:    pluginMeta.Name,
			PluginVersion: pluginMeta.Version,
			AssetPath:     "contracts/exposure",
			ArtifactPath:  artifactPath,
			Reason:        AssetAlertReasonExposureMissing,
			Detail:        err.Error(),
			Err:           err,
		}
		w.emitAssetAlert(ctx, alertErr)
		return alertErr
	}

	var errs []error
	for _, capability := range catalog.Capabilities {
		if err := w.syncCapability(ctx, artifactPath, root, pluginMeta, capability); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", capability.capabilityID(), err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if w.workflowCatalog != nil {
		if _, err := w.workflowCatalog.Refresh(ctx); err != nil {
			w.logger.WarnF(ctx, "[capability_sync] refresh workflow catalog failed: %v", err)
		}
	}
	return nil
}

func (w *SyncWorker) emitAssetAlert(ctx context.Context, alertErr *AssetAlertError) {
	if w == nil || alertErr == nil {
		return
	}
	if w.alerting != nil {
		w.alerting.NotifyAssetIssue(ctx, alertErr.ToInput())
	}
}

func (w *SyncWorker) raiseAssetAlert(ctx context.Context, artifactPath string, plugin pluginMetadata, capabilityID, assetPath, reason, detail string, err error) error {
	alertErr := &AssetAlertError{
		PluginID:      plugin.ID,
		PluginName:    plugin.Name,
		PluginVersion: plugin.Version,
		CapabilityID:  strings.TrimSpace(capabilityID),
		AssetPath:     assetPath,
		ArtifactPath:  artifactPath,
		Reason:        reason,
		Detail:        detail,
		Err:           err,
	}
	w.emitAssetAlert(ctx, alertErr)
	return alertErr
}

func (w *SyncWorker) syncCapability(ctx context.Context, artifactPath, root string, plugin pluginMetadata, capability catalogCapability) error {
	capabilityID := capability.capabilityID()
	if capabilityID == "" {
		return errors.New("capability id missing")
	}

	startedAt := w.now()
	job, err := w.jobRepo.Create(ctx, &models.CapabilitySyncJob{
		CapabilityID:  capabilityID,
		PluginID:      plugin.ID,
		PluginVersion: plugin.Version,
		Status:        "running",
		StartedAt:     startedAt,
		Metadata:      mustJSON(map[string]string{"plugin_name": plugin.Name}),
	})
	if err != nil {
		return fmt.Errorf("create sync job: %w", err)
	}
	if w.audit != nil {
		w.audit.PublishCatalogEvent(ctx, eventbus.TopicCapabilityCatalogSyncStarted, CatalogSyncEvent{
			JobID:         job.UUID.String(),
			PluginID:      plugin.ID,
			PluginName:    plugin.Name,
			PluginVersion: plugin.Version,
			CapabilityID:  capabilityID,
		})
	}

	var syncErr error
	defer func() {
		fields := map[string]interface{}{
			"finished_at": w.now(),
		}
		eventName := eventbus.TopicCapabilityCatalogSyncSucceeded
		if syncErr != nil {
			fields["status"] = "failed"
			fields["error_summary"] = syncErr.Error()
			eventName = eventbus.TopicCapabilityCatalogSyncFailed
		} else {
			fields["status"] = "succeeded"
		}
		if err := w.jobRepo.UpdateFields(ctx, job.UUID, fields); err != nil {
			w.logger.WarnF(ctx, "[capability_sync] update job %s failed: %v", job.UUID, err)
		}
		hashBefore := job.HashBefore
		hashAfter := job.HashAfter
		if w.audit != nil {
			w.audit.PublishCatalogEvent(ctx, eventName, CatalogSyncEvent{
				JobID:         job.UUID.String(),
				PluginID:      plugin.ID,
				PluginName:    plugin.Name,
				PluginVersion: plugin.Version,
				CapabilityID:  capabilityID,
				HashBefore:    hashBefore,
				HashAfter:     hashAfter,
			})
		}
	}()

	if len(capability.Protocols) == 0 {
		syncErr = fmt.Errorf("capability %s has no protocol bindings", capabilityID)
		return syncErr
	}
	if err := w.validateSchemaRefs(ctx, artifactPath, root, plugin, capability); err != nil {
		syncErr = err
		return syncErr
	}

	if existing, err := w.recordRepo.GetByCapabilityID(ctx, capabilityID); err == nil {
		if updateErr := w.jobRepo.UpdateFields(ctx, job.UUID, map[string]interface{}{
			"hash_before": existing.CapabilitiesHash,
		}); updateErr != nil {
			w.logger.WarnF(ctx, "[capability_sync] set job hash_before failed: %v", updateErr)
		}
		job.HashBefore = existing.CapabilitiesHash
	} else if !errors.Is(err, repo.ErrCapabilityRecordNotFound) {
		syncErr = fmt.Errorf("fetch capability record: %w", err)
		return syncErr
	}

	record, err := buildCapabilityRecord(plugin, capability, w.now())
	if err != nil {
		syncErr = err
		return syncErr
	}
	saved, err := w.recordRepo.Upsert(ctx, record)
	if err != nil {
		syncErr = fmt.Errorf("upsert capability record: %w", err)
		return syncErr
	}
	if updateErr := w.jobRepo.UpdateFields(ctx, job.UUID, map[string]interface{}{
		"hash_after": saved.CapabilitiesHash,
	}); updateErr != nil {
		w.logger.WarnF(ctx, "[capability_sync] set job hash_after failed: %v", updateErr)
	}
	job.HashAfter = saved.CapabilitiesHash
	job.HashBefore = saved.CapabilitiesHash

	if err := w.syncWorkflowTemplates(ctx, capabilityID, capability.WorkflowTemplates, saved.CapabilitiesHash); err != nil {
		syncErr = err
		return syncErr
	}

	return nil
}

func (w *SyncWorker) syncWorkflowTemplates(ctx context.Context, capabilityID string, templates []catalogWorkflowTemplate, capabilityHash string) error {
	if err := w.templateRepo.DeleteByCapabilityID(ctx, capabilityID); err != nil {
		return fmt.Errorf("cleanup workflow templates: %w", err)
	}
	if len(templates) == 0 {
		return nil
	}

	now := w.now()
	payload := make([]*models.WorkflowTemplateRef, 0, len(templates))
	for _, tpl := range templates {
		if tpl.TemplateID == "" {
			return errors.New("workflow template missing template_id")
		}
		hash := hashJSON(struct {
			ID    string
			Steps json.RawMessage
		}{
			ID:    tpl.TemplateID,
			Steps: tpl.Steps,
		})
		requiresUpgrade := true
		if tpl.RequiresManualUpgrade != nil {
			requiresUpgrade = *tpl.RequiresManualUpgrade
		}

		payload = append(payload, &models.WorkflowTemplateRef{
			CapabilityID:          capabilityID,
			TemplateID:            tpl.TemplateID,
			Name:                  tpl.Name,
			Description:           tpl.Description,
			Steps:                 datatypes.JSON(tpl.Steps),
			ParamsSchema:          datatypes.JSON(tpl.ParamsSchema),
			ProtocolRequirements:  datatypes.JSON(tpl.ProtocolRequirements),
			CapabilitiesHash:      capabilityHash,
			TemplateHash:          hash,
			RequiresManualUpgrade: requiresUpgrade,
			LastSyncedAt:          &now,
		})
	}
	if _, err := w.templateRepo.UpsertBatch(ctx, payload); err != nil {
		return fmt.Errorf("upsert workflow templates: %w", err)
	}
	return nil
}

func (w *SyncWorker) validateSchemaRefs(ctx context.Context, artifactPath, root string, plugin pluginMetadata, capability catalogCapability) error {
	for _, binding := range capability.Protocols {
		schemaRef := strings.TrimSpace(binding.SchemaRef)
		if schemaRef == "" {
			continue
		}
		if strings.Contains(schemaRef, "://") {
			continue
		}
		refPath := filepath.Join(root, filepath.Clean(schemaRef))
		raw, err := os.ReadFile(refPath)
		if err != nil {
			detail := fmt.Sprintf("protocol schema %s missing: %v", schemaRef, err)
			return w.raiseAssetAlert(ctx, artifactPath, plugin, capability.capabilityID(), schemaRef, AssetAlertReasonSchemaMissing, detail, err)
		}
		if len(raw) == 0 {
			continue
		}
		if strings.HasSuffix(strings.ToLower(schemaRef), ".json") && !json.Valid(raw) {
			parseErr := fmt.Errorf("protocol schema %s is invalid JSON", schemaRef)
			return w.raiseAssetAlert(ctx, artifactPath, plugin, capability.capabilityID(), schemaRef, AssetAlertReasonSchemaInvalid, parseErr.Error(), parseErr)
		}
	}
	return nil
}

func ensureExposureDir(root string) error {
	exposurePath := filepath.Join(root, "contracts", "exposure")
	info, err := os.Stat(exposurePath)
	if err != nil {
		return fmt.Errorf("contracts/exposure missing: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("contracts/exposure is not a directory")
	}
	return nil
}

func buildCapabilityRecord(plugin pluginMetadata, capability catalogCapability, now time.Time) (*models.CapabilityRecord, error) {
	record := &models.CapabilityRecord{
		CapabilityID:         capability.capabilityID(),
		PluginID:             plugin.ID,
		PluginVersion:        plugin.Version,
		Title:                capability.Title,
		Description:          capability.Description,
		Categories:           toJSON(capability.Categories),
		Intents:              toJSON(capability.Intents),
		ToolScope:            toJSON(capability.ToolScope),
		Protocols:            toJSON(capability.Protocols),
		Policy:               datatypes.JSON(capability.Policy),
		WorkflowTemplateRefs: toJSON(capability.WorkflowTemplates),
		CompositeGraphs:      datatypes.JSON(capability.CompositeGraphs),
		Annotations:          datatypes.JSON(capability.Annotations),
		Status:               defaultStatus(capability.Status),
		PublishedAt:          firstNonZeroTime(capability.PublishedAt, &now),
		CreatedBy:            plugin.ID,
		UpdatedBy:            "capability_sync",
	}
	if record.Policy == nil {
		record.Policy = datatypes.JSON([]byte("{}"))
	}
	record.CapabilitiesHash = hashJSON(capability.hashSource())
	record.ProtocolHash = hashJSON(capability.Protocols)
	return record, nil
}

func defaultStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "published"
	}
	return status
}

func firstNonZeroTime(t *time.Time, fallback *time.Time) *time.Time {
	if t != nil && !t.IsZero() {
		return t
	}
	return fallback
}

func toJSON(value interface{}) datatypes.JSON {
	if value == nil {
		return datatypes.JSON([]byte("null"))
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(raw)
}

func hashJSON(value interface{}) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func extractArchive(path string) (string, func(), error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", nil, err
	}

	tempDir, err := os.MkdirTemp("", "capability-sync-*")
	if err != nil {
		_ = r.Close()
		return "", nil, err
	}

	cleanup := func() {
		_ = r.Close()
		_ = os.RemoveAll(tempDir)
	}

	for _, file := range r.File {
		if err := extractZipFile(file, tempDir); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return tempDir, cleanup, nil
}

func extractZipFile(file *zip.File, dest string) error {
	targetPath := filepath.Join(dest, file.Name)
	if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(dest)) {
		return fmt.Errorf("zip traversal detected: %s", file.Name)
	}
	if file.FileInfo().IsDir() {
		return os.MkdirAll(targetPath, file.Mode())
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func loadManifest(root string) (*plugin_mgr.Manifest, error) {
	candidates := []string{
		filepath.Join(root, "plugin.yaml"),
		filepath.Join(root, "payload", "plugin.yaml"),
	}
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err == nil {
			var manifest plugin_mgr.Manifest
			if err := yaml.Unmarshal(raw, &manifest); err != nil {
				return nil, err
			}
			return &manifest, nil
		}
	}
	return nil, errors.New("plugin.yaml not found")
}

func loadCatalog(root string) (*capabilityCatalog, error) {
	path := filepath.Join(root, "capabilities", "catalog.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var catalog capabilityCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

type capabilityCatalog struct {
	Plugin       pluginMetadata      `json:"plugin"`
	Capabilities []catalogCapability `json:"capabilities"`
}

type pluginMetadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type catalogCapability struct {
	ID                string                    `json:"id"`
	CapabilityIDAlias string                    `json:"capability_id"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description"`
	Categories        []string                  `json:"categories"`
	Intents           []string                  `json:"intents"`
	ToolScope         []string                  `json:"tool_scope"`
	Policy            json.RawMessage           `json:"policy"`
	Protocols         []models.ProtocolBinding  `json:"protocols"`
	WorkflowTemplates []catalogWorkflowTemplate `json:"workflow_templates"`
	CompositeGraphs   json.RawMessage           `json:"composite_graphs"`
	Annotations       json.RawMessage           `json:"annotations"`
	Status            string                    `json:"status"`
	PublishedAt       *time.Time                `json:"published_at"`
}

func (c catalogCapability) capabilityID() string {
	if strings.TrimSpace(c.ID) != "" {
		return c.ID
	}
	return strings.TrimSpace(c.CapabilityIDAlias)
}

func (c catalogCapability) hashSource() interface{} {
	return struct {
		ID        string
		Title     string
		Intents   []string
		ToolScope []string
		Protocols []models.ProtocolBinding
		Policy    json.RawMessage
	}{
		ID:        c.capabilityID(),
		Title:     c.Title,
		Intents:   c.Intents,
		ToolScope: c.ToolScope,
		Protocols: c.Protocols,
		Policy:    c.Policy,
	}
}

type catalogWorkflowTemplate struct {
	TemplateID            string          `json:"template_id"`
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	Steps                 json.RawMessage `json:"steps"`
	ParamsSchema          json.RawMessage `json:"params_schema"`
	ProtocolRequirements  json.RawMessage `json:"protocol_requirements"`
	RequiresManualUpgrade *bool           `json:"requires_manual_upgrade"`
}

func mustJSON(value interface{}) datatypes.JSON {
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return datatypes.JSON(raw)
}
