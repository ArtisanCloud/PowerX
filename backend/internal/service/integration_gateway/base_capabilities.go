package integration_gateway

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
)

var (
	platformCapabilityCache     []platformCapabilityDefinition
	platformCapabilityCacheOnce sync.Once
)

const (
	// PlatformPluginID 标识 CoreX 内置能力的 “虚拟” 插件 ID。
	PlatformPluginID = "corex.platform"
	// PlatformPluginVersion 标记平台能力记录的版本。
	PlatformPluginVersion = "2025.01"

	platformActor = "integration_gateway.bootstrap"
)

// BaseCapabilitySeederOptions 配置平台能力 Seeder。
type BaseCapabilitySeederOptions struct {
	RecordRepo    *repo.CapabilityRecordRepository
	RegistryRepo  *repo.CapabilityRegistryRepository
	TenantRepo    *tenantRepo.TenantRepository
	Logger        *pxlog.Logger
	Clock         func() time.Time
	PluginID      string
	PluginVersion string
}

// BaseCapabilitySeeder 负责在 Capability Registry 中写入 CoreX 内置能力。
type BaseCapabilitySeeder struct {
	repo          *repo.CapabilityRecordRepository
	registry      *repo.CapabilityRegistryRepository
	tenantRepo    *tenantRepo.TenantRepository
	logger        *pxlog.Logger
	clock         func() time.Time
	pluginID      string
	pluginVersion string
}

// NewBaseCapabilitySeeder 构造 Seeder。
func NewBaseCapabilitySeeder(opts BaseCapabilitySeederOptions) *BaseCapabilitySeeder {
	if opts.RecordRepo == nil {
		panic("base capability seeder requires capability record repository")
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	pluginID := opts.PluginID
	if pluginID == "" {
		pluginID = PlatformPluginID
	}
	pluginVersion := opts.PluginVersion
	if pluginVersion == "" {
		pluginVersion = PlatformPluginVersion
	}
	return &BaseCapabilitySeeder{
		repo:          opts.RecordRepo,
		registry:      opts.RegistryRepo,
		tenantRepo:    opts.TenantRepo,
		logger:        opts.Logger,
		clock:         clock,
		pluginID:      pluginID,
		pluginVersion: pluginVersion,
	}
}

// Ensure 将平台能力写入 CapabilityRecord（幂等）。
func (s *BaseCapabilitySeeder) Ensure(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("base capability seeder is not configured")
	}
	now := s.clock()
	defs := platformCapabilityDefinitions()
	for _, def := range defs {
		record := def.toRecord(s.pluginID, s.pluginVersion, now)
		if _, err := s.repo.Upsert(ctx, record); err != nil {
			return fmt.Errorf("seed capability %s failed: %w", def.CapabilityID, err)
		}
	}
	if s.logger != nil {
		s.logger.InfoF(ctx, "[integration_gateway] platform capabilities seeded count=%d", len(defs))
	}
	if s.registry != nil && s.tenantRepo != nil {
		if err := s.ensurePlatformRegistrations(ctx, defs); err != nil {
			return err
		}
	}
	return nil
}

type platformCapabilityDefinition struct {
	CapabilityID string
	Title        string
	Description  string
	Module       string
	Categories   []string
	Intents      []string
	ToolScopes   []string
	Policy       capabilityPolicy
	Protocols    []models.ProtocolBinding
	Docs         []string
}

type capabilityPolicy struct {
	Prefer   string   `json:"prefer,omitempty"`
	Fallback []string `json:"fallback,omitempty"`
}

type capabilityAnnotations struct {
	Source string   `json:"source"`
	Module string   `json:"module,omitempty"`
	Docs   []string `json:"docs,omitempty"`
}

