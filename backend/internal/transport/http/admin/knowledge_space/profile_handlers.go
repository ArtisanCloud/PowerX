package knowledge_space

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type profileVersionView struct {
	UUID           string          `json:"uuid"`
	ProfileKey     string          `json:"profileKey"`
	Version        int             `json:"version"`
	Status         string          `json:"status"`
	DisplayName    string          `json:"displayName"`
	Config         datatypes.JSON  `json:"config"`
	RollbackFromID uint64          `json:"rollbackFromId,omitempty"`
	PublishedAt    *time.Time      `json:"publishedAt,omitempty"`
	PublishedBy    string          `json:"publishedBy,omitempty"`
	CreatedBy      string          `json:"createdBy,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

func toProfileVersionView(uuidVal uuid.UUID, profileKey string, version int, status, displayName string, config datatypes.JSON, rollbackFromID uint64, publishedAt *time.Time, publishedBy, createdBy string, createdAt, updatedAt time.Time) profileVersionView {
	return profileVersionView{
		UUID:           uuidVal.String(),
		ProfileKey:     strings.TrimSpace(profileKey),
		Version:        version,
		Status:         strings.TrimSpace(status),
		DisplayName:    strings.TrimSpace(displayName),
		Config:         config,
		RollbackFromID: rollbackFromID,
		PublishedAt:    publishedAt,
		PublishedBy:    strings.TrimSpace(publishedBy),
		CreatedBy:      strings.TrimSpace(createdBy),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

type createProfileVersionRequest struct {
	ProfileKey  string         `json:"profileKey" binding:"required"`
	DisplayName string         `json:"displayName"`
	Config      datatypes.JSON `json:"config"`
	CreatedBy   string         `json:"createdBy"`
}

type publishProfileVersionRequest struct {
	PublishedBy string `json:"publishedBy"`
}

type rollbackProfileVersionRequest struct {
	CreatedBy string `json:"createdBy"`
}

type profileHandler struct {
	db *gorm.DB
}

func newProfileHandler(db *gorm.DB) *profileHandler {
	if db == nil {
		return nil
	}
	return &profileHandler{db: db}
}

func (h *profileHandler) routes(group *gin.RouterGroup) {
	if group == nil {
		return
	}
	group.GET("/ingestion/versions", h.listIngestion)
	group.POST("/ingestion/versions", h.createIngestion)
	group.POST("/ingestion/versions/:uuid/publish", h.publishIngestion)
	group.POST("/ingestion/versions/:uuid/rollback", h.rollbackIngestion)

	group.GET("/index/versions", h.listIndex)
	group.POST("/index/versions", h.createIndex)
	group.POST("/index/versions/:uuid/publish", h.publishIndex)
	group.POST("/index/versions/:uuid/rollback", h.rollbackIndex)

	group.GET("/rag/versions", h.listRAG)
	group.POST("/rag/versions", h.createRAG)
	group.POST("/rag/versions/:uuid/publish", h.publishRAG)
	group.POST("/rag/versions/:uuid/rollback", h.rollbackRAG)
}

func (h *profileHandler) requireTenantUUID(c *gin.Context) (string, bool) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少租户上下文", err)
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(tenantUUID)), true
}

func (h *profileHandler) listIngestion(c *gin.Context) {
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	profileKey := strings.TrimSpace(c.Query("profile_key"))
	if profileKey == "" {
		profileKey = "default"
	}
	status := strings.TrimSpace(c.Query("status"))
	repo := repo.NewIngestionProfileVersionRepository(h.db)
	rows, err := repo.ListByKey(c.Request.Context(), tenantUUID, profileKey, status, 50)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	out := make([]profileVersionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfileVersionView(r.UUID, r.ProfileKey, r.Version, r.Status, r.DisplayName, r.Config, r.RollbackFromID, r.PublishedAt, r.PublishedBy, r.CreatedBy, r.CreatedAt, r.UpdatedAt))
	}
	dto.ResponseSuccess(c, out)
}

func (h *profileHandler) createIngestion(c *gin.Context) {
	var req createProfileVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	repo := repo.NewIngestionProfileVersionRepository(h.db)
	created, err := repo.CreateDraft(c.Request.Context(), tenantUUID, req.ProfileKey, req.DisplayName, req.Config, req.CreatedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "创建失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}

func (h *profileHandler) publishIngestion(c *gin.Context) {
	var req publishProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewIngestionProfileVersionRepository(h.db)
	updated, err := repo.Publish(c.Request.Context(), profileUUID, req.PublishedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "发布失败", err)
		return
	}
	if updated == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toProfileVersionView(updated.UUID, updated.ProfileKey, updated.Version, updated.Status, updated.DisplayName, updated.Config, updated.RollbackFromID, updated.PublishedAt, updated.PublishedBy, updated.CreatedBy, updated.CreatedAt, updated.UpdatedAt))
}

func (h *profileHandler) rollbackIngestion(c *gin.Context) {
	var req rollbackProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewIngestionProfileVersionRepository(h.db)
	created, err := repo.CreateRollbackDraft(c.Request.Context(), profileUUID, req.CreatedBy)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "profile 不存在", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "回滚草稿创建失败", err)
		return
	}
	if created == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}

func (h *profileHandler) listIndex(c *gin.Context) {
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	profileKey := strings.TrimSpace(c.Query("profile_key"))
	if profileKey == "" {
		profileKey = "default"
	}
	status := strings.TrimSpace(c.Query("status"))
	repo := repo.NewIndexProfileVersionRepository(h.db)
	rows, err := repo.ListByKey(c.Request.Context(), tenantUUID, profileKey, status, 50)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	out := make([]profileVersionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfileVersionView(r.UUID, r.ProfileKey, r.Version, r.Status, r.DisplayName, r.Config, r.RollbackFromID, r.PublishedAt, r.PublishedBy, r.CreatedBy, r.CreatedAt, r.UpdatedAt))
	}
	dto.ResponseSuccess(c, out)
}

func (h *profileHandler) createIndex(c *gin.Context) {
	var req createProfileVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	repo := repo.NewIndexProfileVersionRepository(h.db)
	created, err := repo.CreateDraft(c.Request.Context(), tenantUUID, req.ProfileKey, req.DisplayName, req.Config, req.CreatedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "创建失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}

func (h *profileHandler) publishIndex(c *gin.Context) {
	var req publishProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewIndexProfileVersionRepository(h.db)
	updated, err := repo.Publish(c.Request.Context(), profileUUID, req.PublishedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "发布失败", err)
		return
	}
	if updated == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toProfileVersionView(updated.UUID, updated.ProfileKey, updated.Version, updated.Status, updated.DisplayName, updated.Config, updated.RollbackFromID, updated.PublishedAt, updated.PublishedBy, updated.CreatedBy, updated.CreatedAt, updated.UpdatedAt))
}

func (h *profileHandler) rollbackIndex(c *gin.Context) {
	var req rollbackProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewIndexProfileVersionRepository(h.db)
	created, err := repo.CreateRollbackDraft(c.Request.Context(), profileUUID, req.CreatedBy)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "profile 不存在", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "回滚草稿创建失败", err)
		return
	}
	if created == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}

func (h *profileHandler) listRAG(c *gin.Context) {
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	profileKey := strings.TrimSpace(c.Query("profile_key"))
	if profileKey == "" {
		profileKey = "default"
	}
	status := strings.TrimSpace(c.Query("status"))
	repo := repo.NewRAGProfileVersionRepository(h.db)
	rows, err := repo.ListByKey(c.Request.Context(), tenantUUID, profileKey, status, 50)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "查询失败", err)
		return
	}
	out := make([]profileVersionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfileVersionView(r.UUID, r.ProfileKey, r.Version, r.Status, r.DisplayName, r.Config, r.RollbackFromID, r.PublishedAt, r.PublishedBy, r.CreatedBy, r.CreatedAt, r.UpdatedAt))
	}
	dto.ResponseSuccess(c, out)
}

func (h *profileHandler) createRAG(c *gin.Context) {
	var req createProfileVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, ok := h.requireTenantUUID(c)
	if !ok {
		return
	}
	repo := repo.NewRAGProfileVersionRepository(h.db)
	created, err := repo.CreateDraft(c.Request.Context(), tenantUUID, req.ProfileKey, req.DisplayName, req.Config, req.CreatedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "创建失败", err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}

func (h *profileHandler) publishRAG(c *gin.Context) {
	var req publishProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewRAGProfileVersionRepository(h.db)
	updated, err := repo.Publish(c.Request.Context(), profileUUID, req.PublishedBy)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "发布失败", err)
		return
	}
	if updated == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccess(c, toProfileVersionView(updated.UUID, updated.ProfileKey, updated.Version, updated.Status, updated.DisplayName, updated.Config, updated.RollbackFromID, updated.PublishedAt, updated.PublishedBy, updated.CreatedBy, updated.CreatedAt, updated.UpdatedAt))
}

func (h *profileHandler) rollbackRAG(c *gin.Context) {
	var req rollbackProfileVersionRequest
	_ = c.ShouldBindJSON(&req)
	profileUUID, err := uuid.Parse(strings.TrimSpace(c.Param("uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的 profile uuid", err)
		return
	}
	repo := repo.NewRAGProfileVersionRepository(h.db)
	created, err := repo.CreateRollbackDraft(c.Request.Context(), profileUUID, req.CreatedBy)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "profile 不存在", err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "回滚草稿创建失败", err)
		return
	}
	if created == nil {
		dto.ResponseError(c, http.StatusNotFound, "profile 不存在", nil)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toProfileVersionView(created.UUID, created.ProfileKey, created.Version, created.Status, created.DisplayName, created.Config, created.RollbackFromID, created.PublishedAt, created.PublishedBy, created.CreatedBy, created.CreatedAt, created.UpdatedAt))
}
