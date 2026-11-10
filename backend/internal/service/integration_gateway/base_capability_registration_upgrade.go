package integration_gateway

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

// 此清单限定本次破坏性升级；不能把任意插件、自定义 HTTP 代理改成 Core binding。
func mediaRegistrationEndpoint(id string) string {
	switch id {
	case "com.corex.ai.connection.test":
		return "core://ai/connection"
	case "com.corex.ai.catalog.providers.read", "com.corex.ai.catalog.models.read":
		return "core://ai/model-catalog"
	case "com.corex.ai.vlm.invoke", "com.corex.ai.image.invoke", "com.corex.ai.video.invoke", "com.corex.ai.tts.invoke":
		return "core://ai/media"
	case "com.corex.ai.media.tasks.read", "com.corex.ai.media.tasks.cancel":
		return "core://ai/media-tasks"
	}
	return ""
}

func legacyMediaAdapter(id string, ep repo.AdapterEndpoint) bool {
	path, method := "", "POST"
	switch id {
	case "com.corex.ai.connection.test":
		path = "/api/v1/admin/agents/test/connection"
	case "com.corex.ai.catalog.providers.read":
		path, method = "/api/v1/admin/agents/providers", "GET"
	case "com.corex.ai.catalog.models.read":
		path, method = "/api/v1/admin/agents/models", "GET"
	case "com.corex.ai.vlm.invoke":
		path = "/api/v1/ai/vlm/invoke"
	case "com.corex.ai.image.invoke":
		path = "/api/v1/ai/image/invoke"
	case "com.corex.ai.video.invoke":
		path = "/api/v1/ai/video/invoke"
	case "com.corex.ai.tts.invoke":
		path = "/api/v1/ai/tts/invoke"
	case "com.corex.ai.media.tasks.read":
		path, method = "/api/v1/ai/media/tasks/{task_uuid}", "GET"
	case "com.corex.ai.media.tasks.cancel":
		path = "/api/v1/ai/media/tasks/{task_uuid}/cancel"
	default:
		return false
	}
	transport := strings.ToLower(ep.TransportType)
	if transport == "http" || transport == "rest" {
		return ep.Endpoint == path && (ep.ServiceRef == "" || ep.ServiceRef == path) && ep.Labels["method"] == method
	}
	return transport == "grpc" && strings.HasSuffix(id, ".invoke") && ep.Endpoint == "powerx.ai.v1.MultimodalService" && (ep.ServiceRef == "" || ep.ServiceRef == ep.Endpoint) && ep.Labels["rpc"] == "Invoke"
}

