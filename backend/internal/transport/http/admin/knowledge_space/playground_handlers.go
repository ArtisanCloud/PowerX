package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type playgroundHandler struct {
	db     *gorm.DB
	vector vectorstore.Store
}

func newPlaygroundHandler(db *gorm.DB, vector vectorstore.Store) *playgroundHandler {
	if db == nil || vector == nil {
		return nil
	}
	return &playgroundHandler{db: db, vector: vector}
}

type retrievalPlaygroundRequest struct {
	RAGProfileUUID string            `json:"ragProfileUuid"`
	Query          string            `json:"query" binding:"required"`
	TopK           int               `json:"topK"`
	MinScore       float64           `json:"minScore"`
	Filters        map[string]string `json:"filters"`
}

type retrievalCandidate struct {
	ChunkID  string         `json:"chunkId"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Text     string         `json:"text,omitempty"`
}

type retrievalStage struct {
	Name           string `json:"name"`
	CandidateCount int    `json:"candidateCount"`
	LatencyMs      int    `json:"latencyMs"`
	DegradeReason  string `json:"degradeReason,omitempty"`
}

type retrievalPlaygroundResponse struct {
	TraceID      string              `json:"traceId"`
	SpaceID      string              `json:"spaceId"`
	Query        string              `json:"query"`
	Profile      map[string]any      `json:"profile"`
	RetrievalPlan map[string]any     `json:"retrievalPlan"`
	Stages       []retrievalStage    `json:"stages"`
	Candidates   []retrievalCandidate `json:"candidates"`
	ContextPack  map[string]any      `json:"context_pack"`
}

func (h *playgroundHandler) Retrieve(c *gin.Context) {
	var req retrievalPlaygroundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return
	}
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))

	space, err := repo.NewKnowledgeSpaceRepository(h.db).FindByUUID(c.Request.Context(), spaceID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询空间失败", err)
		return
	}
	if space == nil || !strings.EqualFold(space.TenantUUID, tenantUUID) {
		dto.ResponseError(c, http.StatusNotFound, "空间不存在", nil)
		return
	}

	profileInfo := map[string]any{
		"profileKey": strings.TrimSpace(space.RAGProfileKey),
	}
	topK := req.TopK
	minScore := req.MinScore

	ragRepo := repo.NewRAGProfileVersionRepository(h.db)
	if strings.TrimSpace(req.RAGProfileUUID) != "" {
		pid, parseErr := uuid.Parse(strings.TrimSpace(req.RAGProfileUUID))
		if parseErr != nil {
			dto.ResponseError(c, http.StatusBadRequest, "无效的 ragProfileUuid", parseErr)
			return
		}
		row, err := ragRepo.GetByUUID(c.Request.Context(), pid)
		if err != nil {
			dto.ResponseError(c, http.StatusInternalServerError, "查询 profile 失败", err)
			return
		}
		if row == nil || !strings.EqualFold(row.TenantUUID, tenantUUID) {
			dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
			return
		}
		profileInfo["uuid"] = row.UUID.String()
		profileInfo["status"] = row.Status
		profileInfo["version"] = row.Version
		profileInfo["displayName"] = row.DisplayName
		applyRAGDefaults(row.Config, &topK, &minScore)
	} else {
		latest, _ := ragRepo.FindLatestPublished(c.Request.Context(), tenantUUID, strings.TrimSpace(space.RAGProfileKey))
		if latest != nil {
			profileInfo["uuid"] = latest.UUID.String()
			profileInfo["status"] = latest.Status
			profileInfo["version"] = latest.Version
			profileInfo["displayName"] = latest.DisplayName
			applyRAGDefaults(latest.Config, &topK, &minScore)
		}
	}

	if topK <= 0 {
		topK = 10
	}
	if topK > 50 {
		topK = 50
	}
	if minScore <= 0 {
		minScore = 0
	}

	traceID := uuid.NewString()
	queryHash := sha256Sum(strings.TrimSpace(req.Query))
	start := time.Now()
	embedding := hashEmbedding(strings.TrimSpace(req.Query), 32)
	resp, err := h.vector.Query(c.Request.Context(), vectorstore.QueryRequest{
		SpaceID:   spaceID,
		Embedding: embedding,
		TopK:      topK,
		Filters:   req.Filters,
		MinScore:  minScore,
	})
	latencyMs := int(time.Since(start).Milliseconds())
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "向量检索失败", err)
		return
	}

	matches := resp.Matches
	ids := make([]uuid.UUID, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m.ChunkID)
	}
	textMap := h.tryLoadChunkTexts(c.Request.Context(), spaceID, ids)

	candidates := make([]retrievalCandidate, 0, len(matches))
	for _, m := range matches {
		c := retrievalCandidate{
			ChunkID:  m.ChunkID.String(),
			Score:    m.Score,
			Metadata: m.Metadata,
		}
		if txt, ok := textMap[m.ChunkID.String()]; ok {
			c.Text = txt
		}
		candidates = append(candidates, c)
	}

	dto.ResponseSuccess(c, retrievalPlaygroundResponse{
		TraceID: traceID,
		SpaceID: spaceID.String(),
		Query:   req.Query,
		Profile: profileInfo,
		RetrievalPlan: map[string]any{
			"driver":        "vector",
			"top_k":         topK,
			"min_score":     minScore,
			"filters":       req.Filters,
			"embedding_dim": len(embedding),
		},
		Stages: []retrievalStage{
			{Name: "vector_search", CandidateCount: len(candidates), LatencyMs: latencyMs},
		},
		Candidates: candidates,
		ContextPack: map[string]any{
			"chunk_count": len(candidates),
			"min_score":   minScore,
			"top_k":       topK,
			"query_hash":  fmt.Sprintf("%x", queryHash),
		},
	})
}

func applyRAGDefaults(raw []byte, topK *int, minScore *float64) {
	if len(raw) == 0 {
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return
	}
	if v, ok := cfg["top_k"].(float64); ok && topK != nil && *topK == 0 {
		*topK = int(v)
	}
	if v, ok := cfg["min_score"].(float64); ok && minScore != nil && *minScore == 0 {
		*minScore = v
	}
}

func hashEmbedding(text string, dims int) []float32 {
	if dims <= 0 {
		dims = 32
	}
	sum := sha256Sum(text)
	out := make([]float32, 0, dims)
	for i := 0; i < dims; i++ {
		b := sum[i%len(sum)]
		out = append(out, (float32(b)/255.0)*2.0-1.0)
	}
	return out
}

func sha256Sum(text string) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte(text))
	return h.Sum(nil)
}

func (h *playgroundHandler) tryLoadChunkTexts(ctx context.Context, spaceID uuid.UUID, chunkIDs []uuid.UUID) map[string]string {
	_ = ctx
	out := make(map[string]string)
	if h == nil || h.db == nil || len(chunkIDs) == 0 {
		return out
	}
	latestBundleURI := findLatestChunkManifestURI(h.db, spaceID)
	if latestBundleURI == "" {
		return out
	}
	path := resolveArtifactURIToLocalPath(latestBundleURI)
	if path == "" {
		return out
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Chunks []struct {
			ID      string `json:"ID"`
			Content string `json:"Content"`
			IDAlt      string `json:"id"`
			ContentAlt string `json:"content"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return out
	}
	need := make(map[string]struct{}, len(chunkIDs))
	for _, id := range chunkIDs {
		need[id.String()] = struct{}{}
	}
	for _, c := range doc.Chunks {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			id = strings.TrimSpace(c.IDAlt)
		}
		if id == "" {
			continue
		}
		content := c.Content
		if strings.TrimSpace(content) == "" {
			content = c.ContentAlt
		}
		if _, ok := need[id]; ok {
			out[id] = content
		}
	}
	return out
}

