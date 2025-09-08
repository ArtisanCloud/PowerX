// file: internal/transport/grpc/agent/setting_handler.go
package agentgrpc

import (
	"context"
	"github.com/jinzhu/copier"
	"strings"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	v1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"

	"google.golang.org/protobuf/types/known/structpb"
	"gorm.io/datatypes"
)

/*************** Server ***************/
type AgentSettingServer struct {
	v1.UnimplementedAgentSettingServiceServer
	svc *agentSvc.AgentSettingService
}

func NewAgentSettingServer(deps *shared.Deps) *AgentSettingServer {
	return &AgentSettingServer{svc: agentSvc.NewAgentSettingService(deps.DB)}
}

/*************** Providers / Models ***************/

func (s *AgentSettingServer) ListProviders(ctx context.Context, req *v1.ListProvidersRequest) (*v1.ListProvidersResponse, error) {
	mod := modalityToString(req.GetModality())
	list := s.svc.Providers(mod) // 期望返回 []string（与 HTTP 返回一致）
	var providerLists []*v1.AIProviderItem
	err := copier.Copy(&providerLists, list)
	if err != nil {
		return nil, err
	}
	return &v1.ListProvidersResponse{Providers: providerLists}, nil
}

func (s *AgentSettingServer) ListModels(ctx context.Context, req *v1.ListModelsRequest) (*v1.ListModelsResponse, error) {
	mod := modalityToString(req.GetModality())
	models, err := s.svc.Models(mod, strings.TrimSpace(req.GetProvider()))
	if err != nil {
		return nil, err
	}
	return &v1.ListModelsResponse{Models: models}, nil
}

/*************** Settings：保存 / 测试 / 快测 ***************/

func (s *AgentSettingServer) SaveSettings(ctx context.Context, req *v1.SaveSettingsRequest) (*v1.SaveSettingsResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())
	mod := modalityToString(req.GetModality())

	// 与 HTTP 一致：LLM 先直连校验
	if req.GetModality() == v1.Modality_MODALITY_LLM {
		base := req.GetLlm().GetBase()
		if err := s.svc.PingLLM(ctx, env, &tid, base.GetProvider(), base.GetModel(), base.GetBaseUrl(), base.GetApiKey()); err != nil {
			return &v1.SaveSettingsResponse{Ok: false}, err
		}
	}

	cred, prof := buildCredentialAndProfileFromReq(env, tid, mod, req)
	if cred == nil || prof == nil {
		return &v1.SaveSettingsResponse{Ok: false}, nil
	}
	if err := s.svc.SaveCredentialAndProfile(ctx, env, &tid, cred, prof, true); err != nil {
		return nil, err
	}
	return &v1.SaveSettingsResponse{Ok: true}, nil
}

func (s *AgentSettingServer) TestConnection(ctx context.Context, req *v1.TestConnectionRequest) (*v1.TestConnectionResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())
	mod := modalityToString(req.GetModality())

	provider, model, baseURL, apiKey := pickProviderModelConn(req.GetModality(), req)
	if err := s.svc.TestConnectionPreferInput(ctx, env, &tid, mod, provider, model, baseURL, apiKey); err != nil {
		return &v1.TestConnectionResponse{Ok: false, Message: err.Error()}, nil
	}
	return &v1.TestConnectionResponse{Ok: true}, nil
}

func (s *AgentSettingServer) TestQuickCall(ctx context.Context, req *v1.TestQuickCallRequest) (*v1.TestQuickCallResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())

	if req.GetModality() != v1.Modality_MODALITY_LLM {
		return &v1.TestQuickCallResponse{Ok: false, Message: "only LLM supported"}, nil
	}
	llm := req.GetLlm()
	base := llm.GetBase()
	out, err := s.svc.QuickCallLLM(ctx, env, &tid,
		base.GetProvider(), base.GetModel(), base.GetBaseUrl(), base.GetApiKey(),
		llm.GetTemperature(), int(llm.GetMaxTokens()),
		strings.TrimSpace(req.GetPrompt()),
	)
	if err != nil {
		return &v1.TestQuickCallResponse{Ok: false, Message: err.Error()}, nil
	}
	return &v1.TestQuickCallResponse{Ok: true, Result: out}, nil
}

/*************** Profiles：获取 / 设置 / 列表 / 凭据 ***************/

func (s *AgentSettingServer) GetActiveProfile(ctx context.Context, req *v1.GetActiveProfileRequest) (*v1.GetActiveProfileResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())
	mod := modalityToString(req.GetModality())

	prof, err := s.svc.GetActiveProfile(ctx, env, &tid, mod)
	if err != nil || prof == nil {
		return nil, err
	}
	return &v1.GetActiveProfileResponse{
		Env:      env,
		Modality: mod,
		Profile:  toProtoProfile(prof),
	}, nil
}

