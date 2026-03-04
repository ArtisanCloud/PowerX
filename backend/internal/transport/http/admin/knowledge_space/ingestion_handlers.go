package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	knowledgeRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// IngestionHandler exposes ingestion job endpoints.
type IngestionHandler struct {
	svc *ksvc.IngestionService
	db  *gorm.DB
	vs  vectorstore.Store
}

func NewIngestionHandler(deps *shared.Deps) *IngestionHandler {
	if deps == nil || deps.KnowledgeSpace == nil {
		return nil
	}
	return &IngestionHandler{svc: deps.KnowledgeSpace.Ingestion, db: deps.DB, vs: deps.KnowledgeSpace.VectorStore}
}

func (h *IngestionHandler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	if limit <= 0 {
		limit = 20
	}
	jobs, err := h.svc.ListJobs(c.Request.Context(), spaceID, limit)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取入库任务失败", err)
		return
	}
	views := make([]ingestionJobView, 0, len(jobs))
	for i := range jobs {
		views = append(views, toIngestionJobView(&jobs[i]))
	}
	dto.ResponseSuccess(c, views)
}

func (h *IngestionHandler) Get(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	jobID, err := uuid.Parse(c.Param("jobId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "任务 ID 无效", err)
		return
	}
	job, err := h.svc.GetJob(c.Request.Context(), spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取入库任务失败", err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "入库任务不存在", errors.New("not found"))
		return
	}
	dto.ResponseSuccess(c, toIngestionJobView(job))
}

func (h *IngestionHandler) Trigger(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	var req ingestionJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	format := strings.TrimSpace(req.Format)
	if format == "" {
		format = strings.TrimSpace(req.SourceType)
	}
	if format == "" {
		dto.ResponseError(c, http.StatusBadRequest, "缺少文档格式(format/sourceType)", errors.New("missing format"))
		return
	}
	job, err := h.svc.TriggerAsync(c.Request.Context(), ksvc.TriggerIngestionInput{
		SpaceID:             spaceID,
		Format:              format,
		SourceURI:           req.SourceURI,
		DocUUID:             req.DocUUID,
		IngestionProfile:    req.IngestionProfile,
		ProcessorProfile:    req.ProcessorProfile,
		OCRRequired:         req.OCRRequired,
		MaskingProfile:      req.MaskingProfile,
		Priority:            req.Priority,
		RequestedBy:         req.RequestedBy,
		RagSceneKey:         req.RagSceneKey,
		RagBundleKey:        req.RagBundleKey,
		RagPrimary:          req.RagPrimary,
		SegmentMode:         req.SegmentMode,
		ChunkSize:           req.ChunkSize,
		ChunkOverlap:        req.ChunkOverlap,
		SegmentSizePolicy:   req.SegmentSizePolicy,
		SegmentOrder:        req.SegmentOrder,
		Separators:          req.Separators,
		PagePriority:        req.PagePriority,
		AnchorHeadingPath:   req.AnchorHeadingPath,
		AnchorClauseID:      req.AnchorClauseID,
		AnchorRowNumber:     req.AnchorRowNumber,
		AnchorSpeaker:       req.AnchorSpeaker,
		AnchorSentenceIndex: req.AnchorSentenceIndex,
	})
	if err != nil {
		var appErr *dto.AppError
		if errors.As(err, &appErr) {
			dto.RespondErrorFrom(c, err)
			return
		}
		switch {
		case errors.Is(err, ksvc.ErrInvalidInput):
			dto.ResponseError(c, http.StatusBadRequest, "入库参数不合法", err)
		case errors.Is(err, ksvc.ErrSpaceNotFound):
			dto.ResponseError(c, http.StatusNotFound, "知识空间不存在或已退役", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "触发入库失败", err)
		}
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toIngestionJobView(job))
}

type ingestionChunkView struct {
	ChunkID    string         `json:"chunkId"`
	Kind       string         `json:"kind"`
	Content    string         `json:"content"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Confidence float64        `json:"confidence"`
	Masked     bool           `json:"masked"`
}

type ingestionChunkListResponse struct {
	SpaceID   string               `json:"spaceId"`
	JobID     string               `json:"jobId"`
	Format    string               `json:"format,omitempty"`
	SourceURI string               `json:"sourceUri,omitempty"`
	Total     int                  `json:"total"`
	Page      int                  `json:"page"`
	PageSize  int                  `json:"pageSize"`
	Items     []ingestionChunkView `json:"items"`
}

func (h *IngestionHandler) DeleteJob(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	jobID, err := uuid.Parse(c.Param("jobId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "任务 ID 无效", err)
		return
	}

	res, err := h.svc.DeleteJobPurge(c.Request.Context(), spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "删除入库任务失败", err)
		return
	}
	if !res.Deleted {
		dto.ResponseError(c, http.StatusNotFound, "入库任务不存在", errors.New("not found"))
		return
	}
	dto.ResponseSuccess(c, res)
}

func (h *IngestionHandler) Chunks(c *gin.Context) {
	if h == nil || h.svc == nil || h.db == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	jobID, err := uuid.Parse(c.Param("jobId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "任务 ID 无效", err)
		return
	}
	job, err := h.svc.GetJob(c.Request.Context(), spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取入库任务失败", err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "入库任务不存在", errors.New("not found"))
		return
	}

	page, _ := strconv.Atoi(strings.TrimSpace(c.Query("page")))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(strings.TrimSpace(c.Query("pageSize")))
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	isRunning := func(status string) bool {
		s := strings.ToLower(strings.TrimSpace(status))
		return s == "running" || s == "retrying" || s == "pending"
	}

	// Prefer DB chunk store when available (truth source). Fallback to chunk_manifest when DB is unavailable or empty.
	if repo := knowledgeRepo.NewKnowledgeChunkRepository(h.db); repo != nil {
		rows, total, err := repo.ListByJob(c.Request.Context(), spaceID, jobID, page, pageSize)
		if err == nil && total > 0 {
			items := make([]ingestionChunkView, 0, len(rows))
			sourceURI := ""
			for _, row := range rows {
				meta := map[string]any{}
				if len(row.Metadata) > 0 {
					_ = json.Unmarshal(row.Metadata, &meta)
				}
				if sourceURI == "" {
					if v, ok := meta["source_uri"].(string); ok {
						sourceURI = strings.TrimSpace(v)
					}
				}
				conf := 0.0
				if v, ok := meta["confidence"].(float64); ok {
					conf = v
				}
				masked := false
				if v, ok := meta["masked"].(bool); ok {
					masked = v
				}
				items = append(items, ingestionChunkView{
					ChunkID:    row.ChunkUUID.String(),
					Kind:       strings.TrimSpace(row.Kind),
					Content:    row.Content,
					Metadata:   meta,
					Confidence: conf,
					Masked:     masked,
				})
			}
			dto.ResponseSuccess(c, ingestionChunkListResponse{
				SpaceID:   spaceID.String(),
				JobID:     jobID.String(),
				Format:    strings.TrimSpace(job.SourceType),
				SourceURI: sourceURI,
				Total:     int(total),
				Page:      page,
				PageSize:  pageSize,
				Items:     items,
			})
			return
		}
	}

	bundle, err := knowledgeRepo.NewArtifactBundleRepository(h.db).FindByJobID(c.Request.Context(), job.ID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取产物信息失败", err)
		return
	}
	if bundle == nil {
		if isRunning(job.Status) {
			dto.ResponseSuccess(c, ingestionChunkListResponse{
				SpaceID:   spaceID.String(),
				JobID:     jobID.String(),
				Format:    strings.TrimSpace(job.SourceType),
				SourceURI: "",
				Total:     0,
				Page:      page,
				PageSize:  pageSize,
				Items:     []ingestionChunkView{},
			})
			return
		}
		dto.ResponseError(c, http.StatusNotFound, "产物未生成或已清理", errors.New("bundle not found"))
		return
	}

	if strings.TrimSpace(bundle.ChunkManifestURI) == "" {
		if strings.TrimSpace(job.ErrorCode) != "" {
			dto.ResponseError(c, http.StatusFailedDependency, fmt.Sprintf("切块产物缺失（入库失败）：%s %s", job.ErrorCode, job.BlockedReason), errors.New("chunk manifest missing"))
			return
		}
		if isRunning(job.Status) {
			dto.ResponseSuccess(c, ingestionChunkListResponse{
				SpaceID:   spaceID.String(),
				JobID:     jobID.String(),
				Format:    strings.TrimSpace(job.SourceType),
				SourceURI: "",
				Total:     0,
				Page:      page,
				PageSize:  pageSize,
				Items:     []ingestionChunkView{},
			})
			return
		}
		dto.ResponseError(c, http.StatusNotFound, "产物未生成或已清理", errors.New("chunk manifest missing"))
		return
	}

	manifestPath := resolveArtifactURIToLocalPath(bundle.ChunkManifestURI)
	if manifestPath == "" {
		dto.ResponseError(c, http.StatusNotImplemented, "当前环境不支持读取该产物 URI", errors.New("unsupported artifact uri"))
		return
	}
	abs := manifestPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Clean(abs)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		if strings.TrimSpace(job.ErrorCode) != "" {
			dto.ResponseError(c, http.StatusFailedDependency, fmt.Sprintf("读取产物失败（入库失败）：%s %s", job.ErrorCode, job.BlockedReason), err)
			return
		}
		if isRunning(job.Status) {
			dto.ResponseSuccess(c, ingestionChunkListResponse{
				SpaceID:   spaceID.String(),
				JobID:     jobID.String(),
				Format:    strings.TrimSpace(job.SourceType),
				SourceURI: "",
				Total:     0,
				Page:      page,
				PageSize:  pageSize,
				Items:     []ingestionChunkView{},
			})
			return
		}
		dto.ResponseError(c, http.StatusNotFound, "读取产物失败", err)
		return
	}

	var doc struct {
		SpaceID   string `json:"space_id"`
		JobID     string `json:"job_id"`
		Format    string `json:"format"`
		SourceURI string `json:"source_uri"`
		Chunks    []struct {
			ID            string         `json:"id"`
			IDAlt         string         `json:"ID"`
			Kind          string         `json:"kind"`
			KindAlt       string         `json:"Kind"`
			Content       string         `json:"content"`
			ContentAlt    string         `json:"Content"`
			Metadata      map[string]any `json:"metadata"`
			MetadataAlt   map[string]any `json:"Metadata"`
			Confidence    float64        `json:"confidence"`
			ConfidenceAlt float64        `json:"Confidence"`
			Masked        bool           `json:"masked"`
			MaskedAlt     bool           `json:"Masked"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "解析产物失败", err)
		return
	}

	// Keep ordering stable for UI: segment_part -> chunk_idx -> id.
	metaInt := func(meta map[string]any, key string) int {
		if meta == nil {
			return 1<<30 - 1
		}
		v, ok := meta[key]
		if !ok || v == nil {
			return 1<<30 - 1
		}
		switch x := v.(type) {
		case float64:
			return int(x)
		case int:
			return x
		case int64:
			return int(x)
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
				return n
			}
		}
		return 1<<30 - 1
	}
	sort.SliceStable(doc.Chunks, func(i, j int) bool {
		mi := doc.Chunks[i].Metadata
		if mi == nil {
			mi = doc.Chunks[i].MetadataAlt
		}
		mj := doc.Chunks[j].Metadata
		if mj == nil {
			mj = doc.Chunks[j].MetadataAlt
		}
		si := metaInt(mi, "segment_part")
		sj := metaInt(mj, "segment_part")
		if si != sj {
			return si < sj
		}
		ci := metaInt(mi, "chunk_idx")
		cj := metaInt(mj, "chunk_idx")
		if ci != cj {
			return ci < cj
		}
		idi := strings.TrimSpace(doc.Chunks[i].ID)
		if idi == "" {
			idi = strings.TrimSpace(doc.Chunks[i].IDAlt)
		}
		idj := strings.TrimSpace(doc.Chunks[j].ID)
		if idj == "" {
			idj = strings.TrimSpace(doc.Chunks[j].IDAlt)
		}
		return idi < idj
	})

	total := len(doc.Chunks)
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	items := make([]ingestionChunkView, 0, end-start)
	for _, ch := range doc.Chunks[start:end] {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			id = strings.TrimSpace(ch.IDAlt)
		}
		kind := strings.TrimSpace(ch.Kind)
		if kind == "" {
			kind = strings.TrimSpace(ch.KindAlt)
		}
		content := ch.Content
		if strings.TrimSpace(content) == "" {
			content = ch.ContentAlt
		}
		meta := ch.Metadata
		if meta == nil {
			meta = ch.MetadataAlt
		}
		conf := ch.Confidence
		if conf == 0 && ch.ConfidenceAlt != 0 {
			conf = ch.ConfidenceAlt
		}
		masked := ch.Masked || ch.MaskedAlt
		items = append(items, ingestionChunkView{
			ChunkID:    id,
			Kind:       kind,
			Content:    content,
			Metadata:   meta,
			Confidence: conf,
			Masked:     masked,
		})
	}

	dto.ResponseSuccess(c, ingestionChunkListResponse{
		SpaceID:   spaceID.String(),
		JobID:     jobID.String(),
		Format:    strings.TrimSpace(doc.Format),
		SourceURI: strings.TrimSpace(doc.SourceURI),
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
		Items:     items,
	})
}

