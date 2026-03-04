package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	knowledgeRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

type SourceProvider string

const (
	SourceProviderNotion SourceProvider = "notion"
	SourceProviderFeishu SourceProvider = "feishu"
)

type SourceCredentialInput struct {
	TenantUUID string
	Provider   SourceProvider
	AuthType   string // oauth|token
	Label      string
	BaseURL    string
	Token      string
	CreatedBy  string
}

type SourceConnectorInput struct {
	TenantUUID      string
	Provider        SourceProvider
	CredentialUUID  uuid.UUID
	Config          map[string]any
	CreatedBy       string
	WebhookKeyRef   string
	WebhookKeyPlain string
}

type UpdateConnectorInput struct {
	CredentialUUID *uuid.UUID
	Config         map[string]any
	Status         string // active|paused
	UpdatedBy      string
}

type SpaceSyncJobInput struct {
	TenantUUID     string
	SpaceUUID      uuid.UUID
	Provider       SourceProvider
	ConnectorUUID  uuid.UUID
	SyncMode       string
	Schedule       string
	Scope          map[string]any
	CreatedBy      string
	IngestionLabel string
}

// SourceSyncService manages tenant credentials, connector instances and space sync jobs.
// It is intentionally minimal: secrets are stored in metadata for dev-only use; production should use a secret store.
type SourceSyncService struct {
	db        *gorm.DB
	ingestion *IngestionService

	notion *NotionConnector
	feishu *FeishuConnector
}

type SourceSyncServiceOptions struct {
	DB        *gorm.DB
	Ingestion *IngestionService
	Client    *http.Client
}

func NewSourceSyncService(opts SourceSyncServiceOptions) *SourceSyncService {
	if opts.DB == nil {
		panic("source sync service requires db")
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &SourceSyncService{
		db:        opts.DB,
		ingestion: opts.Ingestion,
		notion:    NewNotionConnector(client),
		feishu:    NewFeishuConnector(client),
	}
}

func (s *SourceSyncService) CreateCredential(ctx context.Context, in SourceCredentialInput) (*models.SourceCredential, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(in.TenantUUID))
	if tenantUUID == "" || strings.TrimSpace(in.Label) == "" {
		return nil, ErrInvalidInput
	}
	provider := normalizeProvider(in.Provider)
	if provider == "" {
		return nil, ErrInvalidInput
	}
	authType := strings.ToLower(strings.TrimSpace(in.AuthType))
	if authType == "" {
		authType = "token"
	}
	if authType != "token" && authType != "oauth" {
		return nil, ErrInvalidInput
	}
	meta := map[string]any{
		"base_url": strings.TrimSpace(in.BaseURL),
	}
	if strings.TrimSpace(in.Token) != "" {
		// Dev-only: token stored in metadata; production should store in secret store and reference via SecretRef.
		meta["token"] = strings.TrimSpace(in.Token)
		meta["masked_hint"] = maskHint(in.Token)
	}
	raw, _ := json.Marshal(meta)
	row := &models.SourceCredential{
		TenantUUID: tenantUUID,
		Provider:   string(provider),
		AuthType:   authType,
		Label:      strings.TrimSpace(in.Label),
		Status:     "active",
		Metadata:   datatypes.JSON(raw),
		CreatedBy:  strings.TrimSpace(in.CreatedBy),
		UpdatedBy:  strings.TrimSpace(in.CreatedBy),
	}
	row.Normalize()
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *SourceSyncService) ListCredentials(ctx context.Context, tenantUUID string, provider SourceProvider, limit int) ([]models.SourceCredential, error) {
	repo := knowledgeRepo.NewSourceCredentialRepository(s.db)
	return repo.ListByTenant(ctx, tenantUUID, string(normalizeProvider(provider)), limit)
}

