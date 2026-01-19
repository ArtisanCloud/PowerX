package knowledge_space

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type VectorIndexHandler struct {
	svc *ksvc.VectorIndexService
}

func NewVectorIndexHandler(deps *shared.Deps) *VectorIndexHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return nil
	}

	// DSN selection rule: pgvector.dsn -> database.dsn -> compose.
	pgCfg := cfg.KnowledgeSpace.VectorStore.PgVector
	dsn := strings.TrimSpace(pgCfg.DSN)
	if dsn == "" {
		dsn = strings.TrimSpace(cfg.Database.DSN)
	}
	if dsn == "" && strings.TrimSpace(cfg.Database.Host) != "" {
		sslmode := strings.TrimSpace(cfg.Database.SSLMode)
		if sslmode == "" {
			sslmode = "disable"
		}
		tz := strings.TrimSpace(cfg.Database.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		dsn = "host=" + strings.TrimSpace(cfg.Database.Host) +
			" port=" + strconv.Itoa(cfg.Database.Port) +
			" user=" + strings.TrimSpace(cfg.Database.UserName) +
			" password=" + strings.TrimSpace(cfg.Database.Password) +
			" dbname=" + strings.TrimSpace(cfg.Database.Database) +
			" sslmode=" + sslmode +
			" TimeZone=" + tz
	}

	return &VectorIndexHandler{
		svc: ksvc.NewVectorIndexService(ksvc.VectorIndexServiceOptions{
			DB: deps.DB,
			PGVector: pgvector.Config{
				DSN:            dsn,
				Schema:         strings.TrimSpace(pgCfg.Schema),
				Table:          strings.TrimSpace(pgCfg.Table),
				Dimensions:     pgCfg.Dimensions,
				EnableMigrations: false,
				BatchSize:      pgCfg.BatchSize,
				Lists:          pgCfg.Lists,
				TimeoutSeconds: pgCfg.TimeoutSeconds,
			},
		}),
	}
}

type activateDenseIndexRequest struct {
	EmbeddingProfileKey string `json:"embeddingProfileKey" binding:"required"`
	RequestedBy         string `json:"requestedBy"`
}

func (h *VectorIndexHandler) GetStatus(c *gin.Context) {
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", nil)
		return
	}
	out, err := h.svc.GetStatus(c.Request.Context(), tenantUUID.String(), spaceID, 50)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *VectorIndexHandler) Activate(c *gin.Context) {
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(c.Param("spaceId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "无效的空间 ID", err)
		return
	}
	var req activateDenseIndexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "服务异常", nil)
		return
	}
	res, err := h.svc.ActivateDenseIndex(c.Request.Context(), ksvc.ActivateDenseIndexInput{
		TenantUUID:          tenantUUID.String(),
		SpaceUUID:           spaceID,
		EmbeddingProfileKey: strings.TrimSpace(req.EmbeddingProfileKey),
		RequestedBy:         strings.TrimSpace(req.RequestedBy),
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, res)
}
