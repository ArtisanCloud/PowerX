package knowledge_space

import (
	"net/http"
	"strconv"

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
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
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
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	cases, err := h.svc.ListCasesFiltered(c.Request.Context(), spaceID, ksvc.ListFeedbackFilter{
		Status:   c.Query("status"),
		Severity: c.Query("severity"),
		Limit:    limit,
	})
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

func (h *FeedbackHandler) Close(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	caseID, err := uuid.Parse(c.Param("caseId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "案例 ID 无效", err)
		return
	}
	var req feedbackCaseActionRequest
	_ = c.ShouldBindJSON(&req)
	caseModel, err := h.svc.CloseCase(c.Request.Context(), ksvc.FeedbackCaseUpdateInput{
		SpaceID: spaceID,
		CaseID:  caseID,
		Actor:   req.RequestedBy,
		Notes:   req.ResolutionNotes,
	})
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "关闭反馈失败", err)
		}
		return
	}
	dto.ResponseSuccess(c, toFeedbackResponse(caseModel))
}

func (h *FeedbackHandler) Escalate(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	caseID, err := uuid.Parse(c.Param("caseId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "案例 ID 无效", err)
		return
	}
	var req feedbackCaseActionRequest
	_ = c.ShouldBindJSON(&req)
	caseModel, err := h.svc.EscalateCase(c.Request.Context(), ksvc.FeedbackCaseUpdateInput{
		SpaceID: spaceID,
		CaseID:  caseID,
		Actor:   req.RequestedBy,
		Notes:   req.Reason,
	})
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "升级反馈失败", err)
		}
		return
	}
	dto.ResponseSuccess(c, toFeedbackResponse(caseModel))
}

func (h *FeedbackHandler) Reprocess(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	caseID, err := uuid.Parse(c.Param("caseId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "案例 ID 无效", err)
		return
	}
	var req feedbackCaseActionRequest
	_ = c.ShouldBindJSON(&req)
	caseModel, err := h.svc.ReprocessCase(c.Request.Context(), spaceID, caseID, req.RequestedBy)
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "触发再加工失败", err)
		}
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, toFeedbackResponse(caseModel))
}

func (h *FeedbackHandler) Rollback(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	caseID, err := uuid.Parse(c.Param("caseId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "案例 ID 无效", err)
		return
	}
	var req feedbackCaseActionRequest
	_ = c.ShouldBindJSON(&req)
	caseModel, err := h.svc.RollbackCase(c.Request.Context(), spaceID, caseID, req.RequestedBy, req.Reason)
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "回滚失败", err)
		}
		return
	}
	dto.ResponseSuccess(c, toFeedbackResponse(caseModel))
}

func (h *FeedbackHandler) Export(c *gin.Context) {
	if h == nil || h.svc == nil {
		c.Status(http.StatusNotImplemented)
		return
	}
	spaceID, err := uuid.Parse(c.Param("spaceId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "空间 ID 无效", err)
		return
	}
	limit := 200
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	export, err := h.svc.ExportCases(c.Request.Context(), spaceID, ksvc.ListFeedbackFilter{
		Status:   c.Query("status"),
		Severity: c.Query("severity"),
		Limit:    limit,
	})
	if err != nil {
		switch {
		case err == ksvc.ErrInvalidInput:
			dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		case err == ksvc.ErrSpaceNotFound:
			dto.ResponseError(c, http.StatusGone, "知识空间已删除或退役，请迁移至新的知识空间", err)
		default:
			dto.ResponseError(c, http.StatusInternalServerError, "导出反馈失败", err)
		}
		return
	}
	dto.ResponseSuccess(c, export)
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
