package migration

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dtoops "github.com/ArtisanCloud/PowerX/internal/dto/ops"
	migrationops "github.com/ArtisanCloud/PowerX/internal/service/migration_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type handler struct {
	svc *migrationops.Service
}

func NewHandler(deps *shared.Deps) *handler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &handler{svc: migrationops.NewService(deps.DB)}
}

func (h *handler) TriggerMigration(c *gin.Context) {
	var req dtoops.MigrationRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	row, err := h.svc.TriggerMigration(c.Request.Context(), migrationops.TriggerRequest{
		SourceEnv: req.SourceEnv,
		TargetEnv: req.TargetEnv,
		DryRun:    req.DryRun,
		Operator:  resolveOperator(c),
		TraceID:   strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"record": row})
}

func (h *handler) GetMigration(c *gin.Context) {
	id := parseUint(c.Param("migrationId"))
	row, err := h.svc.GetMigration(c.Request.Context(), id)
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"record": row})
}

func (h *handler) AcceptMigration(c *gin.Context) {
	id := parseUint(c.Param("migrationId"))
	var req dtoops.MigrationAcceptanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	row, err := h.svc.AcceptMigration(c.Request.Context(), migrationops.AcceptanceRequest{
		MigrationID:              id,
		DBMigrationCompleted:     req.DBMigrationCompleted,
		InstanceMigrationPassed:  req.InstanceMigrationPassed,
		AcceptanceConclusionNote: req.Conclusion,
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"record": row})
}

func (h *handler) TriggerTrafficSwitch(c *gin.Context) {
	var req dtoops.MigrationSwitchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	operationID, row, err := h.svc.TriggerTrafficSwitch(c.Request.Context(), migrationops.SwitchRequest{
		MigrationID: parseUint(req.MigrationID),
		Rollback:    req.Rollback,
		Operator:    resolveOperator(c),
		TraceID:     strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"operation_id": operationID, "record": row})
}

func (h *handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, migrationops.ErrInvalidMigrationRequest):
		dto.ResponseError(c, http.StatusBadRequest, "invalid migration request", err)
	case errors.Is(err, migrationops.ErrMigrationNotFound):
		dto.ResponseError(c, http.StatusNotFound, "migration record not found", err)
	case errors.Is(err, migrationops.ErrMigrationNotReady):
		dto.ResponseError(c, http.StatusConflict, "migration not ready", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "migration operation failed", err)
	}
}

func resolveOperator(c *gin.Context) string {
	ctx := c.Request.Context()
	if reqctx.IsRoot(ctx) {
		return "root"
	}
	if reqctx.GetMemberID(ctx) > 0 {
		return "member"
	}
	return "system"
}

func parseUint(raw string) uint64 {
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
