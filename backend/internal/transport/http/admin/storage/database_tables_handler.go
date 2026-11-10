package storage

import (
	"errors"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) databaseTables(c *gin.Context) {
	if h.service == nil {
		fail(c, 503, "STORAGE_OBSERVER_UNAVAILABLE", "数据库观测未配置")
		return
	}
	out, err := h.service.DatabaseTables(c.Request.Context(), c.Param("table_id"))
	if errors.Is(err, storagegov.ErrSystemTableNotFound) {
		fail(c, 404, "STORAGE_TABLE_NOT_REGISTERED", "该表不在系统观测目录中")
		return
	}
	if err != nil {
		fail(c, 503, "STORAGE_DATABASE_TABLES_UNAVAILABLE", "读取系统表观测失败，请重试")
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) tableMonitoring(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.DatabaseTableMonitoring(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) saveTableMonitoring(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64                   `json:"expected_revision"`
		RequestID        string                   `json:"request_id"`
		Policy           *monitoringSettingsInput `json:"policy"`
		Confirm          bool                     `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if in.ExpectedRevision == nil || in.Policy == nil || !in.Policy.complete() || in.RequestID == "" || !in.Confirm {
		h.stateError(c, storagegov.ErrPolicyInvalid)
		return
	}
	out, err := h.service.SaveTableMonitoringConfiguration(c.Request.Context(), *in.ExpectedRevision, in.RequestID, in.Policy.settings())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

type monitoringSettingsInput struct {
	Enabled             *bool  `json:"enabled"`
	WarningBytes        *int64 `json:"warning_bytes"`
	GrowthBytes         *int64 `json:"growth_bytes"`
	GrowthWindowMinutes *int   `json:"growth_window_minutes"`
	StaleAfterMinutes   *int   `json:"stale_after_minutes"`
	FailureLimit        *int   `json:"failure_limit"`
}

func (p *monitoringSettingsInput) complete() bool {
	return p.Enabled != nil && p.WarningBytes != nil && p.GrowthBytes != nil && p.GrowthWindowMinutes != nil && p.StaleAfterMinutes != nil && p.FailureLimit != nil
}
func (p *monitoringSettingsInput) settings() storagegov.TableMonitoringSettings {
	return storagegov.TableMonitoringSettings{Enabled: *p.Enabled, WarningBytes: *p.WarningBytes, GrowthBytes: *p.GrowthBytes, GrowthWindowMinutes: *p.GrowthWindowMinutes, StaleAfterMinutes: *p.StaleAfterMinutes, FailureLimit: *p.FailureLimit}
}
func (h *handler) adoptTableMonitoring(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		RequestID      string `json:"request_id"`
		LegacyRevision *int64 `json:"legacy_revision"`
		LegacyDigest   string `json:"legacy_digest"`
		Confirm        bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm || in.LegacyRevision == nil || *in.LegacyRevision < 0 || len(in.LegacyDigest) != 64 {
		h.stateError(c, storagegov.ErrPolicyInvalid)
		return
	}
	out, e := h.service.AdoptTableMonitoring(c.Request.Context(), *in.LegacyRevision, in.LegacyDigest, in.RequestID)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) tableMonitoringRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, e := h.service.TableMonitoringRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) tableHistory(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.DatabaseTableHistory(c.Request.Context(), c.Param("table_id"))
	if errors.Is(err, storagegov.ErrSystemTableNotFound) {
		fail(c, 404, "STORAGE_TABLE_NOT_REGISTERED", "该表不在系统观测目录中")
		return
	}
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
