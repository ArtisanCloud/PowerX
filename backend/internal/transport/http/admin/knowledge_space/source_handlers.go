package knowledge_space

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type SourceHandler struct {
	svc *ksvc.SourceSyncService
}

func NewSourceHandler(deps *shared.Deps) *SourceHandler {
	if deps == nil || deps.DB == nil || deps.KnowledgeSpace == nil {
		return nil
	}
	return &SourceHandler{
		svc: ksvc.NewSourceSyncService(ksvc.SourceSyncServiceOptions{
			DB:        deps.DB,
			Ingestion: deps.KnowledgeSpace.Ingestion,
			Client:    nil,
		}),
	}
}

type createCredentialRequest struct {
	Provider  string `json:"provider" binding:"required,oneof=notion feishu"`
	AuthType  string `json:"authType" binding:"omitempty,oneof=token oauth"`
	Label     string `json:"label" binding:"required"`
	BaseURL   string `json:"baseUrl"`
	Token     string `json:"token"`
	CreatedBy string `json:"createdBy"`
}

type credentialView struct {
	ID         string `json:"id"`
	TenantUUID string `json:"tenant_uuid"`
	Provider   string `json:"provider"`
	AuthType   string `json:"authType"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	MaskedHint string `json:"maskedHint,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

type createConnectorRequest struct {
	Provider      string         `json:"provider" binding:"required,oneof=notion feishu"`
	CredentialID  string         `json:"credentialId" binding:"required"`
	Config        map[string]any `json:"config"`
	CreatedBy     string         `json:"createdBy"`
	WebhookKeyRef string         `json:"webhookKeyRef"`
}

type updateConnectorRequest struct {
	CredentialID string         `json:"credentialId"`
	Config       map[string]any `json:"config"`
	Status       string         `json:"status" binding:"omitempty,oneof=active paused"`
	UpdatedBy    string         `json:"updatedBy"`
}

type connectorView struct {
	ID           string `json:"id"`
	TenantUUID   string `json:"tenant_uuid"`
	Provider     string `json:"provider"`
	CredentialID string `json:"credentialId"`
	Status       string `json:"status"`
	LastError    string `json:"lastError,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type pauseRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

type createSyncJobRequest struct {
	Provider    string         `json:"provider" binding:"required,oneof=notion feishu"`
	ConnectorID string         `json:"connectorId" binding:"required"`
	SyncMode    string         `json:"syncMode" binding:"omitempty,oneof=incremental full_then_incremental"`
	Schedule    string         `json:"schedule"`
	Scope       map[string]any `json:"scope"`
	CreatedBy   string         `json:"createdBy"`
}

type syncJobView struct {
	ID          string         `json:"id"`
	TenantUUID  string         `json:"tenant_uuid"`
	SpaceID     string         `json:"spaceId"`
	Provider    string         `json:"provider"`
	ConnectorID string         `json:"connectorId"`
	SyncMode    string         `json:"syncMode"`
	Schedule    string         `json:"schedule"`
	Status      string         `json:"status"`
	Scope       map[string]any `json:"scope"`
	LastRunAt   string         `json:"lastRunAt,omitempty"`
	LastOKAt    string         `json:"lastOkAt,omitempty"`
	LastError   string         `json:"lastError,omitempty"`
	LastRunRef  string         `json:"lastRunRef,omitempty"`
	CreatedAt   string         `json:"createdAt,omitempty"`
	UpdatedAt   string         `json:"updatedAt,omitempty"`
}

type spaceSourcesView struct {
	Provider   string         `json:"provider"`
	Credential credentialView `json:"credential"`
	Connector  connectorView  `json:"connector"`
	Jobs       []syncJobView  `json:"jobs"`
}

func (h *SourceHandler) ListCredentials(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	provider := strings.TrimSpace(c.Query("provider"))
	items, err := h.svc.ListCredentials(c.Request.Context(), tenant, ksvc.SourceProvider(provider), 200)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}
	out := make([]credentialView, 0, len(items))
	for i := range items {
		out = append(out, toCredentialView(items[i]))
	}
	dto.ResponseSuccess(c, out)
}

func (h *SourceHandler) CreateCredential(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	var req createCredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	row, err := h.svc.CreateCredential(c.Request.Context(), ksvc.SourceCredentialInput{
		TenantUUID: tenant,
		Provider:   ksvc.SourceProvider(req.Provider),
		AuthType:   req.AuthType,
		Label:      req.Label,
		BaseURL:    req.BaseURL,
		Token:      req.Token,
		CreatedBy:  req.CreatedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toCredentialView(*row))
}

func (h *SourceHandler) CreateConnector(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	var req createConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	credID, err := uuid.Parse(strings.TrimSpace(req.CredentialID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 credentialId", err)
		return
	}
	row, err := h.svc.CreateConnector(c.Request.Context(), ksvc.SourceConnectorInput{
		TenantUUID:     tenant,
		Provider:       ksvc.SourceProvider(req.Provider),
		CredentialUUID: credID,
		Config:         req.Config,
		CreatedBy:      req.CreatedBy,
		WebhookKeyRef:  req.WebhookKeyRef,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toConnectorView(*row))
}

func (h *SourceHandler) PauseConnector(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("connectorId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 connectorId", err)
		return
	}
	var req pauseRequest
	_ = c.ShouldBindJSON(&req)
	row, err := h.svc.PauseConnector(c.Request.Context(), tenant, id, req.Reason, req.RequestedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "操作失败", err)
		return
	}
	dto.ResponseSuccess(c, toConnectorView(*row))
}

func (h *SourceHandler) ListConnectors(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	provider := strings.TrimSpace(c.Query("provider"))
	items, err := h.svc.ListConnectors(c.Request.Context(), tenant, ksvc.SourceProvider(provider), 200)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}
	out := make([]connectorView, 0, len(items))
	for i := range items {
		out = append(out, toConnectorView(items[i]))
	}
	dto.ResponseSuccess(c, out)
}

func (h *SourceHandler) GetConnector(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("connectorId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 connectorId", err)
		return
	}
	conn, err := h.svc.GetConnectorForTenant(c.Request.Context(), tenant, id)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询失败", err)
		return
	}
	if conn == nil {
		dto.ResponseError(c, http.StatusNotFound, "连接器不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toConnectorView(*conn))
}

func (h *SourceHandler) UpdateConnector(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	id, err := uuid.Parse(strings.TrimSpace(c.Param("connectorId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 connectorId", err)
		return
	}
	var req updateConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	var credUUID *uuid.UUID
	if strings.TrimSpace(req.CredentialID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(req.CredentialID))
		if err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "无效的 credentialId", err)
			return
		}
		credUUID = &parsed
	}
	row, err := h.svc.UpdateConnector(c.Request.Context(), tenant, id, ksvc.UpdateConnectorInput{
		CredentialUUID: credUUID,
		Config:         req.Config,
		Status:         req.Status,
		UpdatedBy:      req.UpdatedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "更新失败", err)
		return
	}
	if row == nil {
		dto.ResponseError(c, http.StatusNotFound, "连接器不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toConnectorView(*row))
}

func (h *SourceHandler) ListSpaceSources(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 spaceId", err)
		return
	}
	jobs, err := h.svc.ListSyncJobsForTenant(c.Request.Context(), tenant, spaceID, 200)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", err)
		return
	}

	// Minimal view: expose jobs only; connector/credential are resolved during run.
	out := make([]syncJobView, 0, len(jobs))
	for i := range jobs {
		out = append(out, toSyncJobView(jobs[i]))
	}
	dto.ResponseSuccess(c, gin.H{
		"tenant_uuid": tenant,
		"spaceId":     spaceID.String(),
		"jobs":        out,
	})
}

func (h *SourceHandler) CreateSyncJob(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 spaceId", err)
		return
	}
	var req createSyncJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	connID, err := uuid.Parse(strings.TrimSpace(req.ConnectorID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 connectorId", err)
		return
	}
	job, err := h.svc.CreateSyncJob(c.Request.Context(), ksvc.SpaceSyncJobInput{
		TenantUUID:    tenant,
		SpaceUUID:     spaceID,
		Provider:      ksvc.SourceProvider(req.Provider),
		ConnectorUUID: connID,
		SyncMode:      req.SyncMode,
		Schedule:      req.Schedule,
		Scope:         req.Scope,
		CreatedBy:     req.CreatedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "参数不合法", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toSyncJobView(*job))
}

func (h *SourceHandler) PauseSyncJob(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 spaceId", err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(c.Param("jobId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 jobId", err)
		return
	}
	var req pauseRequest
	_ = c.ShouldBindJSON(&req)
	job, err := h.svc.PauseSyncJob(c.Request.Context(), tenant, spaceID, jobID, req.Reason, req.RequestedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "操作失败", err)
		return
	}
	dto.ResponseSuccess(c, toSyncJobView(*job))
}

func (h *SourceHandler) RunSyncJob(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 spaceId", err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(c.Param("jobId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 jobId", err)
		return
	}
	var req pauseRequest
	_ = c.ShouldBindJSON(&req)
	res, err := h.svc.RunSyncJob(c.Request.Context(), tenant, spaceID, jobID, req.RequestedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "同步失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, res)
}

func (h *SourceHandler) GetSyncJob(c *gin.Context) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", nil)
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 spaceId", err)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSpace(c.Param("jobId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 jobId", err)
		return
	}
	job, err := h.svc.GetSyncJobForTenant(c.Request.Context(), tenant, spaceID, jobID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "查询失败", err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "同步任务不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toSyncJobView(*job))
}

func toCredentialView(item models.SourceCredential) credentialView {
	meta := map[string]any{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &meta)
	}
	masked := ""
	if v, ok := meta["masked_hint"].(string); ok {
		masked = v
	}
	return credentialView{
		ID:         item.UUID.String(),
		TenantUUID: item.TenantUUID,
		Provider:   item.Provider,
		AuthType:   item.AuthType,
		Label:      item.Label,
		Status:     item.Status,
		MaskedHint: masked,
		CreatedAt:  item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  item.UpdatedAt.Format(time.RFC3339),
	}
}

func toConnectorView(item models.SourceConnectorInstance) connectorView {
	return connectorView{
		ID:           item.UUID.String(),
		TenantUUID:   item.TenantUUID,
		Provider:     item.Provider,
		CredentialID: item.CredentialUUID,
		Status:       item.Status,
		LastError:    item.LastError,
		CreatedAt:    item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    item.UpdatedAt.Format(time.RFC3339),
	}
}

func toSyncJobView(item models.SpaceSyncJob) syncJobView {
	scope := map[string]any{}
	if len(item.Scope) > 0 {
		_ = json.Unmarshal(item.Scope, &scope)
	}
	view := syncJobView{
		ID:          item.UUID.String(),
		TenantUUID:  item.TenantUUID,
		SpaceID:     item.SpaceUUID,
		Provider:    item.Provider,
		ConnectorID: item.ConnectorUUID,
		SyncMode:    item.SyncMode,
		Schedule:    item.Schedule,
		Status:      item.Status,
		Scope:       scope,
		LastError:   item.LastError,
		LastRunRef:  item.LastRunRef,
		CreatedAt:   item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   item.UpdatedAt.Format(time.RFC3339),
	}
	if item.LastRunAt != nil {
		view.LastRunAt = item.LastRunAt.Format(time.RFC3339)
	}
	if item.LastOKAt != nil {
		view.LastOKAt = item.LastOKAt.Format(time.RFC3339)
	}
	return view
}
