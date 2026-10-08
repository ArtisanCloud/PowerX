package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const (
	KnowledgeReasonInvalidArgument    = "KNOWLEDGE_INVALID_ARGUMENT"
	KnowledgeReasonUnauthorized       = "KNOWLEDGE_UNAUTHORIZED"
	KnowledgeReasonForbidden          = "KNOWLEDGE_FORBIDDEN"
	KnowledgeReasonSpaceNotFound      = "KNOWLEDGE_SPACE_NOT_FOUND"
	KnowledgeReasonDocumentNotFound   = "KNOWLEDGE_DOCUMENT_NOT_FOUND"
	KnowledgeReasonIndexJobNotFound   = "KNOWLEDGE_INDEX_JOB_NOT_FOUND"
	KnowledgeReasonIndexConflict      = "KNOWLEDGE_INDEX_CONFLICT"
	KnowledgeReasonUpstreamDependency = "KNOWLEDGE_UPSTREAM_DEPENDENCY"

	HostIndexStatusQueued    = "queued"
	HostIndexStatusRunning   = "running"
	HostIndexStatusSucceeded = "succeeded"
	HostIndexStatusFailed    = "failed"

	HostDocumentStatusQueued  = "queued"
	HostDocumentStatusIndexed = "indexed"
	HostDocumentStatusDeleted = "deleted"

	HostIndexOperationUpsert  = "upsert"
	HostIndexOperationDelete  = "delete"
	HostIndexOperationRebuild = "rebuild"
)

// HostContractService owns tenant-scoped documents and their asynchronous
// indexing records. It never accepts a caller-provided tenant UUID.
type HostContractService struct {
	db       *gorm.DB
	wake     chan struct{}
	semantic *SemanticRuntime
}

func (s *HostContractService) WithSemantic(runtime *SemanticRuntime) *HostContractService {
	s.semantic = runtime
	return s
}

func (s *HostContractService) Semantic() *SemanticRuntime { return s.semantic }

func NewHostContractService(db *gorm.DB) *HostContractService {
	return &HostContractService{db: db, wake: make(chan struct{}, 1)}
}

type HostSpace struct {
	SpaceUUID string `json:"space_uuid"`
	Name      string `json:"name"`
	Status    string `json:"status"`
}

type HostDocumentInput struct {
	Indexing       *SemanticIndexingSettings `json:"indexing,omitempty"`
	Artifacts      []SemanticArtifactInput   `json:"artifacts,omitempty"`
	ExternalRef    *SemanticExternalRef      `json:"external_ref,omitempty"`
	IdempotencyKey string                    `json:"idempotency_key,omitempty"`
	Title          string                    `json:"title"`
	URI            string                    `json:"uri"`
	Content        string                    `json:"content"`
	ContentType    string                    `json:"content_type"`
	Checksum       string                    `json:"checksum"`
	Version        string                    `json:"version"`
	Tags           []string                  `json:"tags"`
	Ingestion      *HostIngestionSettings    `json:"ingestion,omitempty"`
}

type HostDocumentJob struct {
	JobUUID      string `json:"job_uuid"`
	Status       string `json:"status"`
	Operation    string `json:"operation"`
	DocumentUUID string `json:"document_uuid,omitempty"`
}

type HostJob struct {
	RebuildMode        string                  `json:"rebuild_mode,omitempty"`
	DocumentCount      int                     `json:"document_count"`
	ProcessedDocuments int                     `json:"processed_documents"`
	DocumentConfigs    []HostDocumentJobConfig `json:"document_configs,omitempty"`
	JobUUID            string                  `json:"job_uuid"`
	SpaceUUID          string                  `json:"space_uuid"`
	DocumentUUID       string                  `json:"document_uuid,omitempty"`
	Operation          string                  `json:"operation"`
	Status             string                  `json:"status"`
	ErrorCode          string                  `json:"error_code,omitempty"`
	Priority           string                  `json:"priority"`
	TraceID            string                  `json:"trace_id"`
	EffectiveConfig    *HostIngestionSnapshot  `json:"effective_config,omitempty"`
	RequestedConfig    *HostIngestionSettings  `json:"requested_config,omitempty"`
	SnapshotChecksum   string                  `json:"snapshot_checksum,omitempty"`
	ArtifactCount      int                     `json:"artifact_count"`
	VectorCount        int                     `json:"vector_count"`
	BundleGeneration   string                  `json:"bundle_generation"`
	ChunkCount         int                     `json:"chunk_count"`
	StartedAt          *time.Time              `json:"started_at,omitempty"`
	CompletedAt        *time.Time              `json:"completed_at,omitempty"`
}