func findLatestChunkManifestURI(db *gorm.DB, spaceID uuid.UUID) string {
	if db == nil || spaceID == uuid.Nil {
		return ""
	}
	type row struct {
		ChunkManifestURI string
	}
	var r row
	bundleTable := (&models.ArtifactBundle{}).TableName()
	jobTable := (&models.IngestionJob{}).TableName()
	join := fmt.Sprintf("JOIN %s ON %s.id = %s.ingestion_job_id", jobTable, jobTable, bundleTable)
	err := db.Table(bundleTable).
		Select(fmt.Sprintf("%s.chunk_manifest_uri", bundleTable)).
		Joins(join).
		Where(fmt.Sprintf("%s.space_uuid = ? AND %s.chunk_manifest_uri <> ''", jobTable, bundleTable), spaceID).
		Order(fmt.Sprintf("%s.id DESC", bundleTable)).
		Limit(1).
		Scan(&r).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(r.ChunkManifestURI)
}

func resolveArtifactURIToLocalPath(uri string) string {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "minio://") {
		return ""
	}
	trimmed := strings.TrimPrefix(uri, "minio://")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	bucket := parts[0]
	key := parts[1]
	// 与 ArtifactStore 的默认落盘路径保持一致
	baseDir := filepath.Join("backend", "reports", "_state", "knowledge-artifacts")
	return filepath.Join(baseDir, bucket, filepath.FromSlash(key))
}
