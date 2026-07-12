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
	"sort"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	registryservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	TenantUUID      string
}

// SyncWorker ingests plugin artifacts and persists CapabilityRecords.
type SyncWorker struct {
	db              *gorm.DB
	recordRepo      *repo.CapabilityRecordRepository
	templateRepo    *repo.WorkflowTemplateRepository
	jobRepo         *repo.CapabilitySyncJobRepository
	eventBus        event_bus.EventBus
	logger          *pxlog.Logger
	now             func() time.Time
	permissionRepo  *iamrepo.PermissionRepository
	audit           *AuditService
	alerting        CapabilityAlerting
	workflowCatalog *WorkflowCatalog
	registrySvc     *registryservice.Service
	tenantUUID      string
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
		db:              cfg.DB,
		recordRepo:      recordRepo,
		templateRepo:    templateRepo,
		jobRepo:         jobRepo,
		eventBus:        cfg.EventBus,
		logger:          logger,
		now:             clock,
		permissionRepo:  iamrepo.NewPermissionRepository(cfg.DB),
		audit:           audit,
		alerting:        cfg.Alerting,
		workflowCatalog: cfg.WorkflowCatalog,
		registrySvc:     registryservice.NewService(registryservice.ServiceOptions{DB: cfg.DB, Clock: clock}),
		tenantUUID:      strings.TrimSpace(cfg.TenantUUID),
	}
}

func (w *SyncWorker) WithTenant(tenantUUID string) *SyncWorker {
	if w == nil {
		return nil
	}
	next := *w
	next.tenantUUID = strings.TrimSpace(tenantUUID)
	return &next
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

	catalog, err := loadCatalog(root, manifest)
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
	if err := w.syncCapabilityRegistration(ctx, plugin.ID, capabilityID, capability); err != nil {
		syncErr = err
		return syncErr
	}
	if err := w.syncCapabilityPermissions(ctx, plugin, capability); err != nil {
		syncErr = err
		return syncErr
	}

	return nil
}

func (w *SyncWorker) syncCapabilityPermissions(ctx context.Context, plugin pluginMetadata, capability catalogCapability) error {
	if w == nil || w.permissionRepo == nil {
		return errors.New("permission repository is not configured")
	}
	pluginID := strings.TrimSpace(plugin.ID)
	if pluginID == "" {
		return errors.New("plugin id missing")
	}
	codes := permissionCodesFromCatalogCapability(pluginID, capability)
	if len(codes) == 0 {
		return nil
	}
	rows := make([]iammodel.Permission, 0, len(codes))
	for _, code := range codes {
		module, resource, action, ok := parsePluginPermissionCode(code, pluginID)
		if !ok {
			return fmt.Errorf("invalid capability permission_code: capability=%s permission_code=%s", capability.capabilityID(), code)
		}
		rows = append(rows, iammodel.Permission{
			Module:      module,
			Resource:    resource,
			Action:      action,
			Effect:      "allow",
			Description: firstNonEmptyCatalogString(strings.TrimSpace(capability.Description), capability.capabilityID()),
			AllowAPIKey: false,
			Meta: datatypes.JSON(mustRawJSON(map[string]any{
				"label":         firstNonEmptyCatalogString(strings.TrimSpace(capability.Title), capability.capabilityID()),
				"module":        module,
				"type":          "plugin_capability",
				"plugin_id":     pluginID,
				"capability_id": capability.capabilityID(),
				"permission":    code,
			})),
			Status:     iammodel.PermissionStatusActive,
			Source:     pluginID,
			Introduced: firstNonEmptyCatalogString(strings.TrimSpace(capability.Version), strings.TrimSpace(plugin.Version)),
		})
	}
	if err := w.permissionRepo.UpsertBatch(ctx, rows); err != nil {
		return err
	}
	return w.grantCapabilityPermissionsToDefaultRoles(ctx, rows, defaultRoleGrantsFromCapability(capability))
}