type HostSearchCitation struct {
	SpaceUUID    string   `json:"space_uuid"`
	DocumentUUID string   `json:"document_uuid"`
	Title        string   `json:"title"`
	URI          string   `json:"uri"`
	Excerpt      string   `json:"excerpt"`
	Tags         []string `json:"tags"`
}

func (s *HostContractService) ListSpaces(ctx context.Context, tenantUUID string) ([]HostSpace, error) {
	if s == nil || s.db == nil {
		return nil, knowledgeError(http.StatusServiceUnavailable, KnowledgeReasonUpstreamDependency, errors.New("knowledge store unavailable"))
	}
	var rows []models.KnowledgeSpace
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ?", tenantUUID).Order("space_name ASC").Find(&rows).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := make([]HostSpace, 0, len(rows))
	for _, row := range rows {
		out = append(out, HostSpace{SpaceUUID: row.UUID.String(), Name: row.SpaceName, Status: row.Status})
	}
	return out, nil
}

func (s *HostContractService) Search(ctx context.Context, tenantUUID, query string, spaceUUIDs []string, limit int) ([]HostSearchCitation, error) {
	if s == nil || s.db == nil {
		return nil, KnowledgeUpstreamDependencyError(errors.New("knowledge store unavailable"))
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, KnowledgeInvalidArgumentError(errors.New("query is required"))
	}
	if limit <= 0 || limit > 100 {
		return nil, KnowledgeInvalidArgumentError(errors.New("invalid limit"))
	}
	parsedSpaces, err := parseUniqueUUIDs(spaceUUIDs)
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	if len(parsedSpaces) > 0 {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.KnowledgeSpace{}).Where("tenant_uuid = ? AND uuid IN ?", tenantUUID, parsedSpaces).Count(&count).Error; err != nil {
			return nil, KnowledgeUpstreamDependencyError(err)
		}
		if count != int64(len(parsedSpaces)) {
			return nil, KnowledgeSpaceNotFoundError(errors.New("knowledge space not found"))
		}
	}
	var matches []struct {
		DocumentUUID string
		Title        string
		URI          string
		SpaceUUID    string
		Content      string
		Tags         datatypes.JSON
		Metadata     datatypes.JSON
	}
	chunkTable := models.HostDocumentChunk{}.TableName()
	docTable := models.TenantDocument{}.TableName()
	q := s.db.WithContext(ctx).Table(chunkTable+" AS c").Select("c.document_uuid,d.title,d.uri,c.space_uuid,c.content,d.tags,c.metadata").Joins("JOIN "+docTable+" AS d ON d.uuid = c.document_uuid AND d.tenant_uuid = c.tenant_uuid AND d.active_index_job_uuid = c.job_uuid").Joins("JOIN "+models.IndexJob{}.TableName()+" AS j ON j.uuid = c.job_uuid AND j.tenant_uuid = c.tenant_uuid AND j.status = ?", HostIndexStatusSucceeded).Where("c.tenant_uuid = ? AND d.queryable = TRUE AND d.index_status = ? AND d.deleted_at IS NULL AND c.deleted_at IS NULL AND LOWER(c.content) LIKE ?", tenantUUID, HostDocumentStatusIndexed, "%"+strings.ToLower(query)+"%")
	if len(parsedSpaces) > 0 {
		q = q.Where("c.space_uuid IN ?", parsedSpaces)
	}
	if err := q.Order("c.created_at DESC").Limit(limit).Scan(&matches).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := []HostSearchCitation{}
	seen := map[string]bool{}
	for _, row := range matches {
		if seen[row.DocumentUUID] {
			continue
		}
		seen[row.DocumentUUID] = true
		var meta map[string]json.RawMessage
		if json.Unmarshal(row.Metadata, &meta) == nil {
			if title := meta["source_title"]; len(title) > 0 {
				_ = json.Unmarshal(title, &row.Title)
			}
			if uri := meta["source_uri"]; len(uri) > 0 {
				_ = json.Unmarshal(uri, &row.URI)
			}
			if tags := meta["source_tags"]; len(tags) > 0 {
				row.Tags = datatypes.JSON(tags)
			}
		}
		out = append(out, HostSearchCitation{SpaceUUID: row.SpaceUUID, DocumentUUID: row.DocumentUUID, Title: row.Title, URI: row.URI, Excerpt: searchExcerpt(row.Content, query), Tags: decodeTags(row.Tags)})
	}

	return out, nil
}