func (s *SourceSyncService) CreateConnector(ctx context.Context, in SourceConnectorInput) (*models.SourceConnectorInstance, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(in.TenantUUID))
	if tenantUUID == "" || in.CredentialUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	provider := normalizeProvider(in.Provider)
	if provider == "" {
		return nil, ErrInvalidInput
	}

	cred, err := knowledgeRepo.NewSourceCredentialRepository(s.db).FindByUUID(ctx, in.CredentialUUID)
	if err != nil {
		return nil, err
	}
	if cred == nil || strings.ToLower(strings.TrimSpace(cred.TenantUUID)) != tenantUUID {
		return nil, ErrInvalidInput
	}

	cfgRaw := datatypes.JSON([]byte(`{}`))
	if in.Config != nil {
		if raw, err := json.Marshal(in.Config); err == nil {
			cfgRaw = datatypes.JSON(raw)
		}
	}
	row := &models.SourceConnectorInstance{
		TenantUUID:      tenantUUID,
		Provider:        string(provider),
		CredentialUUID:  in.CredentialUUID.String(),
		Status:          "active",
		Config:          cfgRaw,
		LastError:       "",
		CreatedBy:       strings.TrimSpace(in.CreatedBy),
		UpdatedBy:       strings.TrimSpace(in.CreatedBy),
	}
	row.Normalize()
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *SourceSyncService) ListConnectors(ctx context.Context, tenantUUID string, provider SourceProvider, limit int) ([]models.SourceConnectorInstance, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" {
		return nil, ErrInvalidInput
	}
	repo := knowledgeRepo.NewSourceConnectorInstanceRepository(s.db)
	return repo.ListByTenant(ctx, tenantUUID, string(normalizeProvider(provider)), limit)
}

func (s *SourceSyncService) GetConnectorForTenant(ctx context.Context, tenantUUID string, connectorID uuid.UUID) (*models.SourceConnectorInstance, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || connectorID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	conn, err := knowledgeRepo.NewSourceConnectorInstanceRepository(s.db).FindByUUID(ctx, connectorID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, nil
	}
	if strings.ToLower(strings.TrimSpace(conn.TenantUUID)) != tenantUUID {
		return nil, ErrInvalidInput
	}
	return conn, nil
}

func (s *SourceSyncService) UpdateConnector(ctx context.Context, tenantUUID string, connectorID uuid.UUID, in UpdateConnectorInput) (*models.SourceConnectorInstance, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || connectorID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	conn, err := s.GetConnectorForTenant(ctx, tenantUUID, connectorID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, nil
	}

	updates := map[string]any{
		"updated_at": time.Now().UTC(),
		"updated_by": strings.TrimSpace(in.UpdatedBy),
	}

	if in.CredentialUUID != nil && *in.CredentialUUID != uuid.Nil {
		cred, err := knowledgeRepo.NewSourceCredentialRepository(s.db).FindByUUID(ctx, *in.CredentialUUID)
		if err != nil {
			return nil, err
		}
		if cred == nil || strings.ToLower(strings.TrimSpace(cred.TenantUUID)) != tenantUUID {
			return nil, ErrInvalidInput
		}
		conn.CredentialUUID = in.CredentialUUID.String()
		updates["credential_uuid"] = conn.CredentialUUID
	}

	if in.Config != nil {
		raw, err := json.Marshal(in.Config)
		if err != nil {
			return nil, err
		}
		conn.Config = datatypes.JSON(raw)
		updates["config"] = conn.Config
	}

	if strings.TrimSpace(in.Status) != "" {
		st := strings.ToLower(strings.TrimSpace(in.Status))
		if st != "active" && st != "paused" {
			return nil, ErrInvalidInput
		}
		conn.Status = st
		updates["status"] = conn.Status
	}

	conn.Normalize()
	if err := s.db.WithContext(ctx).Model(&models.SourceConnectorInstance{}).
		Where("uuid = ?", connectorID).
		Updates(updates).Error; err != nil {
		return nil, err
	}
	return conn, nil
}