func (w *SyncWorker) grantCapabilityPermissionsToDefaultRoles(ctx context.Context, rows []iammodel.Permission, extraRoleCodes []string) error {
	if w == nil || w.db == nil || len(rows) == 0 {
		return nil
	}
	roleCodes, err := defaultCapabilityRoleCodes(extraRoleCodes)
	if err != nil {
		return err
	}
	query := w.db.WithContext(ctx).
		Model(&iammodel.Permission{}).
		Where("status = ?", iammodel.PermissionStatusActive)
	tripleWhere := w.db.Where("1 = 0")
	for _, row := range rows {
		tripleWhere = tripleWhere.Or("(module = ? AND resource = ? AND action = ?)", row.Module, row.Resource, row.Action)
	}
	var permissionIDs []uint64
	if err := query.Where(tripleWhere).Pluck("id", &permissionIDs).Error; err != nil {
		return err
	}
	if len(permissionIDs) == 0 {
		return nil
	}

	roleQuery := w.db.WithContext(ctx).
		Model(&iammodel.Role{}).
		Where("scope = ? AND code IN ?", string(coreiam.RoleScopeTenant), roleCodes)
	if tenantUUID := strings.TrimSpace(w.tenantUUID); tenantUUID != "" {
		roleQuery = roleQuery.Where("tenant_uuid = ?", tenantUUID)
	}
	var roles []iammodel.Role
	if err := roleQuery.Find(&roles).Error; err != nil {
		return err
	}
	if len(roles) == 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	grants := make([]iammodel.RolePermission, 0, len(roles)*len(permissionIDs))
	touchedTenants := map[string]struct{}{}
	for _, role := range roles {
		touchedTenants[strings.TrimSpace(role.TenantUUID)] = struct{}{}
		for _, permissionID := range permissionIDs {
			grants = append(grants, iammodel.RolePermission{
				RoleID:       role.ID,
				PermissionID: permissionID,
				CreatedAt:    now,
			})
		}
	}
	if err := w.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&grants).Error; err != nil {
		return err
	}
	for tenantUUID := range touchedTenants {
		if err := invalidateAgentAuthzIAMCache(ctx, tenantUUID); err != nil {
			return err
		}
	}
	return nil
}

