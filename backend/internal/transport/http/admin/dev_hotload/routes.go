package dev_hotload

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	devhotload "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload"
	"github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/store"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegisterAPIRoutes wires /internal/dev/plugins endpoints.
func RegisterAPIRoutes(public, protected *gin.RouterGroup, deps *shared.Deps) {
	_ = public
	if protected == nil || deps == nil || deps.DevHotloadService == nil {
		return
	}
	handler := &apiHandler{
		svc:       deps.DevHotloadService,
		sseBuffer: deps.DevHotloadOptions.Observability.SSEBufferSize,
	}
	if handler.sseBuffer <= 0 {
		handler.sseBuffer = 16
	}
	group := protected.Group("/internal/dev/plugins")
	group.Use(middleware.AdminOnlyMiddleware())
	group.POST("/register", handler.register)
	group.POST("/reload", handler.reload)
	group.GET("/stream", handler.stream)
	group.GET("/sessions", handler.listSessions)
	group.GET("/sessions/:sessionId", handler.getSession)
	group.DELETE("/sessions/:sessionId", handler.terminate)
	group.GET("/:sessionId", handler.getSession)
	group.DELETE("/register/:sessionId", handler.terminate)
}

type apiHandler struct {
	svc       *devhotload.Service
	sseBuffer int
}

func (h *apiHandler) register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	result, err := h.svc.Register(c.Request.Context(), devhotload.RegisterInput{
		PluginID:        req.PluginID,
		TenantID:        req.TenantID,
		DeveloperID:     req.DeveloperID,
		BuildHash:       req.BuildHash,
		EntryPoints:     req.EntryPoints,
		Manifest:        req.Manifest,
		Metadata:        req.Metadata,
		SandboxEndpoint: req.SandboxEndpoint,
		LogURL:          req.LogURL,
		WatchFileLimit:  req.WatchFileLimit,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, result)
}

func (h *apiHandler) reload(c *gin.Context) {
	var req reloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}
	err = h.svc.Reload(c.Request.Context(), devhotload.ReloadInput{
		SessionID:   sessionID,
		ReloadToken: req.ReloadToken,
		Sequence:    req.Sequence,
		Duration:    time.Duration(req.DurationMs) * time.Millisecond,
		Changed:     req.ChangedFiles,
		Artifacts:   req.Artifacts,
		Success:     req.Success,
		Error:       req.Error,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"sessionId": req.SessionID, "sequence": req.Sequence})
}

func (h *apiHandler) terminate(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("sessionId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}
	note := c.Query("note")
	if err := h.svc.Terminate(c.Request.Context(), sessionID, note); err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"sessionId": sessionID.String(), "note": note})
}

func (h *apiHandler) getSession(c *gin.Context) {
	sessionID, err := uuid.Parse(c.Param("sessionId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid sessionId", err)
		return
	}
	session, err := h.svc.GetSession(c.Request.Context(), sessionID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if session == nil {
		dto.ResponseError(c, http.StatusNotFound, "session not found", nil)
		return
	}
	dto.ResponseSuccess(c, sessionViewFromModel(session))
}

func (h *apiHandler) stream(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Flush()

	_, ch, cancel := h.svc.SubscribeEvents(h.sseBuffer)
	defer cancel()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case evt, ok := <-ch:
			if !ok {
				return false
			}
			payload, _ := json.Marshal(evt)
			fmt.Fprintf(c.Writer, "data: %s\n\n", payload)
			c.Writer.Flush()
			return true
		}
	})
}

