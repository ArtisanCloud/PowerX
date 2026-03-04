package knowledge_space

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type corpusCheckHandler struct {
	svc *ksvc.CorpusCheckService
}

func newCorpusCheckHandler(svc *ksvc.CorpusCheckService) *corpusCheckHandler {
	if svc == nil {
		return nil
	}
	return &corpusCheckHandler{svc: svc}
}

type corpusCheckStartRequest struct {
	RequestedBy string `json:"requestedBy"`
}

func (h *corpusCheckHandler) Start(c *gin.Context) {
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	var req corpusCheckStartRequest
	_ = c.ShouldBindJSON(&req)
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return
	}
	tenantUUID = strings.ToLower(strings.TrimSpace(tenantUUID))
	job, err := h.svc.Start(c.Request.Context(), tenantUUID, spaceID, req.RequestedBy)
	if err != nil {
		if errors.Is(err, ksvc.ErrInvalidInput) {
			dto.ResponseError(c, http.StatusBadRequest, "入参不合法", err)
			return
		}
		// Best-effort: even if scheduling fails (e.g. Event Fabric misconfig), the job record is created and updated to failed.
		// Return 202 so the UI can continue (space creation should not be blocked by corpus-check).
		if job != nil {
			dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "启动失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, job)
}

func (h *corpusCheckHandler) Get(c *gin.Context) {
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(c.Param("jobId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 job ID", err)
		return
	}
	job, err := h.svc.Get(c.Request.Context(), jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	if job == nil || job.SpaceUUID != spaceID {
		dto.ResponseError(c, http.StatusNotFound, "job 不存在", nil)
		return
	}
	dto.ResponseSuccess(c, job)
}