func (s *HostContractService) UpsertDocument(ctx context.Context, tenantUUID, spaceUUID string, in HostDocumentInput) (HostDocumentJob, error) {
	if s == nil || s.db == nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(errors.New("knowledge store unavailable"))
	}
	in = canonicalSemanticDocument(in)
	if err := validateSemanticArtifacts(in); err != nil {
		return HostDocumentJob{}, err
	}
	if err := validateDocumentInput(in); err != nil {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(err)
	}
	in.ContentType = strings.ToLower(strings.TrimSpace(in.ContentType))
	space, err := s.requireSpace(ctx, tenantUUID, spaceUUID)
	if err != nil {
		return HostDocumentJob{}, err
	}
	if space.Status == models.KnowledgeSpaceStatusRetired {
		return HostDocumentJob{}, KnowledgeSpaceNotFoundError(errors.New("space retired"))
	}
	requestInput := in
	requestInput.IdempotencyKey = ""
	requestBytes, _ := json.Marshal(requestInput)
	requestHash := checksumJSON(requestBytes)
	// Replays refer to the already accepted immutable request, even if a
	// Profile/model binding has since changed. Authorization still happens first.
	if in.IdempotencyKey != "" {
		if len(in.IdempotencyKey) > 128 || strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
			return HostDocumentJob{}, KnowledgeInvalidArgumentError(errors.New("invalid idempotency key"))
		}
		var existing models.IndexJob
		if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND idempotency_key = ?", tenantUUID, spaceUUID, in.IdempotencyKey).Limit(1).Find(&existing).Error; err != nil {
			return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
		}
		if existing.UUID != uuid.Nil {
			if existing.RequestChecksum != requestHash {
				return HostDocumentJob{}, KnowledgeIndexConflictError(errors.New("idempotency key reused with different request"))
			}
			return acceptedHostJob(existing), nil
		}
	}
	snapshot, err := s.freezeIngestion(ctx, space, in)
	if err != nil {
		return HostDocumentJob{}, err
	}
	if in.Indexing != nil {
		if s.semantic == nil {
			return HostDocumentJob{}, semanticError(501, "KNOWLEDGE_SEMANTIC_UNSUPPORTED")
		}
		snapshot.IndexMode = in.Indexing.Mode
		snapshot.Indexing, err = s.semantic.freeze(ctx, tenantUUID, spaceUUID, *in.Indexing)
		if err != nil {
			return HostDocumentJob{}, err
		}
	}
	snapshotBytes, _ := json.Marshal(snapshot)
	requestedBytes, _ := json.Marshal(in.Ingestion)
	sourceBytes, _ := json.Marshal(hostDocumentSource{Title: strings.TrimSpace(in.Title), URI: strings.TrimSpace(in.URI), Content: in.Content, ContentType: in.ContentType, Checksum: strings.ToLower(in.Checksum), Version: in.Version, Tags: normalizeTags(in.Tags), Artifacts: in.Artifacts, ExternalRef: in.ExternalRef})
	tags, _ := json.Marshal(normalizeTags(in.Tags))
	var document models.TenantDocument
	var job models.IndexJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockHostSpace(ctx, tx, tenantUUID, spaceUUID); err != nil {
			return err
		}
		if in.IdempotencyKey != "" {
			if len(in.IdempotencyKey) > 128 || strings.TrimSpace(in.IdempotencyKey) != in.IdempotencyKey {
				return KnowledgeInvalidArgumentError(errors.New("invalid idempotency key"))
			}
			var existing models.IndexJob
			if e := tx.Where("tenant_uuid = ? AND space_uuid = ? AND idempotency_key = ?", tenantUUID, spaceUUID, in.IdempotencyKey).Limit(1).Find(&existing).Error; e != nil {
				return e
			}
			if existing.UUID != uuid.Nil {
				if existing.RequestChecksum != requestHash {
					return KnowledgeIndexConflictError(errors.New("idempotency key reused with different request"))
				}
				job = existing
				return nil
			}
		}
		if in.ExternalRef != nil && tx.Dialector.Name() == "postgres" {
			var mapped models.TenantDocument
			if err := tx.Where("tenant_uuid = ? AND space_uuid = ? AND semantic_source->'external_ref'->>'document_uuid' = ?", tenantUUID, spaceUUID, in.ExternalRef.DocumentUUID).Limit(1).Find(&mapped).Error; err != nil {
				return err
			}
			if mapped.UUID != uuid.Nil && mapped.URI != strings.TrimSpace(in.URI) {
				return semanticError(409, "KNOWLEDGE_SOURCE_MAPPING_CONFLICT")
			}
		}
		err := tx.Where("tenant_uuid = ? AND space_uuid = ? AND uri = ?", tenantUUID, spaceUUID, strings.TrimSpace(in.URI)).First(&document).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			document = models.TenantDocument{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, Title: strings.TrimSpace(in.Title), URI: strings.TrimSpace(in.URI), Content: in.Content, ContentType: strings.TrimSpace(in.ContentType), Checksum: strings.ToLower(strings.TrimSpace(in.Checksum)), Version: strings.TrimSpace(in.Version), Tags: tags, IndexStatus: HostDocumentStatusQueued, VisibilityEpoch: uuid.NewString()}
			if err := tx.Create(&document).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			var previousSource hostDocumentSource
			if len(document.SemanticSource) > 0 && json.Unmarshal(document.SemanticSource, &previousSource) != nil {
				return semanticError(409, "KNOWLEDGE_SOURCE_MAPPING_CONFLICT")
			}
			if previousSource.ExternalRef != nil && (in.ExternalRef == nil || *previousSource.ExternalRef != *in.ExternalRef) {
				return semanticError(409, "KNOWLEDGE_SOURCE_MAPPING_CONFLICT")
			}
			if len(previousSource.Artifacts) > 0 && in.Indexing == nil {
				return semanticError(422, "KNOWLEDGE_EXPLICIT_INDEXING_REQUIRED")
			}
			if err := ensureNoHostActivity(tx, tenantUUID, spaceUUID, document.UUID.String()); err != nil {
				return err
			}
			if document.Checksum == strings.ToLower(strings.TrimSpace(in.Checksum)) && document.IndexStatus != HostDocumentStatusDeleted {
				var previous models.IndexJob
				if document.IndexJobUUID == nil {
					return KnowledgeIndexConflictError(errors.New("same legacy document checksum is already accepted"))
				}
				if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, *document.IndexJobUUID).First(&previous).Error; err != nil {
					return err
				}
				if previous.SnapshotChecksum == checksumJSON(snapshotBytes) && previous.Status != HostIndexStatusFailed && (in.Indexing == nil || previous.RequestChecksum == requestHash) {
					return KnowledgeIndexConflictError(errors.New("same source and configuration are already accepted"))
				}
			}
			document.Title, document.Content, document.ContentType = strings.TrimSpace(in.Title), in.Content, strings.TrimSpace(in.ContentType)
			document.Checksum, document.Version, document.Tags = strings.ToLower(strings.TrimSpace(in.Checksum)), strings.TrimSpace(in.Version), tags
			if document.ActiveIndexJobUUID == nil {
				document.IndexStatus, document.IndexedAt = HostDocumentStatusQueued, nil
			}
			if err := tx.Save(&document).Error; err != nil {
				return err
			}
		}
		semanticSource, _ := json.Marshal(hostDocumentSource{Artifacts: in.Artifacts, ExternalRef: in.ExternalRef})
		if err := tx.Model(&document).Update("semantic_source", semanticSource).Error; err != nil {
			return err
		}
		docUUID := document.UUID.String()
		if err := ensureNoHostActivity(tx, tenantUUID, spaceUUID, docUUID); err != nil {
			return err
		}
		job = models.IndexJob{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, DocumentUUID: &docUUID, Operation: HostIndexOperationUpsert, Status: HostIndexStatusQueued, Priority: snapshot.Priority, DocumentCount: 1, ConfigSnapshot: snapshotBytes, RequestedConfig: requestedBytes, SourceSnapshot: sourceBytes, SnapshotChecksum: checksumJSON(snapshotBytes), TraceID: reqctx.GetTraceID(ctx)}
		job.RequestChecksum = requestHash
		if in.IdempotencyKey != "" {
			job.IdempotencyKey = &in.IdempotencyKey
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		jobID := job.UUID.String()
		return tx.Model(&document).Update("index_job_uuid", jobID).Error
	})
	if err != nil {
		return HostDocumentJob{}, mapHostDBError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return acceptedHostJob(job), nil
}

