package capability

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AdapterHandler 管理传输配置的 HTTP handler。
type AdapterHandler struct {
	svc *svc.AdapterService
}

// NewAdapterHandler 构建传输配置处理器。
func NewAdapterHandler(deps *shared.Deps) *AdapterHandler {
	return &AdapterHandler{
		svc: svc.NewAdapterService(deps.DB, nil, nil),
	}
}

type transportProfilesRequest struct {
	Transports []transportProfilePayload `json:"transports" binding:"required"`
}

type transportProfileView struct {
	Transport        string                 `json:"transport"`
	Mode             string                 `json:"mode"`
	TimeoutMillis    int                    `json:"timeout_ms"`
	Streaming        bool                   `json:"streaming"`
	Retry            map[string]interface{} `json:"retry,omitempty"`
	QoS              map[string]interface{} `json:"qos,omitempty"`
	EndpointSelector map[string]interface{} `json:"endpoint_selector,omitempty"`
	LastHealthStatus map[string]interface{} `json:"last_health_status,omitempty"`
}

// ListTransportProfiles 查询传输配置列表。
func (h *AdapterHandler) ListTransportProfiles(c *gin.Context) {
	tenantID := tenantIDFromRequest(c, nil)
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")

	profiles, err := h.svc.ListProfiles(c.Request.Context(), tenantID, capabilityKey, version)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "传输配置不存在", err)
			return
		}
		writeInternalError(c, err)
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"transports": toProfileViews(profiles, nil),
	})
}

// UpsertTransportProfiles 覆盖传输配置。
func (h *AdapterHandler) UpsertTransportProfiles(c *gin.Context) {
	var reqBody transportProfilesRequest
	if err := c.ShouldBindJSON(&reqBody); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}

	tenantID := tenantIDFromRequest(c, nil)
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")

	profiles := toTransportProfiles(reqBody.Transports)
	if err := h.svc.ReplaceProfiles(c.Request.Context(), tenantID, capabilityKey, version, profiles); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseError(c, http.StatusNotFound, "能力契约不存在", err)
			return
		}
		writeInternalError(c, err)
		return
	}

	updated, err := h.svc.ListProfiles(c.Request.Context(), tenantID, capabilityKey, version)
	if err != nil {
		writeInternalError(c, err)
		return
	}

	dto.ResponseSuccess(c, gin.H{
		"transports": toProfileViews(updated, nil),
	})
}

// RunHealthCheck 执行健康检查并返回结果。
func (h *AdapterHandler) RunHealthCheck(c *gin.Context) {
	tenantID := tenantIDFromRequest(c, nil)
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")
	transportRaw := c.Param("transport")

	kind, err := parseTransportKind(transportRaw)
	if err != nil {
		writeBadRequest(c, "invalid_transport", err.Error())
		return
	}

	report, err := h.svc.HealthCheck(c.Request.Context(), tenantID, capabilityKey, version, kind)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			dto.ResponseError(c, http.StatusNotFound, "能力契约不存在", err)
		case errors.Is(err, svc.ErrAdapterNotFound):
			dto.ResponseError(c, http.StatusNotFound, "未注册对应协议适配器", err)
		default:
			writeInternalError(c, err)
		}
		return
	}

	resp := gin.H{
		"status":     report.Status,
		"checked_at": report.CheckedAt.UTC().Format(time.RFC3339),
	}
	if meta := toInterfaceMap(report.Metadata); meta != nil {
		resp["metadata"] = meta
	}
	if report.LastError != nil {
		resp["error"] = map[string]interface{}{
			"namespace":        report.LastError.Namespace,
			"category":         report.LastError.Category,
			"code":             report.LastError.Code,
			"severity":         report.LastError.Severity,
			"stage":            report.LastError.Stage,
			"message":          report.LastError.Message,
			"suggested_action": report.LastError.SuggestedAction,
		}
	}

	dto.ResponseSuccess(c, gin.H{"health": resp})
}

func toProfileViews(items []svc.TransportProfile, health map[string]*svc.TransportHealthReport) []transportProfileView {
	result := make([]transportProfileView, 0, len(items))
	for _, item := range items {
		view := transportProfileView{
			Transport:        item.Transport,
			Mode:             item.Mode,
			TimeoutMillis:    item.TimeoutMillis,
			Streaming:        item.Streaming,
			Retry:            item.Retry,
			QoS:              item.QoS,
			EndpointSelector: item.EndpointSelector,
		}
		if report := item.HealthReport; report != nil {
			view.LastHealthStatus = map[string]interface{}{
				"status":     report.Status,
				"checked_at": report.CheckedAt.UTC().Format(time.RFC3339),
			}
			if report.LastError != nil {
				view.LastHealthStatus["error"] = map[string]interface{}{
					"namespace": report.LastError.Namespace,
					"category":  report.LastError.Category,
					"code":      report.LastError.Code,
					"severity":  report.LastError.Severity,
					"stage":     report.LastError.Stage,
					"message":   report.LastError.Message,
				}
			}
		}
		if health != nil {
			if report, ok := health[strings.ToLower(item.Transport)]; ok && report != nil {
				view.LastHealthStatus = map[string]interface{}{
					"status":     report.Status,
					"checked_at": report.CheckedAt.UTC().Format(time.RFC3339),
				}
				if report.LastError != nil {
					view.LastHealthStatus["error"] = map[string]interface{}{
						"namespace": report.LastError.Namespace,
						"category":  report.LastError.Category,
						"code":      report.LastError.Code,
						"severity":  report.LastError.Severity,
						"stage":     report.LastError.Stage,
						"message":   report.LastError.Message,
					}
				}
			}
		}
		result = append(result, view)
	}
	return result
}

func parseTransportKind(v string) (capb.TransportKind, error) {
	switch strings.ToLower(v) {
	case "http":
		return capb.TransportKind_TRANSPORT_KIND_HTTP, nil
	case "grpc":
		return capb.TransportKind_TRANSPORT_KIND_GRPC, nil
	case "mcp":
		return capb.TransportKind_TRANSPORT_KIND_MCP, nil
	case "agent":
		return capb.TransportKind_TRANSPORT_KIND_AGENT, nil
	default:
		return capb.TransportKind_TRANSPORT_KIND_UNSPECIFIED, fmt.Errorf("unknown transport %s", v)
	}
}

func toInterfaceMap(in map[string]string) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
