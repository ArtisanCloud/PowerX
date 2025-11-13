package knowledge_space

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// FeedbackHandler exposes HTTP endpoints for feedback flows.
type FeedbackHandler struct {
	svc *ksvc.FeedbackService
}

func NewFeedbackHandler(deps *shared.Deps) *FeedbackHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Feedback == nil {
		return nil
	}
	return &FeedbackHandler{svc: deps.KnowledgeSpace.Feedback}
}

func (h *FeedbackHandler) Submit(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	var req feedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	chunkIDs, err := parseChunkIDs(req.LinkedChunks)
	if err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	caseModel, err := h.svc.SubmitFeedback(c.Request.Context(), ksvc.SubmitFeedbackInput{
		SpaceID:      spaceID,
		ReportedBy:   req.ReportedBy,
		Severity:     req.Severity,
		IssueType:    req.IssueType,
		Notes:        req.Notes,
		ToolTraceRef: req.ToolTraceRef,
		LinkedChunks: chunkIDs,
	})
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "提交反馈失败", err)
		}
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toFeedbackResponse(caseModel))
}

func (h *FeedbackHandler) List(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	cases, err := h.svc.ListCases(c.Request.Context(), spaceID, 50)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询反馈失败", err)
		return
	}
	resp := make([]feedbackResponse, 0, len(cases))
	for _, item := range cases {
		resp = append(resp, toFeedbackResponse(item))
	}
	dto.ResponseSuccess(c, resp)
}

func parseChunkIDs(ids []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil {
			return nil, err
		}
		out = append(out, parsed)
	}
	return out, nil
}