func (s *HostContractService) DeleteDocument(ctx context.Context, tenantUUID, spaceUUID, documentUUID string) (HostDocumentJob, error) {
	if _, err := s.requireSpace(ctx, tenantUUID, spaceUUID); err != nil {
		return HostDocumentJob{}, err
	}
	docID, err := uuid.Parse(strings.TrimSpace(documentUUID))
	if err != nil {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(err)
	}
	var document models.TenantDocument
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", tenantUUID, spaceUUID, docID).First(&document).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return HostDocumentJob{}, KnowledgeDocumentNotFoundError(err)
		}
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	docUUID := document.UUID.String()
	job := models.IndexJob{TenantUUID: tenantUUID, SpaceUUID: spaceUUID, DocumentUUID: &docUUID, Operation: HostIndexOperationDelete, Status: HostIndexStatusQueued}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockHostSpace(ctx, tx, tenantUUID, spaceUUID); err != nil {
			return err
		}
		if err := ensureNoHostActivity(tx, tenantUUID, spaceUUID, docUUID); err != nil {
			return err
		}
		if err := tx.Model(&models.SemanticSpaceBinding{}).Where("tenant_uuid = ? AND space_uuid = ?", tenantUUID, spaceUUID).Update("corpus_generation", uuid.NewString()).Error; err != nil {
			return err
		}
		if err := tx.Model(&document).Updates(map[string]any{"index_status": HostDocumentStatusDeleted, "queryable": false, "visibility_epoch": uuid.NewString()}).Error; err != nil {
			return err
		}
		return tx.Create(&job).Error
	}); err != nil {
		return HostDocumentJob{}, KnowledgeUpstreamDependencyError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return HostDocumentJob{JobUUID: job.UUID.String(), Status: job.Status, Operation: job.Operation, DocumentUUID: docUUID}, nil
}