func (h *apiHandler) listSessions(c *gin.Context) {
	pluginID := c.Query("pluginId")
	var tenantID *uint64
	if tidStr := strings.TrimSpace(c.Query("tenant")); tidStr != "" {
		tid, parseErr := strconv.ParseUint(tidStr, 10, 64)
		if parseErr != nil || tid == 0 {
			if parseErr == nil {
				parseErr = fmt.Errorf("tenant must be positive")
			}
			dto.ResponseError(c, http.StatusBadRequest, "invalid tenant", parseErr)
			return
		}
		tenantID = &tid
	}
	statuses := normalizeSessionStatuses(c.QueryArray("status"))
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid limit", err)
		return
	}
	offset, err := parseOffset(c.Query("offset"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid offset", err)
		return
	}

	sessions, err := h.svc.ListSessions(c.Request.Context(), pluginID, tenantID, statuses, limit, offset)
	if err != nil {
		h.writeError(c, err)
		return
	}
	views := make([]sessionView, 0, len(sessions))
	for i := range sessions {
		views = append(views, sessionViewFromModel(&sessions[i]))
	}
	dto.ResponseSuccess(c, gin.H{
		"items":  views,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *apiHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, devhotload.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusForbidden, err.Error(), err)
	case errors.Is(err, devhotload.ErrSessionConflict):
		var conflict *devhotload.SessionConflictError
		if errors.As(err, &conflict) && conflict != nil && conflict.Session != nil {
			dto.ResponseErrorWithDetails(c, http.StatusConflict, err.Error(), err, map[string]any{
				"sessionId": conflict.Session.UUID.String(),
				"pluginId":  conflict.Session.PluginID,
				"tenantId":  conflict.Session.TenantID,
				"status":    conflict.Session.Status,
			})
			return
		}
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	case errors.Is(err, devhotload.ErrSessionNotFound), errors.Is(err, store.ErrNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, devhotload.ErrReloadToken):
		dto.ResponseError(c, http.StatusUnauthorized, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}

type registerRequest struct {
	PluginID        string         `json:"pluginId" binding:"required"`
	TenantID        uint64         `json:"tenantId" binding:"required"`
	DeveloperID     uint64         `json:"developerId" binding:"required"`
	BuildHash       string         `json:"buildHash"`
	EntryPoints     []string       `json:"entryPoints"`
	Manifest        map[string]any `json:"manifest"`
	Metadata        map[string]any `json:"metadata"`
	SandboxEndpoint string         `json:"sandboxEndpoint"`
	LogURL          string         `json:"logUrl"`
	WatchFileLimit  int            `json:"watchFileLimit"`
}

type reloadRequest struct {
	SessionID    string           `json:"sessionId" binding:"required"`
	ReloadToken  string           `json:"reloadToken" binding:"required"`
	Sequence     int64            `json:"sequence"`
	DurationMs   int64            `json:"durationMs"`
	ChangedFiles []string         `json:"changedFiles"`
	Artifacts    []map[string]any `json:"artifacts"`
	Success      bool             `json:"success"`
	Error        string           `json:"error"`
}

type sessionView struct {
	SessionID       string         `json:"sessionId"`
	PluginID        string         `json:"pluginId"`
	TenantID        uint64         `json:"tenantId"`
	DeveloperID     uint64         `json:"developerId"`
	Status          string         `json:"status"`
	ReloadToken     string         `json:"reloadToken"`
	ExpiresAt       time.Time      `json:"expiresAt"`
	SandboxEndpoint string         `json:"sandboxEndpoint"`
	LogURL          string         `json:"logUrl"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

func sessionViewFromModel(m *model.DevHotloadSession) sessionView {
	view := sessionView{
		SessionID:       m.UUID.String(),
		PluginID:        m.PluginID,
		TenantID:        m.TenantID,
		DeveloperID:     m.DeveloperID,
		Status:          m.Status,
		ReloadToken:     m.ReloadToken,
		ExpiresAt:       m.ExpiresAt,
		SandboxEndpoint: m.SandboxEndpoint,
		LogURL:          m.LogURL,
	}
	if len(m.Metadata) > 0 {
		var meta map[string]any
		if err := json.Unmarshal(m.Metadata, &meta); err == nil {
			view.Metadata = meta
		}
	}
	return view
}

func normalizeSessionStatuses(values []string) []string {
	valid := map[string]struct{}{
		model.DevHotloadSessionStatusPending:    {},
		model.DevHotloadSessionStatusActive:     {},
		model.DevHotloadSessionStatusTerminated: {},
		model.DevHotloadSessionStatusExpired:    {},
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		for _, token := range strings.Split(value, ",") {
			status := strings.ToLower(strings.TrimSpace(token))
			if status == "" {
				continue
			}
			if _, ok := valid[status]; !ok {
				continue
			}
			if _, exists := seen[status]; exists {
				continue
			}
			seen[status] = struct{}{}
			result = append(result, status)
		}
	}
	return result
}

func parseLimit(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 50, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, fmt.Errorf("limit must be positive")
	}
	if limit > 200 {
		limit = 200
	}
	return limit, nil
}

func parseOffset(value string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, fmt.Errorf("offset must be >= 0")
	}
	return offset, nil
}