func (s *AgentSettingServer) SetActiveProfile(ctx context.Context, req *v1.SetActiveProfileRequest) (*v1.SetActiveProfileResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())

	if err := s.svc.SetActiveProfile(ctx, env, &tid, strings.TrimSpace(req.GetModality()), strings.TrimSpace(req.GetProvider()), strings.TrimSpace(req.GetModel())); err != nil {
		return nil, err
	}
	return &v1.SetActiveProfileResponse{Ok: true}, nil
}

func (s *AgentSettingServer) ListProfiles(ctx context.Context, req *v1.ListProfilesRequest) (*v1.ListProfilesResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())

	var mods []string
	for _, m := range req.GetModalities() {
		mods = append(mods, modalityToString(m))
	}
	out, err := s.svc.ListProfiles(ctx, env, &tid, mods...)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListProfilesResponse{Env: env}
	for _, p := range out {
		pp := p
		resp.Profiles = append(resp.Profiles, toProtoProfile(&pp))
	}
	return resp, nil
}

func (s *AgentSettingServer) ListCredentials(ctx context.Context, req *v1.ListCredentialsRequest) (*v1.ListCredentialsResponse, error) {
	env := firstNonEmpty(strings.TrimSpace(req.GetEnv()), firstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tid := tenantFromReqOrCtx(ctx, req.GetCtx())

	out, err := s.svc.ListCredentials(ctx, env, &tid)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListCredentialsResponse{Env: env}
	for _, c := range out {
		cc := c
		resp.Credentials = append(resp.Credentials, &v1.AIProviderCredential{
			Name:       cc.Name,
			Provider:   cc.Provider,
			AuthScheme: cc.AuthScheme,
			Data:       jsonMapToStruct(cc.Data),
		})
	}
	return resp, nil
}

/*************** helpers：装配层（与 HTTP Handler 对齐） ***************/

// 从请求或 context 里解析 tenant_id（优先 req.ctx.tenant_id）
func tenantFromReqOrCtx(ctx context.Context, rctx *commonv1.RequestContext) uint64 {
	if rctx != nil && rctx.GetTenantId() > 0 {
		return uint64(rctx.GetTenantId())
	}
	if tid := reqctx.GetTenantID(ctx); tid > 0 {
		return tid
	}
	return 0
}

func modalityToString(m v1.Modality) string {
	switch m {
	case v1.Modality_MODALITY_LLM:
		return "llm"
	case v1.Modality_MODALITY_IMAGE:
		return "image"
	case v1.Modality_MODALITY_EMBEDDING:
		return "embedding"
	case v1.Modality_MODALITY_VIDEO:
		return "video"
	default:
		return "llm"
	}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func jsonMapToStruct(m datatypes.JSONMap) *structpb.Struct {
	if m == nil {
		s, _ := structpb.NewStruct(map[string]any{})
		return s
	}
	s, _ := structpb.NewStruct(map[string]any(m))
	return s
}

func toProtoProfile(p *dbmodel.AIModelProfile) *v1.AIModelProfile {
	if p == nil {
		return nil
	}
	return &v1.AIModelProfile{
		Modality: p.Modality,
		Provider: p.Provider,
		Model:    p.Model,
		Tags:     []string(p.Tags),
		Defaults: jsonMapToStruct(p.Defaults),
		// 注意：你的 DB 结构没有 Meta，这里不再返回
	}
}

// 组装 Credential + Profile（各模态字段对齐 HTTP 表单；Seal/落库由 Service 内部实现）
func buildCredentialAndProfileFromReq(env string, tenantID uint64, mod string, req *v1.SaveSettingsRequest) (*dbmodel.AIProviderCredential, *dbmodel.AIModelProfile) {
	var provider, model, baseURL, apiKey string
	data := datatypes.JSONMap{}
	defaults := datatypes.JSONMap{}

	switch mod {
	case "llm":
		if req.GetLlm() == nil || req.GetLlm().GetBase() == nil {
			return nil, nil
		}
		base := req.GetLlm().GetBase()
		provider = strings.TrimSpace(base.GetProvider())
		model = strings.TrimSpace(base.GetModel())
		baseURL = strings.TrimSpace(base.GetBaseUrl())
		apiKey = strings.TrimSpace(base.GetApiKey())
		if v := base.GetRegion(); v != "" {
			data["region"] = v
		}
		if v := base.GetOrganization(); v != "" {
			data["organization"] = v
		}
		if v := base.GetAzureDeployment(); v != "" {
			data["azure_deployment"] = v
		}
		if t := req.GetLlm().GetTemperature(); t != 0 {
			defaults["temperature"] = t
		}
		if mt := req.GetLlm().GetMaxTokens(); mt != 0 {
			defaults["maxTokens"] = mt
		}
		if tp := req.GetLlm().GetTopP(); tp != 0 {
			defaults["topP"] = tp
		}
		defaults["stream"] = req.GetLlm().GetStream()

	case "image":
		if req.GetImage() == nil || req.GetImage().GetBase() == nil {
			return nil, nil
		}
		base := req.GetImage().GetBase()
		provider = strings.TrimSpace(base.GetProvider())
		model = strings.TrimSpace(base.GetModel())
		baseURL = strings.TrimSpace(base.GetBaseUrl())
		apiKey = strings.TrimSpace(base.GetApiKey())
		if v := req.GetImage().GetSize(); v != "" {
			defaults["size"] = v
		}
		if v := req.GetImage().GetQuality(); v != "" {
			defaults["quality"] = v
		}
		if v := req.GetImage().GetFormat(); v != "" {
			defaults["format"] = v
		}
		if v := req.GetImage().GetPromptHint(); v != "" {
			defaults["promptHint"] = v
		}

	case "embedding":
		if req.GetEmbedding() == nil || req.GetEmbedding().GetBase() == nil {
			return nil, nil
		}
		base := req.GetEmbedding().GetBase()
		provider = strings.TrimSpace(base.GetProvider())
		model = strings.TrimSpace(base.GetModel())
		baseURL = strings.TrimSpace(base.GetBaseUrl())
		apiKey = strings.TrimSpace(base.GetApiKey())
		if v := req.GetEmbedding().GetDimensions(); v != 0 {
			defaults["dimensions"] = v
		}
		if v := req.GetEmbedding().GetTruncate(); v != "" {
			defaults["truncate"] = v
		}
		if v := req.GetEmbedding().GetBatch(); v != 0 {
			defaults["batch"] = v
		}

	case "video":
		if req.GetVideo() == nil || req.GetVideo().GetBase() == nil {
			return nil, nil
		}
		base := req.GetVideo().GetBase()
		provider = strings.TrimSpace(base.GetProvider())
		model = strings.TrimSpace(base.GetModel())
		baseURL = strings.TrimSpace(base.GetBaseUrl())
		apiKey = strings.TrimSpace(base.GetApiKey())
		if v := req.GetVideo().GetResolution(); v != "" {
			defaults["resolution"] = v
		}
		if v := req.GetVideo().GetFps(); v != 0 {
			defaults["fps"] = v
		}
		if v := req.GetVideo().GetMaxDurationSec(); v != 0 {
			defaults["maxDurationSec"] = v
		}
		if v := req.GetVideo().GetPromptHint(); v != "" {
			defaults["promptHint"] = v
		}
	default:
		return nil, nil
	}

	// 连接字段统一收集
	if baseURL != "" {
		data["base_url"] = baseURL
	}
	if apiKey != "" {
		data["api_key"] = apiKey
	}

	cred := &dbmodel.AIProviderCredential{
		Env:        env,
		TenantID:   &tenantID,
		Name:       utils.Slug(env + "-" + provider), // 与 HTTP 相同：e.g. "default-ollama"
		Provider:   provider,
		AuthScheme: "bearer",
		Data:       data,
	}

	prof := &dbmodel.AIModelProfile{
		Env:      env,
		TenantID: &tenantID,
		Modality: mod,
		Provider: provider,
		Model:    model,
		Defaults: defaults,
		Tags:     datatypes.JSONSlice[string]{mod},
	}
	return cred, prof
}

// 从 TestConnectionRequest 中挑 provider/model/base_url/api_key
func pickProviderModelConn(mod v1.Modality, req *v1.TestConnectionRequest) (provider, model, baseURL, apiKey string) {
	switch mod {
	case v1.Modality_MODALITY_LLM:
		if req.GetLlm() != nil && req.GetLlm().GetBase() != nil {
			b := req.GetLlm().GetBase()
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case v1.Modality_MODALITY_IMAGE:
		if req.GetImage() != nil && req.GetImage().GetBase() != nil {
			b := req.GetImage().GetBase()
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case v1.Modality_MODALITY_EMBEDDING:
		if req.GetEmbedding() != nil && req.GetEmbedding().GetBase() != nil {
			b := req.GetEmbedding().GetBase()
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case v1.Modality_MODALITY_VIDEO:
		if req.GetVideo() != nil && req.GetVideo().GetBase() != nil {
			b := req.GetVideo().GetBase()
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	}
	return "", "", "", ""
}
