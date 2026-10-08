package knowledge_space

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type hostRebuildReference struct {
	DocumentUUID string `json:"document_uuid"`
	JobUUID      string `json:"job_uuid"`
}

type hostDocumentSource struct {
	Title       string                  `json:"title"`
	URI         string                  `json:"uri"`
	Content     string                  `json:"content"`
	ContentType string                  `json:"content_type"`
	Checksum    string                  `json:"checksum"`
	Version     string                  `json:"version"`
	Tags        []string                `json:"tags"`
	Artifacts   []SemanticArtifactInput `json:"artifacts,omitempty"`
	ExternalRef *SemanticExternalRef    `json:"external_ref,omitempty"`
}

func strictJSON(raw []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected one JSON object")
	}
	return nil
}

// Run 由 Core 生命周期启动；数据库保存低频受理/终态，high 在未领取队列中优先。
// 文本处理无需外部副作用，单实例一个执行槽；并发实例通过行锁及 claim token 互斥。
func (s *HostContractService) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		handled, err := s.ProcessNextDocumentJob(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		if handled {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-s.wake:
		}
	}
}
func (s *HostContractService) ProcessNextDocumentJob(ctx context.Context) (bool, error) {
	now := time.Now()
	// 中断的任务明确失败，避免伪造成功或无限 running；重试由新的提交受理。
	if err := s.db.WithContext(ctx).Model(&models.IndexJob{}).Where("status = ? AND lease_until < ?", HostIndexStatusRunning, now).Updates(map[string]any{"status": HostIndexStatusFailed, "error_code": "KNOWLEDGE_WORKER_INTERRUPTED", "completed_at": now}).Error; err != nil {
		return false, err
	}
	var job models.IndexJob
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Where("status = ?", HostIndexStatusQueued).Order("CASE WHEN priority = 'high' THEN 0 ELSE 1 END").Order("created_at ASC").Order("id ASC")
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		if err := q.Limit(1).Find(&job).Error; err != nil {
			return err
		}
		if job.UUID == uuid.Nil {
			return gorm.ErrRecordNotFound
		}
		token := uuid.NewString()
		lease := now.Add(5 * time.Minute)
		res := tx.Model(&models.IndexJob{}).Where("uuid = ? AND status = ?", job.UUID, HostIndexStatusQueued).Updates(map[string]any{"status": HostIndexStatusRunning, "started_at": now, "claim_token": token, "lease_until": lease})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		job.ClaimToken = token
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	count, workErr := s.executeHostJob(workCtx, job)
	status, code := HostIndexStatusSucceeded, ""
	if workErr != nil {
		status, code = HostIndexStatusFailed, "KNOWLEDGE_INDEX_EXECUTION_FAILED"
		if known := dto.CodeOf(workErr); known != "" {
			code = known
		}
		if strings.Contains(workErr.Error(), KnowledgeReasonSnapshotInvalid) {
			code = KnowledgeReasonSnapshotInvalid
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	res := s.db.WithContext(finishCtx).Model(&models.IndexJob{}).Where("tenant_uuid = ? AND uuid = ? AND status = ? AND claim_token = ?", job.TenantUUID, job.UUID, HostIndexStatusRunning, job.ClaimToken).Updates(map[string]any{"status": status, "error_code": code, "completed_at": time.Now(), "chunk_count": count})
	if res.Error == nil && res.RowsAffected == 1 && workErr != nil && job.DocumentUUID != nil {
		if err := s.db.WithContext(finishCtx).Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ? AND index_job_uuid = ? AND active_index_job_uuid IS NULL", job.TenantUUID, *job.DocumentUUID, job.UUID.String()).Update("index_status", HostIndexStatusFailed).Error; err != nil {
			return true, err
		}
	}
	return true, res.Error
}
func buildHostDocumentChunks(job models.IndexJob, document string, source hostDocumentSource, config HostIngestionSnapshot, ordinal int) ([]models.HostDocumentChunk, error) {
	if checksumBytes([]byte(source.Content)) != config.SourceChecksum || source.Version != config.SourceVersion || validateHostSnapshot(config, source.ContentType) != nil || validateHostChunkBudget(config, source.Content) != nil {
		return nil, errors.New(KnowledgeReasonSnapshotInvalid)
	}
	format := "txt"
	if source.ContentType == "text/markdown" {
		format = "markdown"
	}
	chunks := ChunkDocument(uuid.MustParse(job.SpaceUUID), format, source.URI, []DocumentUnit{{Content: source.Content, Confidence: 1, Provenance: map[string]any{"source_checksum": source.Checksum, "source_version": source.Version}}}, ChunkingOptions{DocUUID: document, Mode: config.SegmentMode, SizePolicy: config.SegmentSizePolicy, ChunkSize: config.ChunkSize, ChunkOverlap: config.ChunkOverlap, SegmentOrder: config.SegmentOrder, Separators: config.Separators, Anchors: ChunkAnchors{HeadingPath: config.AnchorHeadingPath, ClauseID: config.AnchorClauseID, RowNumber: config.AnchorRowNumber, Speaker: config.AnchorSpeaker, SentenceIndex: config.AnchorSentenceIndex}}, nil)
	rows := []models.HostDocumentChunk{}
	for _, ch := range chunks {
		if ch.Kind != "chunk" {
			continue
		}
		ch.Metadata["host_job_uuid"], ch.Metadata["snapshot_checksum"] = job.UUID.String(), job.SnapshotChecksum
		ch.Metadata["source_title"], ch.Metadata["source_uri"], ch.Metadata["source_version"] = source.Title, source.URI, source.Version
		ch.Metadata["source_tags"] = source.Tags
		raw, _ := json.Marshal(ch.Metadata)
		row := models.HostDocumentChunk{TenantUUID: job.TenantUUID, SpaceUUID: job.SpaceUUID, JobUUID: job.UUID.String(), DocumentUUID: document, Ordinal: ordinal + len(rows) + 1, Kind: ch.Kind, Content: ch.Content, Checksum: checksumBytes([]byte(ch.Content)), Metadata: raw}
		row.UUID = uuid.NewSHA1(job.UUID, []byte(document+":"+ch.ID.String()))
		rows = append(rows, row)
	}
	if len(rows) == 0 || len(rows) > 4096 {
		return nil, errors.New("invalid content chunk count")
	}
	return rows, nil
}
func (s *HostContractService) executeHostJob(ctx context.Context, job models.IndexJob) (int, error) {
	if job.Operation == HostIndexOperationDelete {
		if job.DocumentUUID == nil {
			return 0, errors.New("document UUID missing")
		}
		return 0, s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", job.TenantUUID, job.SpaceUUID, *job.DocumentUUID).Delete(&models.TenantDocument{}).Error
	}
	var documents []hostFrozenDocument
	if checksumJSON(job.ConfigSnapshot) != job.SnapshotChecksum {
		return 0, errors.New(KnowledgeReasonSnapshotInvalid)
	}
	switch job.Operation {
	case HostIndexOperationUpsert:
		if job.DocumentUUID == nil {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		var source hostDocumentSource
		var config HostIngestionSnapshot
		if strictJSON(job.ConfigSnapshot, &config) != nil || strictJSON(job.SourceSnapshot, &source) != nil {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		documents = []hostFrozenDocument{{DocumentUUID: *job.DocumentUUID, Source: source, Config: config}}
	case HostIndexOperationRebuild:
		var batch hostRebuildBatch
		if strictJSON(job.ConfigSnapshot, &batch) != nil || batch.Schema != hostRebuildBatchSchema || len(batch.Documents) == 0 || len(batch.Documents) != job.DocumentCount {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		documents = batch.Documents
	default:
		return 0, errors.New("unsupported operation")
	}
	rows := []models.HostDocumentChunk{}
	seen := map[string]bool{}
	for i, item := range documents {
		id, err := uuid.Parse(item.DocumentUUID)
		if err != nil || id == uuid.Nil || seen[item.DocumentUUID] || (job.DocumentUUID != nil && *job.DocumentUUID != item.DocumentUUID) {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		seen[item.DocumentUUID] = true
		var built []models.HostDocumentChunk
		var buildErr error
		if item.Config.Indexing != nil {
			if s.semantic == nil {
				return 0, semanticError(501, "KNOWLEDGE_SEMANTIC_UNSUPPORTED")
			}
			built, buildErr = s.semantic.build(ctx, job, item.DocumentUUID, item.Source, item.Config, len(rows))
		} else {
			built, buildErr = buildHostDocumentChunks(job, item.DocumentUUID, item.Source, item.Config, len(rows))
		}
		err = buildErr
		if err != nil {
			return 0, err
		}
		rows = append(rows, built...)
		if len(rows) > 65536 {
			return 0, errors.New("space chunk budget exceeded")
		}
		if err := s.db.WithContext(ctx).Model(&models.IndexJob{}).Where("uuid = ? AND claim_token = ? AND status = ?", job.UUID, job.ClaimToken, HostIndexStatusRunning).Update("processed_documents", i+1).Error; err != nil {
			return 0, err
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.lockHostSpace(ctx, tx, job.TenantUUID, job.SpaceUUID); err != nil {
			return err
		}
		var live models.IndexJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ? AND status = ? AND claim_token = ?", job.TenantUUID, job.UUID, HostIndexStatusRunning, job.ClaimToken).First(&live).Error; err != nil {
			return err
		}
		if live.LeaseUntil == nil || !live.LeaseUntil.After(time.Now()) {
			return errors.New("job lease expired")
		}
		for _, item := range documents {
			var doc models.TenantDocument
			if err := tx.Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ? AND index_job_uuid = ? AND index_status <> ?", job.TenantUUID, job.SpaceUUID, item.DocumentUUID, job.UUID.String(), HostDocumentStatusDeleted).First(&doc).Error; err != nil {
				return err
			}
			if doc.Checksum != item.Source.Checksum || doc.Version != item.Source.Version {
				return errors.New("document version changed before index activation")
			}
			if item.Config.Indexing != nil {
				var binding models.SemanticSpaceBinding
				if tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND space_uuid = ? AND configuration_generation = ?", job.TenantUUID, job.SpaceUUID, item.Config.Indexing.ConfigurationGeneration).First(&binding).Error != nil {
					return semanticError(409, "KNOWLEDGE_INDEX_GENERATION_CONFLICT")
				}
				if err := tx.Model(&binding).Update("corpus_generation", uuid.NewString()).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.CreateInBatches(rows, 100).Error; err != nil {
			return err
		}
		for _, item := range documents {
			res := tx.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ? AND index_job_uuid = ?", job.TenantUUID, item.DocumentUUID, job.UUID.String()).Updates(map[string]any{"active_index_job_uuid": job.UUID.String(), "index_status": HostDocumentStatusIndexed, "indexed_at": time.Now()})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return errors.New("activation lost document ownership")
			}
		}
		// 新产物、active 指针和任务成功处于同一事务，失败不切换旧索引。
		return tx.Model(&live).Updates(map[string]any{"status": HostIndexStatusSucceeded, "completed_at": time.Now(), "error_code": "", "chunk_count": len(rows), "processed_documents": len(documents)}).Error
	})
	return len(rows), err
}
func (s *HostContractService) GetJobChunks(ctx context.Context, tenant, jobID string) ([]HostChunk, error) {
	if _, err := s.GetIndexJob(ctx, tenant, jobID); err != nil {
		return nil, err
	}
	var rows []models.HostDocumentChunk
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND job_uuid = ?", tenant, jobID).Order("ordinal ASC").Find(&rows).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	out := make([]HostChunk, 0, len(rows))
	for _, row := range rows {
		out = append(out, HostChunk{UUID: row.UUID.String(), SpaceUUID: row.SpaceUUID, JobUUID: row.JobUUID, DocumentUUID: row.DocumentUUID, Ordinal: row.Ordinal, Kind: row.Kind, Content: row.Content, Checksum: row.Checksum, Metadata: append(json.RawMessage(nil), row.Metadata...)})
	}
	return out, nil
}

type HostChunk struct {
	UUID         string          `json:"uuid"`
	SpaceUUID    string          `json:"space_uuid"`
	JobUUID      string          `json:"job_uuid"`
	DocumentUUID string          `json:"document_uuid"`
	Ordinal      int             `json:"ordinal"`
	Kind         string          `json:"kind"`
	Content      string          `json:"content"`
	Checksum     string          `json:"checksum"`
	Metadata     json.RawMessage `json:"metadata"`
}