func (s *BaseCapabilitySeeder) upgradeMediaRegistration(ctx context.Context, current repo.Registration, def platformCapabilityDefinition, at time.Time) error {
	endpoint := mediaRegistrationEndpoint(def.CapabilityID)
	if endpoint == "" {
		return nil
	}
	conflict := func() error {
		return fmt.Errorf("UPGRADE_CAPABILITY_BINDING_CONFLICT: %s/%s", current.TenantUUID, current.CapabilityID)
	}
	// 来源已由 PreserveInstalledState 的 record owner 检查保护；仍须识别具体旧绑定。
	knownContract := current.ContractRef == "backend/internal/transport/http/admin/agent/api.go" || current.ContractRef == "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml" || current.ContractRef == "docs/contracts/ai-media-v2.md" || current.ContractRef == "docs/contracts/ai-media-v2.openapi.yaml"
	if def.CapabilityID == "com.corex.ai.connection.test" {
		knownContract = current.ContractRef == "docs/contracts/ai-settings-connection-test.md" || current.ContractRef == "docs/contracts/ai-connection-test.v1.openapi.yaml"
	}
	if !knownContract {
		return conflict()
	}
	expected := s.buildPlatformRegistration(def, current.TenantUUID, at)
	if len(expected.Adapters) != 1 || expected.Adapters[0].TransportType != "core_internal" || expected.Adapters[0].Endpoint != endpoint || expected.Adapters[0].Labels["method"] != "INVOKE" {
		return fmt.Errorf("UPGRADE_CAPABILITY_CONTRACT_INVALID: %s", def.CapabilityID)
	}
	if len(current.Adapters) == 0 {
		return conflict()
	}
	var primary *repo.AdapterEndpoint
	for n := range current.Adapters {
		ep := &current.Adapters[n]
		if ep.TransportType == "core_internal" && ep.Endpoint == endpoint && (ep.ServiceRef == "" || ep.ServiceRef == endpoint) && ep.Labels["method"] == "INVOKE" {
			primary = ep
		} else if !legacyMediaAdapter(def.CapabilityID, *ep) {
			return conflict()
		}
	}
	if primary == nil {
		primary = &current.Adapters[0]
	}
	next := current
	next.Version = current.Version + 1
	next.ContractRef = expected.ContractRef
	// 保留运维参数、禁用位、可见范围；协议及 Schema 来自正式合同。
	ep := *primary
	ep.AdapterID = expected.Adapters[0].AdapterID
	ep.TransportType, ep.Endpoint, ep.ServiceRef = "core_internal", endpoint, endpoint
	ep.Labels = map[string]string{}
	for k, v := range primary.Labels {
		ep.Labels[k] = v
	}
	for _, key := range []string{"rpc", "sts_direct", "method", "schema_ref", "auth_type", "actor_context", "resource_scope", "tool_scope"} {
		delete(ep.Labels, key)
	}
	for k, v := range expected.Adapters[0].Labels {
		ep.Labels[k] = v
	}
	next.Adapters = []repo.AdapterEndpoint{ep}
	next.RoutingPolicy.FallbackSequence = []string{ep.AdapterID}
	// 比较前先保留旧 ID；写新快照时另建 policy，不能复用 UUID 覆盖历史。
	if current.ContractRef == next.ContractRef && reflect.DeepEqual(current.Adapters, next.Adapters) && reflect.DeepEqual(current.RoutingPolicy, next.RoutingPolicy) {
		return nil
	}
	next.RoutingPolicy.ID = uuid.Nil
	if next.FallbackPlan != nil {
		p := *next.FallbackPlan
		p.ID = uuid.Nil
		next.FallbackPlan = &p
	}
	next.UpdatedBy = platformActor
	saved, err := s.registry.Update(ctx, nil, next, current.Version)
	if err != nil {
		return fmt.Errorf("upgrade registration %s/%s: %w", current.TenantUUID, current.CapabilityID, err)
	}
	operation, version := "UPGRADE_MEDIA_REGISTRATION", 2
	if def.CapabilityID == "com.corex.ai.connection.test" {
		operation, version = "UPGRADE_AI_CONNECTION_REGISTRATION", 1
	}
	return s.repo.DB.WithContext(ctx).Create(&audit.AuditEvent{
		OccurredAt: at, TenantUUID: current.TenantUUID, CorrelationID: reqctx.GetTraceID(ctx),
		Source: "capability.catalog_upgrade", Operation: operation, ResourceType: "capability", ResourceID: current.CapabilityID,
		Outcome: "SUCCESS", Severity: "INFO", ActorUserName: platformActor,
		ChangesBefore: encodeJSONValue(map[string]any{"version": current.Version, "contract_ref": current.ContractRef, "status": current.Status}, "{}"),
		ChangesAfter:  encodeJSONValue(map[string]any{"version": saved.Version, "contract_ref": saved.ContractRef, "status": saved.Status, "transport_type": ep.TransportType, "endpoint": ep.Endpoint, "method": ep.Labels["method"]}, "{}"),
		Meta:          encodeJSONValue(map[string]any{"contract_version": version, "authorization_preserved": true, "version_lock": "requires_existing_admin_confirmation"}, "{}"),
	}).Error
}