func (s *SourceSyncService) PauseConnector(ctx context.Context, tenantUUID string, connectorID uuid.UUID, reason string, requestedBy string) (*models.SourceConnectorInstance, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || connectorID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repo := knowledgeRepo.NewSourceConnectorInstanceRepository(s.db)
	conn, err := repo.FindByUUID(ctx, connectorID)
	if err != nil {
		return nil, err
	}
	if conn == nil || strings.ToLower(strings.TrimSpace(conn.TenantUUID)) != tenantUUID {
		return nil, ErrInvalidInput
	}
	conn.Status = "paused"
	conn.LastError = strings.TrimSpace(reason)
	conn.UpdatedBy = strings.TrimSpace(requestedBy)
	conn.Normalize()
	if err := s.db.WithContext(ctx).Model(&models.SourceConnectorInstance{}).
		Where("uuid = ?", connectorID).
		Updates(map[string]any{
			"status":     conn.Status,
			"last_error": conn.LastError,
			"updated_by": conn.UpdatedBy,
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
		return nil, err
	}
	return conn, nil
}

func (s *SourceSyncService) CreateSyncJob(ctx context.Context, in SpaceSyncJobInput) (*models.SpaceSyncJob, error) {
	tenantUUID := strings.ToLower(strings.TrimSpace(in.TenantUUID))
	if tenantUUID == "" || in.SpaceUUID == uuid.Nil || in.ConnectorUUID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	provider := normalizeProvider(in.Provider)
	if provider == "" {
		return nil, ErrInvalidInput
	}
	schedule := strings.TrimSpace(in.Schedule)
	if schedule == "" {
		schedule = "@hourly"
	}
	syncMode := strings.ToLower(strings.TrimSpace(in.SyncMode))
	if syncMode == "" {
		syncMode = "full_then_incremental"
	}
	scope := in.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	scope["provider"] = string(provider)
	raw, _ := json.Marshal(scope)

	space, err := knowledgeRepo.NewKnowledgeSpaceRepository(s.db).FindByUUID(ctx, in.SpaceUUID)
	if err != nil {
		return nil, err
	}
	if space == nil || strings.ToLower(strings.TrimSpace(space.TenantUUID)) != tenantUUID {
		return nil, ErrSpaceNotFound
	}

	row := &models.SpaceSyncJob{
		TenantUUID:     tenantUUID,
		SpaceUUID:      in.SpaceUUID.String(),
		ConnectorUUID:  in.ConnectorUUID.String(),
		Provider:       string(provider),
		SyncMode:       syncMode,
		Schedule:       schedule,
		Status:         "active",
		Scope:          datatypes.JSON(raw),
		LastError:      "",
		LastRunRef:     "",
		CreatedBy:      strings.TrimSpace(in.CreatedBy),
		UpdatedBy:      strings.TrimSpace(in.CreatedBy),
	}
	row.Normalize()
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *SourceSyncService) ListSyncJobs(ctx context.Context, space uuid.UUID, limit int) ([]models.SpaceSyncJob, error) {
	repo := knowledgeRepo.NewSpaceSyncJobRepository(s.db)
	return repo.ListBySpace(ctx, space, limit)
}

func (s *SourceSyncService) ListSyncJobsForTenant(ctx context.Context, tenantUUID string, space uuid.UUID, limit int) ([]models.SpaceSyncJob, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || space == uuid.Nil {
		return nil, ErrInvalidInput
	}
	spaceRow, err := knowledgeRepo.NewKnowledgeSpaceRepository(s.db).FindByUUID(ctx, space)
	if err != nil {
		return nil, err
	}
	if spaceRow == nil || strings.ToLower(strings.TrimSpace(spaceRow.TenantUUID)) != tenantUUID {
		return nil, ErrSpaceNotFound
	}
	return s.ListSyncJobs(ctx, space, limit)
}

func (s *SourceSyncService) GetSyncJobForTenant(ctx context.Context, tenantUUID string, space uuid.UUID, jobID uuid.UUID) (*models.SpaceSyncJob, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || space == uuid.Nil || jobID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	job, err := knowledgeRepo.NewSpaceSyncJobRepository(s.db).FindByUUID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, nil
	}
	if strings.ToLower(strings.TrimSpace(job.TenantUUID)) != tenantUUID || strings.TrimSpace(job.SpaceUUID) != space.String() {
		return nil, ErrInvalidInput
	}
	return job, nil
}