func defaultCapabilityRoleCodes(extra []string) ([]string, error) {
	allowed := map[string]struct{}{
		string(coreiam.CodeRoleOwner):    {},
		string(coreiam.CodeRoleAdmin):    {},
		string(coreiam.CodeRoleUser):     {},
		string(coreiam.CodeRoleReadonly): {},
		string(coreiam.CodeRoleVendor):   {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 2+len(extra))
	add := func(code string) error {
		code = strings.TrimSpace(code)
		if code == "" {
			return nil
		}
		if _, ok := allowed[code]; !ok {
			return fmt.Errorf("invalid default role grant code: %s", code)
		}
		if _, exists := seen[code]; exists {
			return nil
		}
		seen[code] = struct{}{}
		out = append(out, code)
		return nil
	}
	if err := add(string(coreiam.CodeRoleOwner)); err != nil {
		return nil, err
	}
	if err := add(string(coreiam.CodeRoleAdmin)); err != nil {
		return nil, err
	}
	for _, code := range extra {
		if err := add(code); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func defaultRoleGrantsFromCapability(capability catalogCapability) []string {
	var annotations map[string]any
	_ = json.Unmarshal(capability.Annotations, &annotations)
	candidates := stringListFromAnyCapability(annotations["default_role_grants"])
	candidates = append(candidates, stringListFromAnyCapability(annotations["default_role_codes"])...)
	candidates = append(candidates, capability.DefaultRoleGrants...)
	return dedupeSortedStrings(candidates)
}

func invalidateAgentAuthzIAMCache(ctx context.Context, tenantUUID string) error {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil
	}
	store := cache.GetCache()
	if store == nil {
		return nil
	}
	_, err := store.Increment(ctx, fmt.Sprintf("agentauthz:effective:iam-version:%s", strings.ToLower(tenantUUID)), 1)
	return err
}

func permissionCodesFromCatalogCapability(pluginID string, capability catalogCapability) []string {
	var annotations map[string]any
	_ = json.Unmarshal(capability.Annotations, &annotations)
	candidates := stringListFromAnyCapability(annotations["permission_codes"])
	if code := strings.TrimSpace(anyStringCapability(annotations["permission_code"])); code != "" {
		candidates = append(candidates, code)
	}
	if len(candidates) == 0 {
		for _, scope := range capability.ToolScope {
			scope = strings.TrimSpace(scope)
			if scope == "" {
				continue
			}
			if strings.Contains(scope, ":") {
				candidates = append(candidates, scope)
				continue
			}
			candidates = append(candidates, pluginID+"."+scope+":use")
		}
	}
	return dedupeSortedStrings(candidates)
}

func parsePluginPermissionCode(code string, pluginID string) (module, resource, action string, ok bool) {
	left, right, found := strings.Cut(strings.TrimSpace(code), ":")
	if !found {
		return "", "", "", false
	}
	action = strings.TrimSpace(right)
	left = strings.TrimSpace(left)
	pluginID = strings.TrimSpace(pluginID)
	if pluginID != "" {
		prefix := pluginID + "."
		if strings.HasPrefix(left, prefix) {
			resource = strings.TrimSpace(strings.TrimPrefix(left, prefix))
			if resource == "" || action == "" {
				return "", "", "", false
			}
			return pluginID, resource, action, true
		}
	}
	parts := strings.Split(left, ".")
	if len(parts) < 2 || action == "" {
		return "", "", "", false
	}
	module = strings.TrimSpace(parts[0])
	resource = strings.TrimSpace(strings.Join(parts[1:], "."))
	if module == "" || resource == "" {
		return "", "", "", false
	}
	return module, resource, action, true
}

func stringListFromAnyCapability(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(anyStringCapability(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func anyStringCapability(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func (w *SyncWorker) syncCapabilityRegistration(ctx context.Context, pluginID string, capabilityID string, capability catalogCapability) error {
	tenantUUID := strings.TrimSpace(w.tenantUUID)
	if tenantUUID == "" {
		return nil
	}
	if w.registrySvc == nil {
		return errors.New("capability registry service is not configured")
	}
	adapters := make([]registryservice.AdapterEndpoint, 0, len(capability.Protocols))
	for _, protocol := range capability.Protocols {
		adapter := registrationAdapterFromProtocol(capabilityID, pluginID, protocol)
		if adapter.AdapterID == "" {
			continue
		}
		adapters = append(adapters, adapter)
	}
	if len(adapters) == 0 {
		return fmt.Errorf("capability %s has no executable protocol adapters", capabilityID)
	}
	latest, err := w.registrySvc.GetRegistration(ctx, capabilityID, tenantUUID, registryservice.GetRegistrationOptions{IncludeDisabled: true})
	payload := registryservice.RegistrationPayload{
		CapabilityID: capabilityID,
		TenantUUID:   tenantUUID,
		ContractRef:  defaultString(capability.Version, "1.0.0"),
		Status:       "published",
		Adapters:     adapters,
		RoutingPolicy: registryservice.RoutingPolicy{
			Strategy: "weighted_round_robin",
		},
	}
	if err == nil {
		payload.Version = latest.Version
		_, err = w.registrySvc.UpdateRegistration(ctx, registryservice.UpdateRegistrationInput{
			Registration: payload,
			Actor:        "capability_sync",
		})
		return err
	}
	if !errors.Is(err, registryservice.ErrRegistrationNotFound) {
		return err
	}
	_, err = w.registrySvc.CreateRegistration(ctx, registryservice.CreateRegistrationInput{
		Registration: payload,
		Actor:        "capability_sync",
	})
	return err
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func registrationAdapterFromProtocol(capabilityID string, pluginID string, protocol models.ProtocolBinding) registryservice.AdapterEndpoint {
	channel := strings.TrimSpace(protocol.Channel)
	if channel == "" {
		return registryservice.AdapterEndpoint{}
	}
	adapterID := strings.TrimSpace(protocol.ToolRef)
	if adapterID == "" {
		adapterID = strings.TrimSpace(protocol.RPC)
	}
	if adapterID == "" {
		adapterID = strings.TrimSpace(protocol.Method)
	}
	if adapterID == "" {
		adapterID = channel
	}
	endpoint := normalizePluginProtocolEndpoint(pluginID, channel, protocol.Endpoint)
	serviceRef := strings.TrimSpace(protocol.RPC)
	if serviceRef == "" {
		serviceRef = strings.TrimSpace(protocol.ToolRef)
	}
	labels := map[string]string{
		"source": "plugin_catalog",
	}
	if method := strings.ToUpper(strings.TrimSpace(protocol.Method)); method != "" {
		labels["method"] = method
	}
	if rpc := strings.TrimSpace(protocol.RPC); rpc != "" {
		labels["rpc"] = rpc
	}
	if toolRef := strings.TrimSpace(protocol.ToolRef); toolRef != "" {
		labels["tool_ref"] = toolRef
	}
	return registryservice.AdapterEndpoint{
		AdapterID:     capabilityID + "." + adapterID,
		TransportType: channel,
		Endpoint:      endpoint,
		ServiceRef:    serviceRef,
		Weight:        100,
		TimeoutMS:     30000,
		Labels:        labels,
		IsActive:      true,
	}
}

func normalizePluginProtocolEndpoint(pluginID string, channel string, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	lowerEndpoint := strings.ToLower(endpoint)
	if strings.HasPrefix(lowerEndpoint, "http://") || strings.HasPrefix(lowerEndpoint, "https://") || strings.HasPrefix(endpoint, "/_p/") {
		return endpoint
	}
	normalizedChannel := strings.ToLower(strings.TrimSpace(channel))
	if normalizedChannel != "rest" && normalizedChannel != "http" {
		return endpoint
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return endpoint
	}
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return "/_p/" + pluginID + endpoint
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

func buildCapabilityRecord(plugin pluginMetadata, capability catalogCapability, now time.Time) (*models.CapabilityRecord, error) {
	record := &models.CapabilityRecord{
		CapabilityID:         capability.capabilityID(),
		PluginID:             plugin.ID,
		PluginVersion:        plugin.Version,
		Title:                firstNonEmptyCatalogString(strings.TrimSpace(capability.Title), firstLocaleText(capability.TitleI18n), capability.capabilityID()),
		Description:          firstNonEmptyCatalogString(strings.TrimSpace(capability.Description), firstLocaleText(capability.DescriptionI18n)),
		Categories:           toJSON(capability.Categories),
		Intents:              toJSON(capability.Intents),
		ToolScope:            toJSON(capability.ToolScope),
		Protocols:            toJSON(capability.Protocols),
		Policy:               datatypes.JSON(capability.Policy),
		WorkflowTemplateRefs: toJSON(capability.WorkflowTemplates),
		CompositeGraphs:      datatypes.JSON(capability.CompositeGraphs),
		Annotations:          datatypes.JSON(capability.normalizedAnnotations()),
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

func firstLocaleText(in map[string]string) string {
	cleaned := cleanLocaleTextMap(in)
	for _, locale := range []string{"zh-CN", "zh", "en", "en-US", "ja", "ko"} {
		if value := strings.TrimSpace(cleaned[locale]); value != "" {
			return value
		}
	}
	for _, value := range cleaned {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyCatalogString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
		filepath.Join(root, "plugin.merged.yaml"),
		filepath.Join(root, "payload", "plugin.merged.yaml"),
		filepath.Join(root, "plugin.merged.json"),
		filepath.Join(root, "payload", "plugin.merged.json"),
		filepath.Join(root, "plugin.yaml"),
		filepath.Join(root, "payload", "plugin.yaml"),
	}
	for _, candidate := range candidates {
		raw, err := os.ReadFile(candidate)
		if err == nil {
			var manifest plugin_mgr.Manifest
			switch strings.ToLower(filepath.Ext(candidate)) {
			case ".json":
				if err := json.Unmarshal(raw, &manifest); err != nil {
					return nil, err
				}
			default:
				if err := yaml.Unmarshal(raw, &manifest); err != nil {
					return nil, err
				}
			}
			return &manifest, nil
		}
	}
	return nil, errors.New("plugin.yaml not found")
}

func loadCatalog(root string, manifest *plugin_mgr.Manifest) (*capabilityCatalog, error) {
	path := filepath.Join(root, "capabilities", "catalog.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var catalog capabilityCatalog
		if err := json.Unmarshal(raw, &catalog); err != nil {
			return nil, err
		}
		return &catalog, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return loadCatalogFromPluginCapabilities(root, manifest)
}

func loadCatalogFromPluginCapabilities(root string, pluginManifest *plugin_mgr.Manifest) (*capabilityCatalog, error) {
	path := filepath.Join(root, "plugin.d", "capabilities.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var capabilityManifest capabilityManifestCatalog
	if err := yaml.Unmarshal(raw, &capabilityManifest); err != nil {
		return nil, fmt.Errorf("parse plugin.d/capabilities.yaml: %w", err)
	}
	if len(capabilityManifest.Capabilities.Provides) == 0 {
		return nil, errors.New("plugin.d/capabilities.yaml has no capabilities.provides")
	}

	catalog := &capabilityCatalog{
		Capabilities: make([]catalogCapability, 0, len(capabilityManifest.Capabilities.Provides)),
	}
	exposureByCapability := exposureChannelsByCapability(pluginManifest)
	for _, provide := range capabilityManifest.Capabilities.Provides {
		capability, err := loadDescriptorCapability(root, provide)
		if err != nil {
			pluginID := ""
			if pluginManifest != nil {
				pluginID = pluginManifest.ID
			}
			capability, err = loadExposureCapability(pluginID, provide, exposureByCapability[provide.ID])
			if err != nil {
				return nil, err
			}
		}
		catalog.Capabilities = append(catalog.Capabilities, capability)
	}
	return catalog, nil
}

func exposureChannelsByCapability(manifest *plugin_mgr.Manifest) map[string][]plugin_mgr.ExposureChannel {
	out := map[string][]plugin_mgr.ExposureChannel{}
	if manifest == nil {
		return out
	}
	for _, channel := range manifest.Exposure.Channels {
		capabilityID := strings.TrimSpace(channel.Capability)
		if capabilityID == "" {
			continue
		}
		out[capabilityID] = append(out[capabilityID], channel)
	}
	return out
}

func loadExposureCapability(pluginID string, provide capabilityManifestProvide, channels []plugin_mgr.ExposureChannel) (catalogCapability, error) {
	capabilityID := strings.TrimSpace(provide.ID)
	if capabilityID == "" {
		return catalogCapability{}, fmt.Errorf("capability id missing")
	}
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return catalogCapability{}, fmt.Errorf("plugin id missing")
	}
	if len(channels) == 0 {
		return catalogCapability{}, fmt.Errorf("capability descriptor %s missing and no exposure channel fallback found", strings.TrimSpace(provide.Descriptor))
	}
	protocols := make([]models.ProtocolBinding, 0, len(channels))
	permissionCodes := make([]string, 0, len(channels))
	for _, channel := range channels {
		protocols = append(protocols, models.ProtocolBinding{
			Channel:  strings.TrimSpace(channel.Type),
			Endpoint: strings.TrimSpace(channel.Entrypoint),
			Method:   strings.TrimSpace(channel.Method),
			AuthType: strings.TrimSpace(channel.Auth),
		})
		if code := permissionCodeFromExposure(pluginID, channel); code != "" {
			permissionCodes = append(permissionCodes, code)
		}
	}
	annotations := mustRawJSON(map[string]any{
		"descriptor":         strings.TrimSpace(provide.Descriptor),
		"version":            strings.TrimSpace(provide.Version),
		"permission_codes":   dedupeSortedStrings(permissionCodes),
		"agent_usable":       true,
		"risk_level":         "unknown",
		"source":             "exposure",
		"descriptor_missing": true,
	})
	return catalogCapability{
		ID:          capabilityID,
		Version:     strings.TrimSpace(provide.Version),
		Title:       capabilityID,
		Description: "Capability derived from plugin exposure mapping",
		ToolScope:   dedupeSortedStrings(permissionCodes),
		Protocols:   protocols,
		Annotations: annotations,
		Status:      "published",
	}, nil
}

func permissionCodeFromExposure(pluginID string, channel plugin_mgr.ExposureChannel) string {
	securityCode := strings.TrimSpace(fmt.Sprint(channel.Security["permission_code"]))
	if securityCode != "" && securityCode != "<nil>" {
		return securityCode
	}
	resource, action, ok := strings.Cut(strings.TrimSpace(channel.RBAC), ":")
	if !ok {
		return ""
	}
	resource = strings.TrimSpace(resource)
	action = strings.TrimSpace(action)
	if resource == "" || action == "" {
		return ""
	}
	if pluginID == "" {
		return ""
	}
	return pluginID + "." + resource + ":" + action
}

func dedupeSortedStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func loadDescriptorCapability(root string, provide capabilityManifestProvide) (catalogCapability, error) {
	descriptor := strings.TrimSpace(provide.Descriptor)
	if descriptor == "" {
		return catalogCapability{}, fmt.Errorf("capability %s descriptor missing", provide.ID)
	}
	descriptorPath := filepath.Join(root, filepath.Clean(descriptor))
	if !strings.HasPrefix(filepath.Clean(descriptorPath), filepath.Clean(root)) {
		return catalogCapability{}, fmt.Errorf("capability descriptor path escapes plugin root: %s", descriptor)
	}
	raw, err := os.ReadFile(descriptorPath)
	if err != nil {
		return catalogCapability{}, fmt.Errorf("read capability descriptor %s: %w", descriptor, err)
	}

	var descriptorDoc struct {
		ID              string            `yaml:"id"`
		Type            string            `yaml:"type"`
		Version         string            `yaml:"version"`
		Title           string            `yaml:"title"`
		TitleI18n       map[string]string `yaml:"title_i18n"`
		Description     string            `yaml:"description"`
		DescriptionI18n map[string]string `yaml:"description_i18n"`
		Status          string            `yaml:"status"`
		Security        struct {
			PermissionCode    string   `yaml:"permission_code"`
			RiskLevel         string   `yaml:"risk_level"`
			DefaultRoleGrants []string `yaml:"default_role_grants"`
			DefaultRoleCodes  []string `yaml:"default_role_codes"`
		} `yaml:"security"`
		DefaultRoleGrants []string `yaml:"default_role_grants"`
		DefaultRoleCodes  []string `yaml:"default_role_codes"`
		Agent             struct {
			Usable    *bool  `yaml:"usable"`
			RiskLevel string `yaml:"risk_level"`
		} `yaml:"agent"`
		RBAC struct {
			Resource          string   `yaml:"resource"`
			Actions           []string `yaml:"actions"`
			DefaultRoleGrants []string `yaml:"default_role_grants"`
			DefaultRoleCodes  []string `yaml:"default_role_codes"`
		} `yaml:"rbac"`
		Metadata struct {
			Protocols map[string]any `yaml:"protocols"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(raw, &descriptorDoc); err != nil {
		return catalogCapability{}, fmt.Errorf("parse capability descriptor %s: %w", descriptor, err)
	}

	capabilityID := strings.TrimSpace(provide.ID)
	if capabilityID == "" {
		capabilityID = strings.TrimSpace(descriptorDoc.ID)
	}
	if capabilityID == "" {
		return catalogCapability{}, fmt.Errorf("capability descriptor %s id missing", descriptor)
	}

	protocols, err := buildProtocolBindings(descriptorDoc.Metadata.Protocols, provide)
	if err != nil {
		return catalogCapability{}, fmt.Errorf("%s protocols: %w", capabilityID, err)
	}

	title := strings.TrimSpace(descriptorDoc.Title)
	if title == "" {
		title = capabilityID
	}
	version := strings.TrimSpace(provide.Version)
	if version == "" {
		version = strings.TrimSpace(descriptorDoc.Version)
	}
	defaultRoleGrants := dedupeSortedStrings(append(append(append(append(append(
		append([]string(nil), descriptorDoc.DefaultRoleGrants...),
		descriptorDoc.DefaultRoleCodes...),
		descriptorDoc.Security.DefaultRoleGrants...),
		descriptorDoc.Security.DefaultRoleCodes...),
		descriptorDoc.RBAC.DefaultRoleGrants...),
		descriptorDoc.RBAC.DefaultRoleCodes...))

	annotations := mustRawJSON(map[string]any{
		"descriptor":          descriptor,
		"type":                strings.TrimSpace(descriptorDoc.Type),
		"version":             version,
		"permission_code":     strings.TrimSpace(descriptorDoc.Security.PermissionCode),
		"default_role_grants": defaultRoleGrants,
		"agent_usable":        descriptorAgentUsable(descriptorDoc.Agent.Usable),
		"risk_level":          firstNonEmptyDescriptorString(descriptorDoc.Agent.RiskLevel, descriptorDoc.Security.RiskLevel),
		"title_i18n":          cleanLocaleTextMap(descriptorDoc.TitleI18n),
		"description_i18n":    cleanLocaleTextMap(descriptorDoc.DescriptionI18n),
		"rbac": map[string]any{
			"resource": strings.TrimSpace(descriptorDoc.RBAC.Resource),
			"actions":  descriptorDoc.RBAC.Actions,
		},
		"security": map[string]any{
			"permission_code": strings.TrimSpace(descriptorDoc.Security.PermissionCode),
		},
		"agent": map[string]any{
			"usable":     descriptorAgentUsable(descriptorDoc.Agent.Usable),
			"risk_level": firstNonEmptyDescriptorString(descriptorDoc.Agent.RiskLevel, descriptorDoc.Security.RiskLevel),
		},
	})

	return catalogCapability{
		ID:                capabilityID,
		Type:              strings.TrimSpace(descriptorDoc.Type),
		Version:           version,
		Title:             title,
		TitleI18n:         cleanLocaleTextMap(descriptorDoc.TitleI18n),
		Description:       strings.TrimSpace(descriptorDoc.Description),
		DescriptionI18n:   cleanLocaleTextMap(descriptorDoc.DescriptionI18n),
		ToolScope:         append([]string(nil), descriptorDoc.RBAC.Actions...),
		DefaultRoleGrants: defaultRoleGrants,
		Protocols:         protocols,
		Annotations:       annotations,
		Status:            strings.TrimSpace(descriptorDoc.Status),
	}, nil
}

func descriptorAgentUsable(value *bool) bool {
	if value == nil {
		return true
	}
	return *value
}

func firstNonEmptyDescriptorString(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}

func buildProtocolBindings(protocols map[string]any, provide capabilityManifestProvide) ([]models.ProtocolBinding, error) {
	if len(protocols) == 0 {
		return nil, errors.New("metadata.protocols missing")
	}
	bindings := make([]models.ProtocolBinding, 0)
	for channel, raw := range protocols {
		channel = strings.TrimSpace(channel)
		if channel == "" {
			continue
		}
		items, err := normalizeProtocolItems(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", channel, err)
		}
		for _, item := range items {
			binding := models.ProtocolBinding{
				Channel:       channel,
				Endpoint:      strings.TrimSpace(readString(item, "path", "endpoint")),
				Method:        strings.TrimSpace(readString(item, "method")),
				RPC:           strings.TrimSpace(readString(item, "rpc")),
				ToolRef:       strings.TrimSpace(readString(item, "tool_ref", "toolRef")),
				ToolScope:     strings.TrimSpace(readString(item, "tool_scope", "toolScope")),
				AuthType:      strings.TrimSpace(readString(item, "auth_type", "authType")),
				ActorContext:  strings.TrimSpace(readString(item, "actor_context", "actorContext")),
				ResourceScope: strings.TrimSpace(readString(item, "resource_scope", "resourceScope")),
				STSDirect:     readBool(item, "sts_direct", "stsDirect"),
			}
			if schemaRef := schemaRefForChannel(channel, provide); schemaRef != "" {
				binding.SchemaRef = schemaRef
			}
			bindings = append(bindings, binding)
		}
	}
	if len(bindings) == 0 {
		return nil, errors.New("metadata.protocols has no bindings")
	}
	return bindings, nil
}

func normalizeProtocolItems(raw any) ([]map[string]any, error) {
	switch value := raw.(type) {
	case []any:
		items := make([]map[string]any, 0, len(value))
		for _, entry := range value {
			item, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("binding must be object")
			}
			items = append(items, item)
		}
		return items, nil
	case map[string]any:
		return []map[string]any{value}, nil
	default:
		return nil, fmt.Errorf("binding must be object or array")
	}
}

func readString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func readBool(item map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := item[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v
		case string:
			switch strings.ToLower(strings.TrimSpace(v)) {
			case "1", "true", "yes", "y", "on":
				return true
			case "0", "false", "no", "n", "off":
				return false
			}
		}
	}
	return false
}

func schemaRefForChannel(channel string, provide capabilityManifestProvide) string {
	if !strings.EqualFold(channel, "rest") {
		return ""
	}
	if schema := strings.TrimSpace(provide.Schemas.Input); schema != "" {
		return schema
	}
	if schema := strings.TrimSpace(provide.Schemas.Output); schema != "" {
		return schema
	}
	return ""
}

func mustRawJSON(value interface{}) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage([]byte("{}"))
	}
	return json.RawMessage(raw)
}

type capabilityCatalog struct {
	Plugin       pluginMetadata      `json:"plugin"`
	Capabilities []catalogCapability `json:"capabilities"`
}

type capabilityManifestCatalog struct {
	Capabilities struct {
		Provides []capabilityManifestProvide `yaml:"provides"`
	} `yaml:"capabilities"`
}

type capabilityManifestProvide struct {
	ID         string                          `yaml:"id"`
	Version    string                          `yaml:"version"`
	Descriptor string                          `yaml:"descriptor"`
	Schemas    capabilityManifestProvideSchema `yaml:"schemas"`
}

type capabilityManifestProvideSchema struct {
	Input  string `yaml:"input"`
	Output string `yaml:"output"`
}

type pluginMetadata struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type catalogCapability struct {
	ID                string                    `json:"id"`
	CapabilityIDAlias string                    `json:"capability_id"`
	Type              string                    `json:"-"`
	Version           string                    `json:"-"`
	Title             string                    `json:"title"`
	TitleI18n         map[string]string         `json:"title_i18n"`
	Description       string                    `json:"description"`
	DescriptionI18n   map[string]string         `json:"description_i18n"`
	PermissionCode    string                    `json:"permission_code"`
	AgentUsable       *bool                     `json:"agent_usable"`
	RiskLevel         string                    `json:"risk_level"`
	DefaultRoleGrants []string                  `json:"default_role_grants"`
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

func (c catalogCapability) normalizedAnnotations() json.RawMessage {
	annotations := map[string]any{}
	if len(c.Annotations) > 0 && string(c.Annotations) != "null" {
		_ = json.Unmarshal(c.Annotations, &annotations)
	}
	if code := strings.TrimSpace(c.PermissionCode); code != "" {
		annotations["permission_code"] = code
	}
	if c.AgentUsable != nil {
		annotations["agent_usable"] = *c.AgentUsable
	}
	if risk := strings.TrimSpace(c.RiskLevel); risk != "" {
		annotations["risk_level"] = risk
	}
	if defaultRoleGrants := dedupeSortedStrings(c.DefaultRoleGrants); len(defaultRoleGrants) > 0 {
		annotations["default_role_grants"] = defaultRoleGrants
	}
	if titleI18n := cleanLocaleTextMap(c.TitleI18n); len(titleI18n) > 0 {
		annotations["title_i18n"] = titleI18n
	}
	if descriptionI18n := cleanLocaleTextMap(c.DescriptionI18n); len(descriptionI18n) > 0 {
		annotations["description_i18n"] = descriptionI18n
	}
	return mustRawJSON(annotations)
}

func (c catalogCapability) hashSource() interface{} {
	return struct {
		ID              string
		Title           string
		TitleI18n       map[string]string
		DescriptionI18n map[string]string
		Intents         []string
		ToolScope       []string
		Protocols       []models.ProtocolBinding
		Policy          json.RawMessage
	}{
		ID:              c.capabilityID(),
		Title:           c.Title,
		TitleI18n:       cleanLocaleTextMap(c.TitleI18n),
		DescriptionI18n: cleanLocaleTextMap(c.DescriptionI18n),
		Intents:         c.Intents,
		ToolScope:       c.ToolScope,
		Protocols:       c.Protocols,
		Policy:          c.Policy,
	}
}

func cleanLocaleTextMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for locale, value := range in {
		locale = strings.TrimSpace(locale)
		value = strings.TrimSpace(value)
		if locale == "" || value == "" {
			continue
		}
		out[locale] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
