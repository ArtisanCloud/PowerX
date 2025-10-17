package capability_registry

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	discovery "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// DiscoveryHandler 负责处理 Discovery 相关 HTTP 请求。
type DiscoveryHandler struct {
	service *discovery.Service
}

// NewDiscoveryHandler 创建处理器。
func NewDiscoveryHandler(service *discovery.Service) *DiscoveryHandler {
	if service == nil {
		panic("discovery handler requires service")
	}
	return &DiscoveryHandler{service: service}
}

// GetSnapshot 返回缓存快照。
func (h *DiscoveryHandler) GetSnapshot(ctx *gin.Context) {
	var req getSnapshotRequest
	if err := dto.ValidateRequestWithContext(ctx, &req); err != nil {
		dto.ResponseValidationError(ctx, err)
		return
	}

	snapshot, err := h.service.GetSnapshot(ctx.Request.Context(), req.TenantID, req.CapabilityID, req.ClientID)
	if err != nil {
		dto.RespondErrorFrom(ctx, err)
		return
	}

	ttl := int(snapshot.RemainingTTL(h.service.Now()).Seconds())
	if ttl < 0 {
		ttl = 0
	}
	ctx.Header("Cache-Control", "max-age="+strconv.Itoa(ttl))
	dto.ResponseSuccess(ctx, toHTTPResponse(snapshot))
}

// Sync 批量同步快照。
func (h *DiscoveryHandler) Sync(ctx *gin.Context) {
	var req syncRequest
	if err := dto.ValidateRequestWithContext(ctx, &req); err != nil {
		dto.ResponseValidationError(ctx, err)
		return
	}

	snapshots, err := h.service.Sync(ctx.Request.Context(), discovery.SyncRequest{
		TenantID:     req.TenantID,
		Capabilities: req.Capabilities,
		ClientID:     req.ClientID,
		Force:        req.Force,
	})
	if err != nil {
		dto.RespondErrorFrom(ctx, err)
		return
	}

	resp := make([]httpDiscoverySnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		resp = append(resp, toHTTPResponse(snapshot))
	}
	dto.ResponseSuccess(ctx, resp)
}

type syncRequest struct {
	TenantID     string   `json:"tenant_id" binding:"required"`
	Capabilities []string `json:"capabilities"`
	ClientID     string   `json:"client_id"`
	Force        bool     `json:"force"`
}

type getSnapshotRequest struct {
	TenantID     string `uri:"tenantId" binding:"required"`
	CapabilityID string `uri:"capabilityId" binding:"required"`
	ClientID     string `form:"client_id"`
}

type httpDiscoverySnapshot struct {
	CapabilityID   string      `json:"capability_id"`
	TenantID       string      `json:"tenant_id"`
	Version        uint64      `json:"version"`
	IssuedAt       time.Time   `json:"issued_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
	RoutingPolicy  interface{} `json:"routing_policy"`
	Adapters       interface{} `json:"adapters"`
	FallbackPlan   interface{} `json:"fallback_plan,omitempty"`
	MetadataDigest string      `json:"metadata_digest"`
	PolicyDigest   string      `json:"policy_digest,omitempty"`
	Source         string      `json:"source"`
	ClientID       string      `json:"client_id"`
	Stale          bool        `json:"stale"`
}

func toHTTPResponse(snapshot discovery.Snapshot) httpDiscoverySnapshot {
	return httpDiscoverySnapshot{
		CapabilityID:   snapshot.CapabilityID,
		TenantID:       snapshot.TenantID,
		Version:        snapshot.Version,
		IssuedAt:       snapshot.IssuedAt,
		ExpiresAt:      snapshot.ExpiresAt,
		RoutingPolicy:  snapshot.RoutingPolicy,
		Adapters:       snapshot.Adapters,
		FallbackPlan:   snapshot.FallbackPlan,
		MetadataDigest: snapshot.MetadataDigest,
		PolicyDigest:   snapshot.PolicyDigest,
		Source:         string(snapshot.Source),
		ClientID:       snapshot.ClientID,
		Stale:          snapshot.Stale,
	}
}