func (s *SourceSyncService) PauseSyncJob(ctx context.Context, tenantUUID string, space uuid.UUID, jobID uuid.UUID, reason string, requestedBy string) (*models.SpaceSyncJob, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || space == uuid.Nil || jobID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	repo := knowledgeRepo.NewSpaceSyncJobRepository(s.db)
	job, err := repo.FindByUUID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil || strings.ToLower(strings.TrimSpace(job.TenantUUID)) != tenantUUID || strings.TrimSpace(job.SpaceUUID) != space.String() {
		return nil, ErrInvalidInput
	}
	job.Status = "paused"
	job.LastError = strings.TrimSpace(reason)
	job.UpdatedBy = strings.TrimSpace(requestedBy)
	job.Normalize()
	if err := s.db.WithContext(ctx).Model(&models.SpaceSyncJob{}).
		Where("uuid = ?", jobID).
		Updates(map[string]any{
			"status":     job.Status,
			"last_error": job.LastError,
			"updated_by": job.UpdatedBy,
			"updated_at": time.Now().UTC(),
		}).Error; err != nil {
		return nil, err
	}
	return job, nil
}

type RunSyncJobResult struct {
	SyncJobID     string `json:"sync_job_id"`
	IngestionJob  string `json:"ingestion_job_id,omitempty"`
	HasMore       bool   `json:"has_more"`
	NextCursor    string `json:"next_cursor,omitempty"`
	Provider      string `json:"provider"`
	DocumentCount int    `json:"document_count"`
}

