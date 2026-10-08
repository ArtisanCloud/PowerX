package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	HostRebuildReuseSnapshot = "reuse_snapshot"
	HostRebuildApplyConfig   = "apply_config"
	hostRebuildBatchSchema   = "powerx.knowledge.rebuild-batch/v1"
)

type HostRebuildInput struct {
	Mode           string                    `json:"mode"`
	IdempotencyKey string                    `json:"idempotency_key,omitempty"`
	Ingestion      *HostIngestionSettings    `json:"ingestion,omitempty"`
	Indexing       *SemanticIndexingSettings `json:"indexing,omitempty"`
}
type hostFrozenDocument struct {
	DocumentUUID     string                `json:"document_uuid"`
	BaseIndexJobUUID string                `json:"base_index_job_uuid,omitempty"`
	Config           HostIngestionSnapshot `json:"config"`
	Source           hostDocumentSource    `json:"source"`
}
type hostRebuildBatch struct {
	Schema    string               `json:"schema"`
	Documents []hostFrozenDocument `json:"documents"`
}
type HostDocumentJobConfig struct {
	DocumentUUID    string                `json:"document_uuid"`
	EffectiveConfig HostIngestionSnapshot `json:"effective_config"`
}

func (s *HostContractService) RebuildDocument(ctx context.Context, tenant, space, document string, input HostRebuildInput) (HostDocumentJob, error) {
	id, err := uuid.Parse(document)
	if err != nil || id == uuid.Nil {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(errors.New("document UUID required"))
	}
	return s.acceptRebuild(ctx, tenant, space, id.String(), input)
}
func (s *HostContractService) RebuildSpace(ctx context.Context, tenant, space string, input HostRebuildInput) (HostDocumentJob, error) {
	return s.acceptRebuild(ctx, tenant, space, "", input)
}
func (s *HostContractService) lockHostSpace(ctx context.Context, tx *gorm.DB, tenant, space string) (*models.KnowledgeSpace, error) {
	id, err := uuid.Parse(space)
	if err != nil || id == uuid.Nil {
		return nil, KnowledgeInvalidArgumentError(errors.New("space UUID required"))
	}
	var row models.KnowledgeSpace
	err = tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ? AND status <> ?", tenant, id, models.KnowledgeSpaceStatusRetired).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, KnowledgeSpaceNotFoundError(err)
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func ensureNoHostActivity(tx *gorm.DB, tenant, space, document string) error {
	q := tx.Model(&models.IndexJob{}).Where("tenant_uuid = ? AND space_uuid = ? AND status IN ?", tenant, space, []string{HostIndexStatusQueued, HostIndexStatusRunning})
	if document != "" {
		q = q.Where("document_uuid = ? OR (operation = ? AND document_uuid IS NULL)", document, HostIndexOperationRebuild)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return KnowledgeIndexConflictError(errors.New("target has an active indexing task"))
	}
	return nil
}
func acceptedHostJob(row models.IndexJob) HostDocumentJob {
	out := HostDocumentJob{JobUUID: row.UUID.String(), Status: row.Status, Operation: row.Operation}
	if row.DocumentUUID != nil {
		out.DocumentUUID = *row.DocumentUUID
	}
	return out
}
func (s *HostContractService) acceptRebuild(ctx context.Context, tenant, space, document string, input HostRebuildInput) (HostDocumentJob, error) {
	if input.Mode == "" {
		input.Mode = HostRebuildReuseSnapshot
	}
	if input.Mode != HostRebuildReuseSnapshot && input.Mode != HostRebuildApplyConfig {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(errors.New("mode must be reuse_snapshot or apply_config"))
	}
	if (input.Mode == HostRebuildReuseSnapshot && (input.Ingestion != nil || input.Indexing != nil)) || (input.Mode == HostRebuildApplyConfig && input.Ingestion == nil && input.Indexing == nil) {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(errors.New("ingestion is required only for apply_config"))
	}
	if len(input.IdempotencyKey) > 128 || strings.TrimSpace(input.IdempotencyKey) != input.IdempotencyKey {
		return HostDocumentJob{}, KnowledgeInvalidArgumentError(errors.New("invalid idempotency_key"))
	}
	requestBytes, _ := json.Marshal(map[string]any{"mode": input.Mode, "document_uuid": document, "ingestion": input.Ingestion, "indexing": input.Indexing})
	requestHash := checksumJSON(requestBytes)
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = uuid.NewString()
	}
	var job models.IndexJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, err := s.lockHostSpace(ctx, tx, tenant, space)
		if err != nil {
			return err
		}
		var old models.IndexJob
		result := tx.Where("tenant_uuid = ? AND space_uuid = ? AND idempotency_key = ?", tenant, space, input.IdempotencyKey).Limit(1).Find(&old)
		if result.Error != nil {
			return result.Error
		}
		if old.UUID != uuid.Nil {
			if old.RequestChecksum != requestHash {
				return KnowledgeIndexConflictError(errors.New("idempotency_key already used for a different rebuild request"))
			}
			job = old
			return nil
		}
		var documents []models.TenantDocument
		q := tx.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND space_uuid = ? AND index_status <> ?", tenant, space, HostDocumentStatusDeleted)
		if document != "" {
			q = q.Where("uuid = ?", document)
		}
		var sourceBytes struct{ Total int64 }
		lengthExpr := "SUM(OCTET_LENGTH(content))"
		if tx.Dialector.Name() == "sqlite" {
			lengthExpr = "SUM(LENGTH(CAST(content AS BLOB)))"
		}
		if err := q.Session(&gorm.Session{}).Select("COALESCE(" + lengthExpr + ",0) AS total").Scan(&sourceBytes).Error; err != nil {
			return err
		}
		if sourceBytes.Total > 32<<20 {
			return KnowledgeInvalidArgumentError(errors.New("rebuild source snapshot exceeds 32 MiB"))
		}
		if err := q.Order("uuid ASC").Find(&documents).Error; err != nil {
			return err
		}
		if document != "" && len(documents) != 1 {
			return KnowledgeDocumentNotFoundError(errors.New("document not found in this tenant/space"))
		}
		if len(documents) == 0 {
			return KnowledgeIndexConflictError(errors.New("no documents to rebuild"))
		}
		if len(documents) > 1000 {
			return KnowledgeInvalidArgumentError(errors.New("space rebuild exceeds 1000 document limit"))
		}
		if err := ensureNoHostActivity(tx, tenant, space, document); err != nil {
			return err
		}
		batch := hostRebuildBatch{Schema: hostRebuildBatchSchema, Documents: []hostFrozenDocument{}}
		priority := "normal"
		for _, doc := range documents {
			source := hostDocumentSource{Title: doc.Title, URI: doc.URI, Content: doc.Content, ContentType: doc.ContentType, Checksum: doc.Checksum, Version: doc.Version, Tags: decodeTags(doc.Tags)}
			if len(doc.SemanticSource) > 0 {
				var saved hostDocumentSource
				if json.Unmarshal(doc.SemanticSource, &saved) != nil {
					return KnowledgeInvalidArgumentError(errors.New("invalid saved semantic artifacts"))
				}
				source.Artifacts, source.ExternalRef = saved.Artifacts, saved.ExternalRef
			}
			if checksumBytes([]byte(source.Content)) != source.Checksum {
				return KnowledgeInvalidArgumentError(errors.New("current document checksum is invalid"))
			}
			var config HostIngestionSnapshot
			base := ""
			if doc.ActiveIndexJobUUID != nil {
				base = *doc.ActiveIndexJobUUID
			}
			if input.Mode == HostRebuildApplyConfig && len(source.Artifacts) > 0 && input.Indexing == nil {
				return semanticError(422, "KNOWLEDGE_EXPLICIT_INDEXING_REQUIRED")
			}
			if input.Mode == HostRebuildApplyConfig {
				loader := &HostContractService{db: tx}
				config, err = loader.freezeIngestion(ctx, locked, HostDocumentInput{Title: doc.Title, URI: doc.URI, Content: doc.Content, ContentType: doc.ContentType, Checksum: doc.Checksum, Version: doc.Version, Ingestion: input.Ingestion})
				if err != nil {
					return err
				}
			} else {
				config, err = s.lastSuccessfulHostConfig(tx, tenant, space, doc.UUID.String(), base)
				if err != nil {
					return err
				}
				config.SourceChecksum, config.SourceVersion = source.Checksum, source.Version
				if err := validateHostSnapshot(config, source.ContentType); err != nil {
					return err
				}
				if err := validateHostChunkBudget(config, source.Content); err != nil {
					return err
				}
			}
			if input.Mode == HostRebuildApplyConfig && input.Indexing != nil {
				if s.semantic == nil {
					return semanticError(501, "KNOWLEDGE_SEMANTIC_UNSUPPORTED")
				}
				config.IndexMode = input.Indexing.Mode
				config.Indexing, err = s.semantic.freeze(ctx, tenant, space, *input.Indexing)
				if err != nil {
					return err
				}
			}
			if config.Indexing != nil {
				if err := validateSemanticArtifacts(HostDocumentInput{Indexing: &config.Indexing.SemanticIndexingSettings, Artifacts: source.Artifacts, ExternalRef: source.ExternalRef}); err != nil {
					return err
				}
			}
			if config.Priority == "high" {
				priority = "high"
			}
			batch.Documents = append(batch.Documents, hostFrozenDocument{DocumentUUID: doc.UUID.String(), BaseIndexJobUUID: base, Source: source, Config: config})
		}
		payload, _ := json.Marshal(batch)
		requested, _ := json.Marshal(input.Ingestion)
		job = models.IndexJob{TenantUUID: tenant, SpaceUUID: space, Operation: HostIndexOperationRebuild, Status: HostIndexStatusQueued, Priority: priority, IdempotencyKey: &input.IdempotencyKey, RequestChecksum: requestHash, RebuildMode: input.Mode, DocumentCount: len(documents), SourceSnapshot: payload, ConfigSnapshot: payload, SnapshotChecksum: checksumJSON(payload), RequestedConfig: requested, TraceID: reqctx.GetTraceID(ctx)}
		if document != "" {
			job.DocumentUUID = &document
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		for _, doc := range documents {
			res := tx.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ?", tenant, doc.UUID).Update("index_job_uuid", job.UUID.String())
			if res.Error != nil {
				return res.Error
			}
		}
		return nil
	})
	if err != nil {
		return HostDocumentJob{}, mapHostDBError(err)
	}
	s.runDocumentJobAsync(job.UUID)
	return acceptedHostJob(job), nil
}
func (s *HostContractService) lastSuccessfulHostConfig(tx *gorm.DB, tenant, space, document, jobID string) (HostIngestionSnapshot, error) {
	var config HostIngestionSnapshot
	if jobID == "" {
		return config, unsupportedSetting("no successful configuration snapshot; use apply_config")
	}
	var job models.IndexJob
	if err := tx.Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ? AND status = ?", tenant, space, jobID, HostIndexStatusSucceeded).First(&job).Error; err != nil {
		return config, KnowledgeIndexConflictError(errors.New("successful index snapshot unavailable"))
	}
	if checksumJSON(job.ConfigSnapshot) != job.SnapshotChecksum {
		return config, knowledgeError(500, KnowledgeReasonSnapshotInvalid, errors.New("successful snapshot checksum invalid"))
	}
	if job.Operation == HostIndexOperationRebuild {
		var batch hostRebuildBatch
		if strictJSON(job.ConfigSnapshot, &batch) != nil || batch.Schema != hostRebuildBatchSchema {
			return config, unsupportedSetting("legacy copied-chunk rebuild has no source configuration; use apply_config")
		}
		for _, item := range batch.Documents {
			if item.DocumentUUID == document {
				return item.Config, nil
			}
		}
		return config, KnowledgeIndexConflictError(errors.New("document not present in successful rebuild"))
	}
	if job.DocumentUUID == nil || *job.DocumentUUID != document || strictJSON(job.ConfigSnapshot, &config) != nil {
		return config, KnowledgeIndexConflictError(errors.New("invalid successful document snapshot"))
	}
	return config, nil
}
