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
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type hostRebuildReference struct {
	DocumentUUID string `json:"document_uuid"`
	JobUUID      string `json:"job_uuid"`
}

type hostDocumentSource struct {
	Title       string   `json:"title"`
	URI         string   `json:"uri"`
	Content     string   `json:"content"`
	ContentType string   `json:"content_type"`
	Checksum    string   `json:"checksum"`
	Version     string   `json:"version"`
	Tags        []string `json:"tags"`
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
		if strings.Contains(workErr.Error(), KnowledgeReasonSnapshotInvalid) {
			code = KnowledgeReasonSnapshotInvalid
		}
	}
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	res := s.db.WithContext(finishCtx).Model(&models.IndexJob{}).Where("tenant_uuid = ? AND uuid = ? AND status = ? AND claim_token = ?", job.TenantUUID, job.UUID, HostIndexStatusRunning, job.ClaimToken).Updates(map[string]any{"status": status, "error_code": code, "completed_at": time.Now(), "chunk_count": count})
	if res.Error == nil && res.RowsAffected == 1 && workErr != nil && job.DocumentUUID != nil {
		if err := s.db.WithContext(finishCtx).Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ? AND index_job_uuid = ?", job.TenantUUID, *job.DocumentUUID, job.UUID.String()).Update("index_status", HostIndexStatusFailed).Error; err != nil {
			return true, err
		}
	}
	return true, res.Error
}
func (s *HostContractService) executeHostJob(ctx context.Context, job models.IndexJob) (int, error) {
	if job.Operation == HostIndexOperationUpsert {
		var source hostDocumentSource
		var config HostIngestionSnapshot
		if len(job.ConfigSnapshot) == 0 || strictJSON(job.ConfigSnapshot, &config) != nil || strictJSON(job.SourceSnapshot, &source) != nil || checksumJSON(job.ConfigSnapshot) != job.SnapshotChecksum || checksumBytes([]byte(source.Content)) != config.SourceChecksum || source.Version != config.SourceVersion || job.DocumentUUID == nil || validateHostSnapshot(config, source.ContentType) != nil {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		format := "txt"
		if source.ContentType == "text/markdown" {
			format = "markdown"
		}
		chunks := ChunkDocument(uuid.MustParse(job.SpaceUUID), format, source.URI, []DocumentUnit{{Content: source.Content, Confidence: 1, Provenance: map[string]any{"source_checksum": source.Checksum, "source_version": source.Version}}}, ChunkingOptions{DocUUID: *job.DocumentUUID, Mode: config.SegmentMode, SizePolicy: config.SegmentSizePolicy, ChunkSize: config.ChunkSize, ChunkOverlap: config.ChunkOverlap, SegmentOrder: config.SegmentOrder, Separators: config.Separators, Anchors: ChunkAnchors{HeadingPath: config.AnchorHeadingPath, ClauseID: config.AnchorClauseID, RowNumber: config.AnchorRowNumber, Speaker: config.AnchorSpeaker, SentenceIndex: config.AnchorSentenceIndex}}, nil)
		rows := []models.HostDocumentChunk{}
		for _, ch := range chunks {
			if ch.Kind != "chunk" {
				continue
			}
			ch.Metadata["host_job_uuid"] = job.UUID.String()
			ch.Metadata["snapshot_checksum"] = job.SnapshotChecksum
			raw, _ := json.Marshal(ch.Metadata)
			row := models.HostDocumentChunk{TenantUUID: job.TenantUUID, SpaceUUID: job.SpaceUUID, JobUUID: job.UUID.String(), DocumentUUID: *job.DocumentUUID, Ordinal: len(rows) + 1, Kind: ch.Kind, Content: ch.Content, Checksum: checksumBytes([]byte(ch.Content)), Metadata: raw}
			row.UUID = uuid.NewSHA1(job.UUID, []byte(ch.ID.String()))
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			return 0, errors.New("no content chunks")
		}
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var live models.IndexJob
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ? AND status = ? AND claim_token = ?", job.TenantUUID, job.UUID, HostIndexStatusRunning, job.ClaimToken).First(&live).Error; err != nil {
				return err
			}
			if live.LeaseUntil == nil || !live.LeaseUntil.After(time.Now()) {
				return errors.New("job lease expired")
			}
			if err := tx.CreateInBatches(rows, 100).Error; err != nil {
				return err
			}
			return tx.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ? AND index_job_uuid = ?", job.TenantUUID, *job.DocumentUUID, job.UUID.String()).Updates(map[string]any{"index_status": HostDocumentStatusIndexed, "indexed_at": time.Now()}).Error
		})
		return len(rows), err
	}
	if job.DocumentUUID == nil && job.Operation != HostIndexOperationRebuild {
		return 0, errors.New("document UUID missing")
	}
	if job.Operation == HostIndexOperationDelete {
		return 0, s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND uuid = ?", job.TenantUUID, job.SpaceUUID, *job.DocumentUUID).Delete(&models.TenantDocument{}).Error
	}
	// rebuild 不能仅标记成功；为当前文档重新执行已冻结配置，保留旧任务产物。
	if job.Operation == HostIndexOperationRebuild {
		var references []hostRebuildReference
		if strictJSON(job.SourceSnapshot, &references) != nil {
			return 0, errors.New(KnowledgeReasonSnapshotInvalid)
		}
		rows := []models.HostDocumentChunk{}
		for _, ref := range references {
			var old []models.HostDocumentChunk
			if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND space_uuid = ? AND document_uuid = ? AND job_uuid = ?", job.TenantUUID, job.SpaceUUID, ref.DocumentUUID, ref.JobUUID).Order("ordinal ASC").Find(&old).Error; err != nil {
				return 0, err
			}
			if len(old) == 0 {
				return 0, errors.New("source chunks unavailable")
			}
			for _, chunk := range old {
				chunk.ID = 0
				chunk.UUID = uuid.NewSHA1(job.UUID, []byte(chunk.UUID.String()))
				chunk.JobUUID = job.UUID.String()
				chunk.Ordinal = len(rows) + 1
				chunk.CreatedAt = time.Time{}
				chunk.UpdatedAt = time.Time{}
				rows = append(rows, chunk)
			}
		}
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var live models.IndexJob
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND status = ? AND claim_token = ?", job.UUID, HostIndexStatusRunning, job.ClaimToken).First(&live).Error; err != nil {
				return err
			}
			if live.LeaseUntil == nil || !live.LeaseUntil.After(time.Now()) {
				return errors.New("lease expired")
			}
			if len(rows) > 0 {
				if err := tx.CreateInBatches(rows, 100).Error; err != nil {
					return err
				}
			}
			for _, ref := range references {
				if err := tx.Model(&models.TenantDocument{}).Where("tenant_uuid = ? AND uuid = ? AND index_job_uuid = ?", job.TenantUUID, ref.DocumentUUID, ref.JobUUID).Updates(map[string]any{"index_job_uuid": job.UUID.String(), "indexed_at": time.Now()}).Error; err != nil {
					return err
				}
			}
			return nil
		})
		return len(rows), err
	}
	return 0, errors.New("unsupported operation")
}
func (s *HostContractService) GetJobChunks(ctx context.Context, tenant, jobID string) ([]models.HostDocumentChunk, error) {
	if _, err := s.GetIndexJob(ctx, tenant, jobID); err != nil {
		return nil, err
	}
	var rows []models.HostDocumentChunk
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND job_uuid = ?", tenant, jobID).Order("ordinal ASC").Find(&rows).Error; err != nil {
		return nil, KnowledgeUpstreamDependencyError(err)
	}
	return rows, nil
}