func (s *SourceSyncService) RunSyncJob(ctx context.Context, tenantUUID string, space uuid.UUID, jobID uuid.UUID, requestedBy string) (*RunSyncJobResult, error) {
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	if tenantUUID == "" || space == uuid.Nil || jobID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if s.ingestion == nil {
		return nil, errors.New("ingestion service unavailable")
	}

	jobRepo := knowledgeRepo.NewSpaceSyncJobRepository(s.db)
	job, err := jobRepo.FindByUUID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job == nil || strings.ToLower(strings.TrimSpace(job.TenantUUID)) != tenantUUID || strings.TrimSpace(job.SpaceUUID) != space.String() {
		return nil, ErrInvalidInput
	}
	if strings.ToLower(strings.TrimSpace(job.Status)) != "active" {
		return nil, fmt.Errorf("sync job not active: %s", job.Status)
	}

	connRepo := knowledgeRepo.NewSourceConnectorInstanceRepository(s.db)
	connID, _ := uuid.Parse(strings.TrimSpace(job.ConnectorUUID))
	conn, err := connRepo.FindByUUID(ctx, connID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, ErrInvalidInput
	}
	credID, _ := uuid.Parse(strings.TrimSpace(conn.CredentialUUID))
	cred, err := knowledgeRepo.NewSourceCredentialRepository(s.db).FindByUUID(ctx, credID)
	if err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, ErrInvalidInput
	}

	scope := map[string]any{}
	if len(job.Scope) > 0 {
		_ = json.Unmarshal(job.Scope, &scope)
	}
	cursor, _ := scope["cursor"].(string)
	if strings.TrimSpace(cursor) == "" && job.LastOKAt != nil {
		// For incremental mode, prefer time-based filtering when no pagination cursor is present.
		// Connectors may ignore this value if they do not support filtering.
		scope["updated_since"] = job.LastOKAt.UTC().Format(time.RFC3339)
	}

	token := ""
	baseURL := ""
	meta := map[string]any{}
	if len(cred.Metadata) > 0 {
		_ = json.Unmarshal(cred.Metadata, &meta)
		if v, ok := meta["token"].(string); ok {
			token = strings.TrimSpace(v)
		}
		if v, ok := meta["base_url"].(string); ok {
			baseURL = strings.TrimSpace(v)
		}
	}

	req := SourceFetchRequest{
		Provider: string(normalizeProvider(SourceProvider(job.Provider))),
		BaseURL:  baseURL,
		Token:    token,
		Scope:    scope,
		Cursor:   cursor,
		Limit:    50,
	}
	var resp SourceFetchResponse
	switch SourceProvider(req.Provider) {
	case SourceProviderNotion:
		resp, err = s.notion.Fetch(ctx, req)
	case SourceProviderFeishu:
		resp, err = s.feishu.Fetch(ctx, req)
	default:
		err = ErrInvalidInput
	}
	if err != nil {
		_ = s.db.WithContext(ctx).Model(&models.SourceConnectorInstance{}).Where("uuid = ?", connID).Updates(map[string]any{
			"last_error": err.Error(),
			"updated_at": time.Now().UTC(),
		}).Error
		_ = s.db.WithContext(ctx).Model(&models.SpaceSyncJob{}).Where("uuid = ?", jobID).Updates(map[string]any{
			"last_error": err.Error(),
			"updated_at": time.Now().UTC(),
		}).Error
		return nil, err
	}
	_ = s.db.WithContext(ctx).Model(&models.SourceConnectorInstance{}).Where("uuid = ?", connID).Updates(map[string]any{
		"last_error": "",
		"updated_at": time.Now().UTC(),
	}).Error

	sourceURI := fmt.Sprintf("%s://sync/%s", req.Provider, jobID.String())
	ingJob, ingestErr := s.ingestion.TriggerWithDocUnits(ctx, TriggerIngestionInput{
		SpaceID:         space,
		Format:          "api",
		SourceURI:       sourceURI,
		IngestionProfile: "",
		ProcessorProfile: "",
		OCRRequired:      false,
		MaskingProfile:   "",
		Priority:         "normal",
		RequestedBy:      strings.TrimSpace(requestedBy),
	}, resp.Units)

	now := time.Now().UTC()
	updates := map[string]any{
		"last_run_at":  now,
		"updated_at":   now,
		"updated_by":   strings.TrimSpace(requestedBy),
		"last_error":   "",
		"last_run_ref": "",
	}
	if resp.HasMore {
		scope["cursor"] = resp.NextCursor
	} else {
		delete(scope, "cursor")
	}
	if raw, err := json.Marshal(scope); err == nil {
		updates["scope"] = datatypes.JSON(raw)
	}
	if ingJob != nil {
		updates["last_run_ref"] = ingJob.UUID.String()
	}
	if ingestErr == nil && ingJob != nil && strings.ToLower(ingJob.Status) == strings.ToLower(models.IngestionStatusCompleted) {
		updates["last_ok_at"] = now
	}
	if ingestErr != nil {
		updates["last_error"] = ingestErr.Error()
	}
	_ = s.db.WithContext(ctx).Model(&models.SpaceSyncJob{}).Where("uuid = ?", jobID).Updates(updates).Error

	return &RunSyncJobResult{
		SyncJobID:     jobID.String(),
		IngestionJob:  safeJobUUID(ingJob),
		HasMore:       resp.HasMore,
		NextCursor:    resp.NextCursor,
		Provider:      req.Provider,
		DocumentCount: len(resp.Units),
	}, nil
}

func safeJobUUID(job *models.IngestionJob) string {
	if job == nil {
		return ""
	}
	return job.UUID.String()
}

func normalizeProvider(p SourceProvider) SourceProvider {
	switch strings.ToLower(strings.TrimSpace(string(p))) {
	case "notion":
		return SourceProviderNotion
	case "feishu":
		return SourceProviderFeishu
	default:
		return ""
	}
}

func maskHint(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 6 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

func tenantUUIDFromContext(ctx context.Context) string {
	val := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if val != "" {
		return strings.ToLower(val)
	}
	return ""
}
