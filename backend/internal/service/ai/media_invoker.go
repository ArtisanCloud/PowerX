package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"sort"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

const MediaCatalogEndpoint = "core://ai/model-catalog"
const MediaInvokeEndpoint = "core://ai/media"
const MediaTasksEndpoint = "core://ai/media-tasks"

type MediaCatalogOperation struct {
	Operation  string `json:"operation"`
	Provider   string `json:"provider,omitempty"`
	App        string `json:"app,omitempty"`
	Capability string `json:"capability,omitempty"`
}
type MediaInvokeOperation struct {
	Operation       string         `json:"operation"`
	ContractVersion int            `json:"contract_version"`
	ModelKey        string         `json:"model_key"`
	Inputs          []ContentItem  `json:"inputs"`
	Params          map[string]any `json:"params,omitempty"`
	IdempotencyKey  string         `json:"idempotency_key,omitempty"`
}
type MediaTaskOperation struct {
	Operation string `json:"operation"`
	TaskUUID  string `json:"task_uuid"`
}
type MediaInvoker struct{ Service *MediaRuntime }

func strictMediaBody(body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return MediaError(400, "AI_OPERATION_INVALID")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return MediaError(400, "AI_OPERATION_INVALID")
	}
	return nil
}
func (i *MediaInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	ability := ""
	endpoint := ""
	switch in.CapabilityID {
	case ConnectionTestCapability:
		endpoint = ConnectionTestEndpoint
	case MediaCatalogCapability, "com.corex.ai.catalog.providers.read":
		endpoint = MediaCatalogEndpoint
	case MediaTaskReadCapability, MediaTaskCancelCapability:
		endpoint = MediaTasksEndpoint
	default:
		for _, a := range []string{catalog.VisionUnderstanding, catalog.ImageGeneration, catalog.VideoGeneration, catalog.SpeechSynthesis} {
			if InvokeCapability(a) == in.CapabilityID {
				ability = a
				endpoint = MediaInvokeEndpoint
			}
		}
	}
	if endpoint == "" {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if in.Method != "INVOKE" || in.Endpoint != endpoint || len(in.Query) > 0 || len(in.Context) > 0 {
		return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
	}
	for key := range in.Payload {
		if key != "body" {
			return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
		}
	}
	if i == nil || i.Service == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	s := i.Service
	tenant := reqctx.GetTenantUUID(ctx)
	if tenant != in.TenantUUID {
		return nil, MediaError(403, "AI_FORBIDDEN")
	}
	if _, err := s.ai.AuthorizeMedia(ctx, tenant, in.CapabilityID); err != nil {
		return nil, err
	}
	if endpoint == ConnectionTestEndpoint {
		return s.ai.testSavedConnection(ctx, in.Body)
	}
	if endpoint == MediaCatalogEndpoint {
		// 先读取判别字段，再按各自 DTO 严格解码；Settings 字段不能混入媒体 v2 操作。
		var discriminator struct {
			Operation string `json:"operation"`
		}
		raw, err := json.Marshal(in.Body)
		if err != nil || json.Unmarshal(raw, &discriminator) != nil {
			return nil, MediaError(400, "AI_OPERATION_INVALID")
		}
		if discriminator.Operation == settingsProvidersOperation || discriminator.Operation == settingsModelsOperation {
			return s.ai.settingsCatalog(ctx, in.CapabilityID, discriminator.Operation, in.Body)
		}
		var op MediaCatalogOperation
		if err := strictMediaBody(in.Body, &op); err != nil {
			return nil, err
		}
		if in.CapabilityID == "com.corex.ai.catalog.providers.read" {
			if op.Operation != "list_providers" || op.Provider != "" || op.App != "" {
				return nil, MediaError(400, "AI_OPERATION_INVALID")
			}
			seen := map[string]bool{}
			providers := []map[string]any{}
			for _, model := range s.ai.mediaRegistry().MediaModels("", "", op.Capability) {
				if !seen[model.Provider] {
					seen[model.Provider] = true
					manifest, _ := s.ai.mediaRegistry().Manifest(model.Provider)
					providers = append(providers, map[string]any{"provider": model.Provider, "label": manifest.Name})
				}
			}
			sort.Slice(providers, func(a, b int) bool { return providers[a]["provider"].(string) < providers[b]["provider"].(string) })
			return map[string]any{"result": map[string]any{"contract_version": 2, "providers": providers}}, nil
		}
		if op.Operation != "list_models" {
			return nil, MediaError(400, "AI_OPERATION_INVALID")
		}
		env, ok, err := s.ai.ResolveTenantEnv(ctx, tenant)
		if err != nil || !ok {
			return nil, MediaError(409, "AI_ENV_NOT_CONFIGURED")
		}
		items, err := s.ai.ListMediaModels(ctx, env, tenant, op.Provider, op.App, op.Capability)
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": map[string]any{"contract_version": 2, "items": items}}, nil
	}
	if endpoint == MediaTasksEndpoint {
		var op MediaTaskOperation
		if err := strictMediaBody(in.Body, &op); err != nil {
			return nil, err
		}
		var result *MediaTaskView
		var err error
		if in.CapabilityID == MediaTaskReadCapability && op.Operation == "get" {
			result, err = s.Get(ctx, op.TaskUUID)
		} else if in.CapabilityID == MediaTaskCancelCapability && op.Operation == "cancel" {
			result, err = s.Cancel(ctx, op.TaskUUID)
		} else {
			return nil, MediaError(400, "AI_OPERATION_INVALID")
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": result}, nil
	}
	var op MediaInvokeOperation
	if err := strictMediaBody(in.Body, &op); err != nil {
		return nil, err
	}
	if op.Operation != "invoke" || op.ContractVersion != 2 {
		return nil, MediaError(409, "AI_CONTRACT_VERSION_REQUIRED")
	}
	result, err := s.Invoke(ctx, ability, op)
	if err != nil {
		return nil, err
	}
	return map[string]any{"result": result}, nil
}
func (s *MediaRuntime) Invoke(ctx context.Context, ability string, op MediaInvokeOperation) (any, error) {
	if s == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if op.ContractVersion != 2 {
		return nil, MediaError(409, "AI_CONTRACT_VERSION_REQUIRED")
	}
	if ability == catalog.VisionUnderstanding {
		tenant := reqctx.GetTenantUUID(ctx)
		env, ok, err := s.ai.ResolveTenantEnv(ctx, tenant)
		if err != nil || !ok {
			return nil, MediaError(409, "AI_ENV_NOT_CONFIGURED")
		}
		result, err := s.ai.VLMInvoke(ctx, env, tenant, op.ModelKey, op.Inputs, op.Params)
		if err != nil {
			return nil, err
		}
		result["contract_version"] = 2
		trace := reqctx.GetTraceID(ctx)
		if trace == "" {
			trace = uuid.NewString()
		}
		result["trace_id"] = trace
		return result, nil
	}
	return s.Submit(ctx, MediaSubmitInput{op.ModelKey, ability, op.Inputs, op.Params, op.IdempotencyKey})
}