type updateChunkRequest struct {
	Content    string `json:"content" binding:"required"`
	EditedBy   string `json:"editedBy"`
	EditReason string `json:"editReason"`
}

func (h *IngestionHandler) UpdateChunk(c *gin.Context) {
	if h == nil || h.svc == nil || h.db == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	if h.vs == nil {
		dto.ResponseError(c, http.StatusNotImplemented, "向量存储未配置，暂不支持编辑后重建索引", errors.New("vector store unavailable"))
		return
	}

	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	jobID, err := uuid.Parse(c.Param("jobId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "任务 ID 无效", err)
		return
	}
	chunkIDStr := strings.TrimSpace(c.Param("chunkId"))
	chunkUUID, err := uuid.Parse(chunkIDStr)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "Chunk ID 无效", err)
		return
	}
	var req updateChunkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		dto.ResponseError(c, http.StatusBadRequest, "内容不能为空", errors.New("empty content"))
		return
	}

	job, err := h.svc.GetJob(c.Request.Context(), spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取入库任务失败", err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "入库任务不存在", errors.New("not found"))
		return
	}

	bundle, err := knowledgeRepo.NewArtifactBundleRepository(h.db).FindByJobID(c.Request.Context(), job.ID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取产物信息失败", err)
		return
	}
	if bundle == nil {
		dto.ResponseError(c, http.StatusNotFound, "产物未生成或已清理", errors.New("bundle not found"))
		return
	}

	// 1) Update DB chunk store first (truth source) when enabled.
	now := time.Now().Format(time.RFC3339Nano)
	updated := false
	chunkKind := ""
	chunkMeta := map[string]any{}
	if repo := knowledgeRepo.NewKnowledgeChunkRepository(h.db); repo != nil {
		if row, err := repo.FindOne(c.Request.Context(), spaceID, chunkUUID); err == nil && row != nil {
			chunkKind = strings.TrimSpace(row.Kind)
			if len(row.Metadata) > 0 {
				_ = json.Unmarshal(row.Metadata, &chunkMeta)
			}
			chunkMeta["edited_at"] = now
			if strings.TrimSpace(req.EditedBy) != "" {
				chunkMeta["edited_by"] = strings.TrimSpace(req.EditedBy)
			}
			if strings.TrimSpace(req.EditReason) != "" {
				chunkMeta["edit_reason"] = strings.TrimSpace(req.EditReason)
			}
			metaBytes, _ := json.Marshal(chunkMeta)
			updates := map[string]any{
				"content":    content,
				"metadata":   metaBytes,
				"updated_at": time.Now(),
			}
			if err := h.db.WithContext(c.Request.Context()).
				Table(row.TableName()).
				Where("space_uuid = ? AND chunk_uuid = ?", spaceID, chunkUUID).
				Updates(updates).Error; err == nil {
				updated = true
			}
		}
	}

	// 2) Best-effort: keep chunk_manifest.json (offline bundle) in sync for replay/debug.
	updatedChunkBytes := []byte{}
	if strings.TrimSpace(bundle.ChunkManifestURI) != "" {
		chunkManifestPath := resolveArtifactURIToLocalPath(bundle.ChunkManifestURI)
		if chunkManifestPath != "" {
			if chunkBytes, err := os.ReadFile(filepath.Clean(chunkManifestPath)); err == nil {
				var manifest map[string]any
				if json.Unmarshal(chunkBytes, &manifest) == nil {
					if chunksAny, ok := manifest["chunks"].([]any); ok && len(chunksAny) > 0 {
						for _, raw := range chunksAny {
							obj, ok := raw.(map[string]any)
							if !ok {
								continue
							}
							id := ""
							if v, ok := obj["ID"].(string); ok {
								id = strings.TrimSpace(v)
							}
							if id == "" {
								if v, ok := obj["id"].(string); ok {
									id = strings.TrimSpace(v)
								}
							}
							if id != chunkIDStr {
								continue
							}
							if v, ok := obj["Kind"].(string); ok {
								chunkKind = strings.TrimSpace(v)
							}
							if chunkKind == "" {
								if v, ok := obj["kind"].(string); ok {
									chunkKind = strings.TrimSpace(v)
								}
							}
							obj["Content"] = content
							obj["content"] = content

							meta := map[string]any{}
							if v, ok := obj["Metadata"].(map[string]any); ok && v != nil {
								meta = v
							} else if v, ok := obj["metadata"].(map[string]any); ok && v != nil {
								meta = v
							}
							if len(chunkMeta) > 0 {
								for k, v := range chunkMeta {
									meta[k] = v
								}
							} else {
								meta["edited_at"] = now
								if strings.TrimSpace(req.EditedBy) != "" {
									meta["edited_by"] = strings.TrimSpace(req.EditedBy)
								}
								if strings.TrimSpace(req.EditReason) != "" {
									meta["edit_reason"] = strings.TrimSpace(req.EditReason)
								}
							}
							obj["Metadata"] = meta
							obj["metadata"] = meta
							chunkMeta = meta
							updated = true
							break
						}
						if out, err := json.MarshalIndent(manifest, "", "  "); err == nil {
							updatedChunkBytes = out
							_ = os.WriteFile(filepath.Clean(chunkManifestPath), out, 0o644)
						}
					}
				}
			}
		}
	}
	if len(updatedChunkBytes) == 0 {
		updatedChunkBytes = []byte(`{}`)
	}
	if !updated {
		dto.ResponseError(c, http.StatusNotFound, "未找到该 Chunk", errors.New("chunk not found"))
		return
	}

	// 更新向量索引（仅该 chunk）
	dim, err := h.activeVectorDimensions(c.Request.Context(), spaceID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "未找到可用的向量索引维度", err)
		return
	}

	meta := make(map[string]any, len(chunkMeta)+2)
	for k, v := range chunkMeta {
		meta[k] = v
	}
	meta["chunk_kind"] = chunkKind
	meta["content_hash"] = ksvc.ContentHash(content)
	if err := h.vs.Upsert(c.Request.Context(), spaceID, []vectorstore.VectorRecord{{
		ChunkID:   chunkUUID,
		Embedding: ksvc.HashEmbedding(content, dim),
		Metadata:  meta,
	}}); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "更新向量索引失败", err)
		return
	}

	// best-effort：同步更新 vector_manifest.json 并刷新 bundle checksum
	var vectorBytes []byte
	if strings.TrimSpace(bundle.VectorManifestURI) != "" {
		vectorManifestPath := resolveArtifactURIToLocalPath(bundle.VectorManifestURI)
		if vectorManifestPath != "" {
			if raw, err := os.ReadFile(filepath.Clean(vectorManifestPath)); err == nil {
				vectorBytes = raw
				var vdoc map[string]any
				if json.Unmarshal(raw, &vdoc) == nil {
					if vectors, ok := vdoc["vectors"].([]any); ok && len(vectors) > 0 {
						for _, v := range vectors {
							rec, ok := v.(map[string]any)
							if !ok {
								continue
							}
							id := ""
							if s, ok := rec["ChunkID"].(string); ok {
								id = strings.TrimSpace(s)
							}
							if id == "" {
								if s, ok := rec["chunk_id"].(string); ok {
									id = strings.TrimSpace(s)
								}
							}
							if id != chunkIDStr {
								continue
							}
							rec["Embedding"] = ksvc.HashEmbedding(content, dim)
							if m, ok := rec["Metadata"].(map[string]any); ok && m != nil {
								m["content_hash"] = ksvc.ContentHash(content)
								m["edited_at"] = now
								rec["Metadata"] = m
							}
							break
						}
						if out, err := json.MarshalIndent(vdoc, "", "  "); err == nil {
							_ = os.WriteFile(filepath.Clean(vectorManifestPath), out, 0o644)
							vectorBytes = out
						}
					}
				}
			}
		}
	}

	var maskingBytes []byte
	if strings.TrimSpace(bundle.MaskingReportURI) != "" {
		maskingPath := resolveArtifactURIToLocalPath(bundle.MaskingReportURI)
		if maskingPath != "" {
			if raw, err := os.ReadFile(filepath.Clean(maskingPath)); err == nil {
				maskingBytes = raw
			}
		}
	}

	sum := sha256.New()
	sum.Write(updatedChunkBytes)
	sum.Write(vectorBytes)
	sum.Write(maskingBytes)
	bundle.Checksum = hex.EncodeToString(sum.Sum(nil))
	_, _ = knowledgeRepo.NewArtifactBundleRepository(h.db).Update(c.Request.Context(), bundle)

	dto.ResponseSuccess(c, gin.H{
		"updated":   true,
		"chunkId":   chunkIDStr,
		"jobId":     jobID.String(),
		"spaceId":   spaceID.String(),
		"updatedAt": now,
	})
}