func (d platformCapabilityDefinition) toRecord(pluginID, pluginVersion string, now time.Time) *models.CapabilityRecord {
	publishedAt := now
	record := &models.CapabilityRecord{
		CapabilityID:         d.CapabilityID,
		PluginID:             pluginID,
		PluginVersion:        pluginVersion,
		Title:                d.Title,
		Description:          d.Description,
		Categories:           encodeJSONArray(d.Categories),
		Intents:              encodeJSONArray(d.Intents),
		ToolScope:            encodeJSONArray(d.ToolScopes),
		Protocols:            encodeJSONValue(d.Protocols, "[]"),
		Policy:               encodeJSONValue(d.Policy, "{}"),
		WorkflowTemplateRefs: encodeJSONValue([]interface{}{}, "[]"),
		CompositeGraphs:      encodeJSONValue([]interface{}{}, "[]"),
		Annotations: encodeJSONValue(capabilityAnnotations{
			Source: "corex",
			Module: d.Module,
			Docs:   d.Docs,
		}, "{}"),
		Status:      "published",
		PublishedAt: &publishedAt,
		CreatedBy:   pluginID,
		UpdatedBy:   platformActor,
	}
	record.CapabilitiesHash = hashStruct(struct {
		ID          string                   `json:"id"`
		Title       string                   `json:"title"`
		Description string                   `json:"description"`
		Categories  []string                 `json:"categories"`
		Intents     []string                 `json:"intents"`
		ToolScopes  []string                 `json:"tool_scopes"`
		Policy      capabilityPolicy         `json:"policy"`
		Protocols   []models.ProtocolBinding `json:"protocols"`
		Version     string                   `json:"version"`
	}{
		ID:          d.CapabilityID,
		Title:       d.Title,
		Description: d.Description,
		Categories:  d.Categories,
		Intents:     d.Intents,
		ToolScopes:  d.ToolScopes,
		Policy:      d.Policy,
		Protocols:   d.Protocols,
		Version:     pluginVersion,
	})
	record.ProtocolHash = hashStruct(d.Protocols)
	return record
}

func encodeJSONArray(values []string) datatypes.JSON {
	if len(values) == 0 {
		return datatypes.JSON([]byte("[]"))
	}
	return encodeJSONValue(values, "[]")
}

func encodeJSONValue(value interface{}, empty string) datatypes.JSON {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		if empty == "" {
			return datatypes.JSON([]byte("null"))
		}
		return datatypes.JSON([]byte(empty))
	}
	return datatypes.JSON(raw)
}

func hashStruct(value interface{}) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum[:])
}