func (s *HostContractService) RebuildIndex(ctx context.Context, tenant, space string) (HostDocumentJob, error) {
	return s.RebuildSpace(ctx, tenant, space, HostRebuildInput{Mode: HostRebuildReuseSnapshot})
}

func (s *HostContractService) GetIndexJob(ctx context.Context, tenantUUID, jobUUID string) (HostJob, error) {
	id, err := uuid.Parse(strings.TrimSpace(jobUUID))
	if err != nil {
		return HostJob{}, KnowledgeInvalidArgumentError(err)
	}
	var job models.IndexJob
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, id).First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return HostJob{}, KnowledgeIndexJobNotFoundError(err)
		}
		return HostJob{}, KnowledgeUpstreamDependencyError(err)
	}
	out := HostJob{JobUUID: job.UUID.String(), SpaceUUID: job.SpaceUUID, Operation: job.Operation, Status: job.Status, ErrorCode: job.ErrorCode, RebuildMode: job.RebuildMode, DocumentCount: job.DocumentCount, ProcessedDocuments: job.ProcessedDocuments, Priority: job.Priority, TraceID: job.TraceID, SnapshotChecksum: job.SnapshotChecksum, ChunkCount: job.ChunkCount, ArtifactCount: job.ArtifactCount, VectorCount: job.VectorCount, BundleGeneration: job.UUID.String(), StartedAt: job.StartedAt, CompletedAt: job.CompletedAt}
	if job.DocumentUUID != nil {
		out.DocumentUUID = *job.DocumentUUID
	}
	if len(job.ConfigSnapshot) > 0 {
		if checksumJSON(job.ConfigSnapshot) != job.SnapshotChecksum {
			out.ErrorCode = KnowledgeReasonSnapshotInvalid
			return out, nil
		}
		if job.Operation == HostIndexOperationRebuild {
			var batch hostRebuildBatch
			if strictJSON(job.ConfigSnapshot, &batch) != nil || batch.Schema != hostRebuildBatchSchema {
				return HostJob{}, knowledgeError(500, KnowledgeReasonSnapshotInvalid, errors.New("invalid rebuild snapshot"))
			}
			out.DocumentConfigs = []HostDocumentJobConfig{}
			for _, item := range batch.Documents {
				out.DocumentConfigs = append(out.DocumentConfigs, HostDocumentJobConfig{item.DocumentUUID, publicHostSnapshot(item.Config)})
			}
			if job.DocumentUUID != nil && len(out.DocumentConfigs) == 1 {
				out.EffectiveConfig = &out.DocumentConfigs[0].EffectiveConfig
			}
		} else {
			var config HostIngestionSnapshot
			if json.Unmarshal(job.ConfigSnapshot, &config) != nil {
				return HostJob{}, knowledgeError(500, KnowledgeReasonSnapshotInvalid, errors.New("invalid job snapshot"))
			}
			public := publicHostSnapshot(config)
			out.EffectiveConfig = &public
		}
	}
	if len(job.RequestedConfig) > 0 && string(job.RequestedConfig) != "null" {
		var config HostIngestionSettings
		if json.Unmarshal(job.RequestedConfig, &config) != nil {
			return HostJob{}, KnowledgeUpstreamDependencyError(errors.New("job requested config invalid"))
		}
		out.RequestedConfig = &config
	}
	if job.DocumentUUID != nil {
		out.DocumentUUID = *job.DocumentUUID
	}
	return out, nil
}