func (h *IngestionHandler) activeVectorDimensions(ctx context.Context, spaceID uuid.UUID) (int, error) {
	if h == nil || h.db == nil {
		return 0, errors.New("db not initialized")
	}
	rec, err := knowledgeRepo.NewKnowledgeVectorIndexRepository(h.db).FindActiveBySpace(ctx, spaceID)
	if err != nil {
		return 0, err
	}
	if rec == nil {
		return 0, errors.New("active vector index not found")
	}
	if rec.Dimensions <= 0 {
		return 0, fmt.Errorf("invalid active vector index dimensions: %d", rec.Dimensions)
	}
	return rec.Dimensions, nil
}

func (h *IngestionHandler) PageImage(c *gin.Context) {
	if h == nil || h.svc == nil || h.db == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	jobID, err := uuid.Parse(c.Param("jobId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "任务 ID 无效", err)
		return
	}
	pageNum, err := strconv.Atoi(strings.TrimSpace(c.Param("pageNumber")))
	if err != nil || pageNum <= 0 || pageNum > 2000 {
		dto.ResponseError(c, http.StatusBadRequest, "页码无效", errors.New("invalid page number"))
		return
	}
	job, err := h.svc.GetJob(c.Request.Context(), spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取入库任务失败", err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "入库任务不存在", errors.New("not found"))
		return
	}
	bundle, err := knowledgeRepo.NewArtifactBundleRepository(h.db).FindByJobID(c.Request.Context(), job.ID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "获取产物信息失败", err)
		return
	}
	if bundle == nil || strings.TrimSpace(bundle.OCRPageImagesURI) == "" {
		dto.ResponseError(c, http.StatusNotFound, "该任务未生成 OCR 页预览产物", errors.New("ocr pages unavailable"))
		return
	}

	manifestPath := resolveArtifactURIToLocalPath(bundle.OCRPageImagesURI)
	if manifestPath == "" {
		dto.ResponseError(c, http.StatusNotImplemented, "当前环境不支持读取该产物 URI", errors.New("unsupported artifact uri"))
		return
	}
	raw, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "读取页清单失败", err)
		return
	}
	var doc struct {
		Pages []struct {
			PageNumber int    `json:"page_number"`
			ImageURI   string `json:"image_uri"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "解析页清单失败", err)
		return
	}
	imageURI := ""
	for _, p := range doc.Pages {
		if p.PageNumber == pageNum {
			imageURI = strings.TrimSpace(p.ImageURI)
			break
		}
	}
	if imageURI == "" {
		dto.ResponseError(c, http.StatusNotFound, "未找到该页图片", errors.New("page not found"))
		return
	}
	imgPath := resolveArtifactURIToLocalPath(imageURI)
	if imgPath == "" {
		dto.ResponseError(c, http.StatusNotImplemented, "当前环境不支持读取该页图片 URI", errors.New("unsupported image uri"))
		return
	}
	imgBytes, err := os.ReadFile(filepath.Clean(imgPath))
	if err != nil {
		dto.ResponseError(c, http.StatusNotFound, "读取页图片失败", err)
		return
	}
	ext := strings.ToLower(filepath.Ext(imgPath))
	ct := "application/octet-stream"
	switch ext {
	case ".png":
		ct = "image/png"
	case ".jpg", ".jpeg":
		ct = "image/jpeg"
	}
	c.Data(http.StatusOK, ct, imgBytes)
}
