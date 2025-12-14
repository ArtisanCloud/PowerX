package knowledge_space

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/context_snapshot"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/qa_bridge"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/toolchain"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type qaBridgeHandler struct {
	svc *qa_bridge.Service
}

func newQABridgeHandler(deps *shared.Deps) *qaBridgeHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.QABridge == nil {
		return nil
	}
	return &qaBridgeHandler{svc: deps.KnowledgeSpace.QABridge}
}

type qaPlanRequest struct {
	Intent          string   `json:"intent" binding:"required"`
	DomainTags      []string `json:"domainTags"`
	SessionID       string   `json:"sessionId"`
	LatencyBudgetMs int      `json:"latencyBudgetMs"`
}

type qaPlanResponse struct {
	TenantUUID      string               `json:"tenant_uuid"`
	Intent          string               `json:"intent"`
	DomainTags      []string             `json:"domainTags"`
	CandidateSpaces []qaPlanSpaceView    `json:"candidateSpaces"`
	Tooling         []qaToolMetadataView `json:"tooling"`
	Telemetry       qaPlanTelemetry      `json:"telemetry"`
	DegradeCount    int                  `json:"degradeCount"`
	SessionID       string               `json:"sessionId"`
	LatencyBudgetMs int                  `json:"latencyBudgetMs"`
	Metadata        map[string]any       `json:"metadata"`
}

type qaPlanSpaceView struct {
	SpaceID          string  `json:"spaceId"`
	SpaceName        string  `json:"spaceName"`
	Strategy         string  `json:"strategy"`
	CitationCoverage float64 `json:"citationCoverage"`
	DegradeReason    string  `json:"degradeReason"`
}

type qaToolMetadataView struct {
	ToolID   string `json:"toolId"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Endpoint string `json:"endpoint"`
}

type qaPlanTelemetry struct {
	TraceID    string `json:"traceId"`
	RecordedAt string `json:"recordedAt"`
}

func (h *qaBridgeHandler) plan(c *gin.Context) {
	var req qaPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	out, err := h.svc.Plan(c.Request.Context(), qa_bridge.PlanInput{
		TenantUUID:      tenantUUID,
		Intent:          req.Intent,
		DomainTags:      req.DomainTags,
		SessionID:       req.SessionID,
		LatencyBudgetMs: req.LatencyBudgetMs,
	})
	if err != nil {
		if errors.Is(err, qa_bridge.ErrInvalidInput) {
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
			return
		}
		if errors.Is(err, qa_bridge.ErrSpacesMissing) {
			dto.ResponseError(c, http.StatusNotFound, "未找到知识空间", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}
	dto.ResponseSuccess(c, qaPlanResponse{
		TenantUUID:      out.TenantUUID.String(),
		Intent:          out.Intent,
		DomainTags:      out.DomainTags,
		CandidateSpaces: toPlanSpaces(out.CandidateSpaces),
		Tooling:         toToolingView(out.Toolings),
		Telemetry: qaPlanTelemetry{
			TraceID:    out.TraceID,
			RecordedAt: out.RecordedAt.Format(time.RFC3339Nano),
		},
		DegradeCount:    out.DegradeCount,
		SessionID:       out.SessionID,
		LatencyBudgetMs: out.LatencyBudgetMs,
		Metadata:        out.Metadata,
	})
}

type qaMemoryRequest struct {
	SessionID string               `json:"sessionId" binding:"required"`
	Updates   []qaMemoryUpdateView `json:"updates"`
}

type qaMemoryUpdateView struct {
	ChunkID     string   `json:"chunkId"`
	SpaceID     string   `json:"spaceId"`
	Status      string   `json:"status"`
	Citations   []string `json:"citations"`
	SourceType  string   `json:"sourceType"`
	Confidence  float64  `json:"confidence"`
	DeltaReason string   `json:"deltaReason"`
}

type qaMemoryResponse struct {
	TenantUUID string               `json:"tenant_uuid"`
	SessionID  string               `json:"sessionId"`
	Citations  []qaMemoryUpdateView `json:"citations"`
}

func (h *qaBridgeHandler) memorySnapshot(c *gin.Context) {
	var req qaMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	input := qa_bridge.MemoryInput{
		TenantUUID: tenantUUID,
		SessionID:  req.SessionID,
		Updates:    fromMemoryView(req.Updates),
	}
	out, err := h.svc.UpsertMemorySnapshot(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, qa_bridge.ErrInvalidInput) {
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}
	dto.ResponseSuccess(c, qaMemoryResponse{
		TenantUUID: out.TenantUUID.String(),
		SessionID:  out.SessionID,
		Citations:  toMemoryView(out.Citations),
	})
}

func (h *qaBridgeHandler) requireTenantUUID(c *gin.Context) (uuid.UUID, bool) {
	uuidStr, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return uuid.Nil, false
	}
	trimmed := strings.TrimSpace(uuidStr)
	parsed, parseErr := uuid.Parse(trimmed)
	if parseErr != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid 格式错误", parseErr)
		return uuid.Nil, false
	}
	return parsed, true
}

func toPlanSpaces(items []qa_bridge.CandidateSpace) []qaPlanSpaceView {
	out := make([]qaPlanSpaceView, 0, len(items))
	for _, item := range items {
		out = append(out, qaPlanSpaceView{
			SpaceID:          item.SpaceID.String(),
			SpaceName:        item.SpaceName,
			Strategy:         item.Strategy,
			CitationCoverage: item.CitationCoverage,
			DegradeReason:    item.DegradeReason,
		})
	}
	return out
}

func toToolingView(items []toolchain.Metadata) []qaToolMetadataView {
	out := make([]qaToolMetadataView, 0, len(items))
	for _, item := range items {
		out = append(out, qaToolMetadataView{
			ToolID:   item.ToolID,
			Name:     item.Name,
			Category: item.Category,
			Endpoint: item.Endpoint,
		})
	}
	return out
}

func fromMemoryView(items []qaMemoryUpdateView) []context_snapshot.Citation {
	out := make([]context_snapshot.Citation, 0, len(items))
	for _, item := range items {
		out = append(out, context_snapshot.Citation{
			ChunkID:     item.ChunkID,
			SpaceID:     item.SpaceID,
			Status:      item.Status,
			Citations:   item.Citations,
			SourceType:  item.SourceType,
			Confidence:  item.Confidence,
			DeltaReason: item.DeltaReason,
		})
	}
	return out
}

func toMemoryView(items []context_snapshot.Citation) []qaMemoryUpdateView {
	out := make([]qaMemoryUpdateView, 0, len(items))
	for _, item := range items {
		out = append(out, qaMemoryUpdateView{
			ChunkID:     item.ChunkID,
			SpaceID:     item.SpaceID,
			Status:      item.Status,
			Citations:   item.Citations,
			SourceType:  item.SourceType,
			Confidence:  item.Confidence,
			DeltaReason: item.DeltaReason,
		})
	}
	return out
}