func (s *HostContractService) requireSpace(ctx context.Context, tenantUUID, spaceUUID string) (*models.KnowledgeSpace, error) {
	id, err := uuid.Parse(strings.TrimSpace(spaceUUID))
	if err != nil {
		return nil, KnowledgeInvalidArgumentError(err)
	}
	var space models.KnowledgeSpace
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenantUUID, id).First(&space).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, KnowledgeSpaceNotFoundError(err)
		}
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	return &space, nil
}

func (s *HostContractService) runDocumentJobAsync(_ uuid.UUID) {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func validateDocumentInput(in HostDocumentInput) error {
	if strings.TrimSpace(in.Title) == "" || strings.TrimSpace(in.URI) == "" || strings.TrimSpace(in.Content) == "" || strings.TrimSpace(in.ContentType) == "" || strings.TrimSpace(in.Version) == "" {
		return errors.New("title, uri, content, content_type and version are required")
	}
	if contentType := strings.ToLower(strings.TrimSpace(in.ContentType)); contentType != "text/markdown" && contentType != "text/plain" {
		return errors.New("content_type must be text/markdown or text/plain")
	}
	if len(in.Content) > 8<<20 {
		return errors.New("content exceeds 8 MiB")
	}
	if checksumBytes([]byte(in.Content)) != strings.ToLower(strings.TrimSpace(in.Checksum)) {
		return errors.New("checksum does not match UTF-8 content")
	}
	checksum := strings.TrimSpace(in.Checksum)
	if len(checksum) != 64 {
		return errors.New("checksum must be a SHA-256 hex string")
	}
	for _, r := range checksum {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return errors.New("checksum must be a SHA-256 hex string")
		}
	}
	return nil
}

func parseUniqueUUIDs(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, err
		}
		value := id.String()
		if _, ok := seen[value]; ok {
			return nil, errors.New("duplicate UUID")
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, raw := range tags {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func decodeTags(raw []byte) []string { var tags []string; _ = json.Unmarshal(raw, &tags); return tags }
func searchExcerpt(content, query string) string {
	content = strings.TrimSpace(content)
	lowered := strings.ToLower(content)
	index := strings.Index(lowered, strings.ToLower(strings.TrimSpace(query)))
	if index < 0 {
		if len(content) > 320 {
			return content[:320]
		}
		return content
	}
	start := index - 120
	if start < 0 {
		start = 0
	}
	end := index + len(query) + 200
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

func KnowledgeInvalidArgumentError(err error) error {
	return knowledgeError(http.StatusBadRequest, KnowledgeReasonInvalidArgument, err)
}
func KnowledgeUnauthorizedError(err error) error {
	return knowledgeError(http.StatusUnauthorized, KnowledgeReasonUnauthorized, err)
}
func KnowledgeForbiddenError(err error) error {
	return knowledgeError(http.StatusForbidden, KnowledgeReasonForbidden, err)
}
func KnowledgeSpaceNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonSpaceNotFound, err)
}
func KnowledgeDocumentNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonDocumentNotFound, err)
}
func KnowledgeIndexJobNotFoundError(err error) error {
	return knowledgeError(http.StatusNotFound, KnowledgeReasonIndexJobNotFound, err)
}
func KnowledgeIndexConflictError(err error) error {
	return knowledgeError(http.StatusConflict, KnowledgeReasonIndexConflict, err)
}
func KnowledgeUpstreamDependencyError(err error) error {
	return knowledgeError(http.StatusServiceUnavailable, KnowledgeReasonUpstreamDependency, err)
}
func knowledgeError(status int, code string, err error) error {
	return dto.NewErrorWithCode(status, code, code, err)
}
func mapHostDBError(err error) error {
	if dto.CodeOf(err) != "" {
		return err
	}
	return KnowledgeUpstreamDependencyError(fmt.Errorf("knowledge persistence failed: %w", err))
}