func platformCapabilityDefinitions() []platformCapabilityDefinition {
	platformCapabilityCacheOnce.Do(func() {
		defs, err := loadPlatformCapabilityDefinitionsFromConfig()
		if err != nil {
			logPlatformCapabilityError(err)
		}
		if len(defs) == 0 {
			if err == nil {
				pxlog.Info(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[integration_gateway] no platform capability config found, using built-in defaults")
			}
			defs = builtinPlatformCapabilityDefinitions()
		}
		platformCapabilityCache = defs
	})
	return platformCapabilityCache
}

func builtinPlatformCapabilityDefinitions() []platformCapabilityDefinition {
	return []platformCapabilityDefinition{
		{
			CapabilityID: "com.corex.media.assets.read",
			Title:        "Media Assets Catalog",
			Description:  "列出与查询媒资资产，支持多存储驱动筛选。",
			Module:       "media",
			Categories:   []string{"media", "storage"},
			Intents:      []string{"media.assets.read"},
			ToolScopes:   []string{"media.assets"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/001-docs-media-storage/contracts/http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/media/v1/media_asset.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets",
					Method:    "GET",
					SchemaRef: "specs/001-docs-media-storage/contracts/http-openapi.yaml#/paths/~1media~1assets/get",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets/{uuid}",
					Method:    "GET",
					SchemaRef: "specs/001-docs-media-storage/contracts/http-openapi.yaml#/paths/~1media~1assets~1{uuid}/get",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.media.v1.MediaAssetAdminService",
					RPC:       "ListMediaAssets",
					SchemaRef: "backend/api/grpc/contracts/powerx/media/v1/media_asset.proto#MediaAssetAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.media.v1.MediaAssetAdminService",
					RPC:       "GetMediaAsset",
					SchemaRef: "backend/api/grpc/contracts/powerx/media/v1/media_asset.proto#MediaAssetAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
			},
		},
		{
			CapabilityID: "com.corex.media.assets.manage",
			Title:        "Media Assets Management",
			Description:  "创建、删除媒资，以及生成上传/下载预签名链接。",
			Module:       "media",
			Categories:   []string{"media", "storage"},
			Intents:      []string{"media.assets.write"},
			ToolScopes:   []string{"media.assets"},
			Policy: capabilityPolicy{
				Prefer:   "grpc",
				Fallback: []string{"rest"},
			},
			Docs: []string{
				"specs/001-docs-media-storage/contracts/http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/media/v1/media_asset.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets",
					Method:    "POST",
					SchemaRef: "specs/001-docs-media-storage/contracts/http-openapi.yaml#/paths/~1media~1assets/post",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets/{uuid}",
					Method:    "DELETE",
					SchemaRef: "specs/001-docs-media-storage/contracts/http-openapi.yaml#/paths/~1media~1assets~1{uuid}/delete",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets/{uuid}",
					Method:    "PUT",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/media/assets/{uuid}/presign",
					Method:    "POST",
					SchemaRef: "specs/001-docs-media-storage/contracts/http-openapi.yaml#/paths/~1media~1assets~1{uuid}~1presign/post",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.media.v1.MediaAssetAdminService",
					RPC:       "CreateMediaAsset",
					SchemaRef: "backend/api/grpc/contracts/powerx/media/v1/media_asset.proto#MediaAssetAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.media.v1.MediaAssetAdminService",
					RPC:       "DeleteMediaAsset",
					SchemaRef: "backend/api/grpc/contracts/powerx/media/v1/media_asset.proto#MediaAssetAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.media.v1.MediaAssetAdminService",
					RPC:       "PresignMediaAsset",
					SchemaRef: "backend/api/grpc/contracts/powerx/media/v1/media_asset.proto#MediaAssetAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "media.assets",
				},
			},
		},
		{
			CapabilityID: "com.corex.eventfabric.publish",
			Title:        "Event Fabric Publish & Subscribe",
			Description:  "使用 CoreX Event Fabric 发布事件、订阅回执并执行业务回放。",
			Module:       "event_fabric",
			Categories:   []string{"event", "integration"},
			Intents:      []string{"event.fabric.publish"},
			ToolScopes:   []string{"event.fabric"},
			Policy: capabilityPolicy{
				Prefer:   "grpc",
				Fallback: []string{},
			},
			Docs: []string{
				"backend/api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "grpc",
					Endpoint:  "powerx.event_fabric.v1.EventDeliveryService",
					RPC:       "PublishEvent",
					SchemaRef: "backend/api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto#EventDeliveryService",
					AuthType:  "tenant_jwt",
					ToolScope: "event.fabric",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.event_fabric.v1.EventSubscriberService",
					RPC:       "Subscribe",
					SchemaRef: "backend/api/grpc/contracts/powerx/event_fabric/v1/event_fabric.proto#EventSubscriberService",
					AuthType:  "tenant_jwt",
					ToolScope: "event.fabric",
				},
			},
		},
		{
			CapabilityID: "com.corex.scheduler.jobs",
			Title:        "Runtime Scheduler Jobs",
			Description:  "创建、更新、查询、暂停、恢复与触发插件/核心模块的运行时调度任务。",
			Module:       "scheduler",
			Categories:   []string{"runtime", "scheduler"},
			Intents:      []string{"runtime.scheduler.jobs.manage", "runtime.scheduler.jobs.run", "runtime.scheduler.jobs.read"},
			ToolScopes:   []string{"scheduler.job.manage", "scheduler.job.run", "scheduler.job.read"},
			Policy: capabilityPolicy{
				Prefer:   "grpc",
				Fallback: []string{},
			},
			Docs: []string{
				"backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto",
				"specs/028-runtime-scheduler/contracts/http-openapi.yaml",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "grpc",
					Endpoint:  "powerx.scheduler.v1.SchedulerService",
					RPC:       "CreateJob",
					SchemaRef: "backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto#SchedulerService",
					AuthType:  "tenant_jwt",
					ToolScope: "scheduler.job.manage",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.scheduler.v1.SchedulerService",
					RPC:       "UpdateJob",
					SchemaRef: "backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto#SchedulerService",
					AuthType:  "tenant_jwt",
					ToolScope: "scheduler.job.manage",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.scheduler.v1.SchedulerService",
					RPC:       "TriggerJob",
					SchemaRef: "backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto#SchedulerService",
					AuthType:  "tenant_jwt",
					ToolScope: "scheduler.job.run",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.scheduler.v1.SchedulerService",
					RPC:       "ListJobs",
					SchemaRef: "backend/api/grpc/contracts/powerx/scheduler/v1/scheduler.proto#SchedulerService",
					AuthType:  "tenant_jwt",
					ToolScope: "scheduler.job.read",
				},
			},
		},
		{
			CapabilityID: "com.corex.workflow.builder",
			Title:        "Workflow Definition Builder",
			Description:  "在 CoreX Workflow Builder 中创建、发布与管理模板。",
			Module:       "workflow",
			Categories:   []string{"workflow"},
			Intents:      []string{"workflow.builder.manage"},
			ToolScopes:   []string{"workflow.builder"},
			Policy: capabilityPolicy{
				Prefer:   "grpc",
				Fallback: []string{},
			},
			Docs: []string{
				"backend/api/grpc/contracts/powerx/workflow/v1/workflow.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "grpc",
					Endpoint:  "powerx.workflow.v1.WorkflowService",
					RPC:       "CreateDefinition",
					SchemaRef: "backend/api/grpc/contracts/powerx/workflow/v1/workflow.proto#WorkflowService",
					AuthType:  "tenant_jwt",
					ToolScope: "workflow.builder",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.workflow.v1.WorkflowService",
					RPC:       "PublishDefinition",
					SchemaRef: "backend/api/grpc/contracts/powerx/workflow/v1/workflow.proto#WorkflowService",
					AuthType:  "tenant_jwt",
					ToolScope: "workflow.builder",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.workflow.v1.WorkflowService",
					RPC:       "ListDefinitions",
					SchemaRef: "backend/api/grpc/contracts/powerx/workflow/v1/workflow.proto#WorkflowService",
					AuthType:  "tenant_jwt",
					ToolScope: "workflow.builder",
				},
			},
		},
		{
			CapabilityID: "com.corex.knowledge.space",
			Title:        "Knowledge Space Admin",
			Description:  "管理知识空间配额、策略与增量任务，供插件直接复用。",
			Module:       "knowledge_space",
			Categories:   []string{"knowledge", "ai"},
			Intents:      []string{"knowledge.space.manage"},
			ToolScopes:   []string{"knowledge.space"},
			Policy: capabilityPolicy{
				Prefer:   "grpc",
				Fallback: []string{},
			},
			Docs: []string{
				"backend/api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "grpc",
					Endpoint:  "powerx.knowledge.v1.KnowledgeSpaceAdminService",
					RPC:       "CreateKnowledgeSpace",
					SchemaRef: "backend/api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto#KnowledgeSpaceAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "knowledge.space",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.knowledge.v1.KnowledgeSpaceAdminService",
					RPC:       "UpdateKnowledgeSpace",
					SchemaRef: "backend/api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto#KnowledgeSpaceAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "knowledge.space",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.knowledge.v1.KnowledgeSpaceAdminService",
					RPC:       "TriggerIngestion",
					SchemaRef: "backend/api/grpc/contracts/powerx/knowledge/v1/knowledge_space.proto#KnowledgeSpaceAdminService",
					AuthType:  "tenant_jwt",
					ToolScope: "knowledge.space",
				},
			},
		},
		{
			CapabilityID: "com.corex.agent.invoke",
			Title:        "Agent Invoke",
			Description:  "非流式调用 Agent 对话。",
			Module:       "agent",
			Categories:   []string{"agent", "ai"},
			Intents:      []string{"agent.invoke"},
			ToolScopes:   []string{"agent.runtime"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/agents/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml#/paths/~1agents~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.runtime",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.agent.v1.AgentInvokeService",
					RPC:       "Invoke",
					SchemaRef: "backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto#AgentInvokeService",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.runtime",
				},
			},
		},
		{
			CapabilityID: "com.corex.agent.stream",
			Title:        "Agent Stream",
			Description:  "通过 SSE/gRPC 流式输出 Agent 对话内容。",
			Module:       "agent",
			Categories:   []string{"agent", "ai"},
			Intents:      []string{"agent.stream"},
			ToolScopes:   []string{"agent.runtime"},
			Policy: capabilityPolicy{
				Prefer: "rest",
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/agent/v1/stream.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/agents/stream/sse",
					Method:    "GET",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml#/paths/~1agents~1stream~1sse/get",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.runtime",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.agent.v1.AgentStreamService",
					RPC:       "Stream",
					SchemaRef: "backend/api/grpc/contracts/powerx/agent/v1/stream.proto#AgentStreamService",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.runtime",
				},
			},
		},
		{
			CapabilityID: "com.corex.agent.session.manage",
			Title:        "Agent Session Management",
			Description:  "创建会话与管理消息。",
			Module:       "agent",
			Categories:   []string{"agent", "ai"},
			Intents:      []string{"agent.session.manage"},
			ToolScopes:   []string{"agent.session"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/agents/sessions",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/agent.http-openapi.yaml#/paths/~1agents~1sessions/post",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.session",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.agent.v1.AgentSessionService",
					RPC:       "CreateSession",
					SchemaRef: "backend/api/grpc/contracts/powerx/agent/v1/agent_api.proto#AgentSessionService",
					AuthType:  "tenant_jwt",
					ToolScope: "agent.session",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.llm.invoke",
			Title:        "LLM Invoke",
			Description:  "大语言模型无状态调用。",
			Module:       "ai",
			Categories:   []string{"ai", "llm"},
			Intents:      []string{"ai.llm.invoke"},
			ToolScopes:   []string{"ai.llm"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/llm/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1llm~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalService",
					RPC:       "Invoke",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.llm.stream",
			Title:        "LLM Stream",
			Description:  "LLM 无状态流式输出。",
			Module:       "ai",
			Categories:   []string{"ai", "llm"},
			Intents:      []string{"ai.llm.stream"},
			ToolScopes:   []string{"ai.llm"},
			Policy: capabilityPolicy{
				Prefer: "rest",
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/llm/stream",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1llm~1stream/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalService",
					RPC:       "Stream",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.llm.session.create",
			Title:        "LLM Session Create",
			Description:  "创建 LLM 会话。",
			Module:       "ai",
			Categories:   []string{"ai", "llm"},
			Intents:      []string{"ai.llm.session.create"},
			ToolScopes:   []string{"ai.llm.session"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/llm/sessions",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1llm~1sessions/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm.session",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalSessionService",
					RPC:       "CreateSession",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalSessionService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm.session",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.llm.session.append",
			Title:        "LLM Session Append",
			Description:  "追加 LLM 会话消息。",
			Module:       "ai",
			Categories:   []string{"ai", "llm"},
			Intents:      []string{"ai.llm.session.append"},
			ToolScopes:   []string{"ai.llm.session"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/llm/sessions/{session_id}/messages",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1llm~1sessions~1{session_id}~1messages/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm.session",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalSessionService",
					RPC:       "AppendMessage",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalSessionService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.llm.session",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.image.invoke",
			Title:        "Image Invoke",
			Description:  "图像生成/理解调用。",
			Module:       "ai",
			Categories:   []string{"ai", "image"},
			Intents:      []string{"ai.image.invoke"},
			ToolScopes:   []string{"ai.image"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/image/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1image~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.image",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalService",
					RPC:       "Invoke",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.image",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.video.invoke",
			Title:        "Video Invoke",
			Description:  "视频生成/理解调用。",
			Module:       "ai",
			Categories:   []string{"ai", "video"},
			Intents:      []string{"ai.video.invoke"},
			ToolScopes:   []string{"ai.video"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/video/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1video~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.video",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalService",
					RPC:       "Invoke",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.video",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.tts.invoke",
			Title:        "TTS Invoke",
			Description:  "语音合成调用。",
			Module:       "ai",
			Categories:   []string{"ai", "tts"},
			Intents:      []string{"ai.tts.invoke"},
			ToolScopes:   []string{"ai.tts"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/tts/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1tts~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.tts",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.MultimodalService",
					RPC:       "Invoke",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#MultimodalService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.tts",
				},
			},
		},
		{
			CapabilityID: "com.corex.ai.embedding.invoke",
			Title:        "Embedding Invoke",
			Description:  "向量生成（embedding）。",
			Module:       "ai",
			Categories:   []string{"ai", "embedding"},
			Intents:      []string{"ai.embedding.invoke"},
			ToolScopes:   []string{"ai.embedding"},
			Policy: capabilityPolicy{
				Prefer:   "rest",
				Fallback: []string{"grpc"},
			},
			Docs: []string{
				"specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml",
				"backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto",
			},
			Protocols: []models.ProtocolBinding{
				{
					Channel:   "rest",
					Endpoint:  "/api/v1/ai/embedding/invoke",
					Method:    "POST",
					SchemaRef: "specs/007-integration-gateway-and-mcp/contracts/ai-multimodal.http-openapi.yaml#/paths/~1ai~1embedding~1invoke/post",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.embedding",
				},
				{
					Channel:   "grpc",
					Endpoint:  "powerx.ai.v1.EmbeddingService",
					RPC:       "Embed",
					SchemaRef: "backend/api/grpc/contracts/powerx/ai/v1/multimodal.proto#EmbeddingService",
					AuthType:  "tenant_jwt",
					ToolScope: "ai.embedding",
				},
			},
		},
	}
}

func (s *BaseCapabilitySeeder) ensurePlatformRegistrations(ctx context.Context, defs []platformCapabilityDefinition) error {
	tenantUUIDs, err := s.tenantRepo.ListActiveUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("list tenants for platform registrations failed: %w", err)
	}
	if len(tenantUUIDs) == 0 {
		if s.logger != nil {
			s.logger.WarnF(ctx, "[integration_gateway] skip platform registrations seeding: no active tenants")
		}
		return nil
	}
	var errs []error
	now := s.clock()
	for _, tenantUUID := range tenantUUIDs {
		for _, def := range defs {
			if err := s.ensureTenantRegistration(ctx, tenantUUID, def, now); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if s.logger != nil {
		s.logger.InfoF(ctx, "[integration_gateway] platform registrations ensured tenants=%d capabilities=%d", len(tenantUUIDs), len(defs))
	}
	return nil
}

func (s *BaseCapabilitySeeder) ensureTenantRegistration(ctx context.Context, tenantUUID string, def platformCapabilityDefinition, publishedAt time.Time) error {
	if s.registry == nil {
		return nil
	}
	if tenantUUID == "" || def.CapabilityID == "" {
		return nil
	}
	_, err := s.registry.GetLatest(ctx, nil, def.CapabilityID, tenantUUID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return fmt.Errorf("load registration %s/%s failed: %w", tenantUUID, def.CapabilityID, err)
	}
	reg := s.buildPlatformRegistration(def, tenantUUID, publishedAt)
	if len(reg.Adapters) == 0 {
		return fmt.Errorf("platform registration %s missing adapters", def.CapabilityID)
	}
	_, err = s.registry.Create(ctx, nil, reg)
	if err != nil {
		return fmt.Errorf("create platform registration %s/%s failed: %w", tenantUUID, def.CapabilityID, err)
	}
	return nil
}

func (s *BaseCapabilitySeeder) buildPlatformRegistration(def platformCapabilityDefinition, tenantUUID string, publishedAt time.Time) repo.Registration {
	adapters := s.buildAdapters(def)
	fallbackSeq := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		fallbackSeq = append(fallbackSeq, adapter.AdapterID)
	}
	if len(adapters) == 0 {
		adapters = []repo.AdapterEndpoint{
			{
				AdapterID:     makeAdapterID(def.CapabilityID, "internal", 0),
				TransportType: "internal",
				Endpoint:      fmt.Sprintf("internal://%s", def.CapabilityID),
				ServiceRef:    fmt.Sprintf("internal://%s", def.CapabilityID),
				Weight:        100,
				TimeoutMS:     5000,
				Labels:        map[string]string{"reason": "generated"},
				IsActive:      true,
			},
		}
		fallbackSeq = []string{adapters[0].AdapterID}
	}
	reg := repo.Registration{
		CapabilityID: def.CapabilityID,
		TenantUUID:   tenantUUID,
		ContractRef: func() string {
			if len(def.Docs) > 0 {
				return def.Docs[0]
			}
			if def.Module != "" {
				return fmt.Sprintf("%s/%s", def.Module, def.CapabilityID)
			}
			return fmt.Sprintf("corex://%s", def.CapabilityID)
		}(),
		Status: "published",
		EnvironmentPolicies: map[string]repo.EnvironmentPolicy{
			"default": {
				IsEnabled: true,
			},
		},
		Adapters: adapters,
		RoutingPolicy: repo.RoutingPolicy{
			Strategy:         "weighted",
			FallbackSequence: fallbackSeq,
			CooldownSeconds:  0,
		},
		ToolGrantIDs: def.ToolScopes,
		UpdatedBy:    platformActor,
	}
	reg.PublishedAt = &publishedAt
	return reg
}

func (s *BaseCapabilitySeeder) buildAdapters(def platformCapabilityDefinition) []repo.AdapterEndpoint {
	adapters := make([]repo.AdapterEndpoint, 0, len(def.Protocols))
	seenChannels := make(map[string]struct{}, len(def.Protocols))
	for idx, binding := range def.Protocols {
		channel := strings.ToLower(strings.TrimSpace(binding.Channel))
		if channel == "" {
			continue
		}
		if _, ok := seenChannels[channel]; ok {
			continue
		}
		seenChannels[channel] = struct{}{}
		adapterID := makeAdapterID(def.CapabilityID, channel, idx)
		endpoint := strings.TrimSpace(binding.Endpoint)
		if endpoint == "" {
			endpoint = strings.TrimSpace(binding.RPC)
		}
		if endpoint == "" {
			endpoint = fmt.Sprintf("internal://%s/%s", def.CapabilityID, channel)
		}
		labels := map[string]string{}
		if binding.Method != "" {
			labels["method"] = strings.ToUpper(strings.TrimSpace(binding.Method))
		}
		if binding.RPC != "" {
			labels["rpc"] = strings.TrimSpace(binding.RPC)
		}
		if binding.SchemaRef != "" {
			labels["schema_ref"] = binding.SchemaRef
		}
		if binding.AuthType != "" {
			labels["auth_type"] = binding.AuthType
		}
		if binding.ToolScope != "" {
			labels["tool_scope"] = binding.ToolScope
		}
		adapters = append(adapters, repo.AdapterEndpoint{
			AdapterID:     adapterID,
			TransportType: transportFromChannel(channel),
			Endpoint:      endpoint,
			ServiceRef:    endpoint,
			Weight:        100,
			TimeoutMS:     5000,
			Labels:        labels,
			IsActive:      true,
		})
	}
	return adapters
}

func transportFromChannel(channel string) string {
	switch strings.ToLower(channel) {
	case "rest", "http", "https":
		return "http"
	case "grpc":
		return "grpc"
	case "mcp":
		return "mcp"
	default:
		return channel
	}
}

func makeAdapterID(capabilityID, channel string, idx int) string {
	prefix := slugify(capabilityID)
	channelSlug := slugify(channel)
	if channelSlug != "" {
		prefix = fmt.Sprintf("%s-%s", prefix, channelSlug)
	}
	if idx > 0 {
		prefix = fmt.Sprintf("%s-%d", prefix, idx)
	}
	return prefix
}

func slugify(value string) string {
	replacer := strings.NewReplacer(
		" ", "-",
		".", "-",
		"/", "-",
		"{", "",
		"}", "",
		":", "-",
	)
	slug := replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	if slug == "" {
		return "adapter"
	}
	return slug
}
