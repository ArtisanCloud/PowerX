package agentgrpc

import (
	"context"
	"strings"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	settingv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/setting"
	"github.com/jinzhu/copier"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/datatypes"
)

/*************** Server ***************/
type SettingAIServiceServer struct {
	settingv1.UnimplementedSettingAIServiceServer
	svc *agentSvc.AgentSettingService
}

func NewSettingAIServiceServer(deps *shared.Deps) *SettingAIServiceServer {
	return &SettingAIServiceServer{
		svc: agentSvc.NewAgentSettingService(deps.DB),
	}
}

/*************** Meta helpers ***************/
func okMeta(ctx context.Context, reqID string) *commonv1.ResponseMeta {
	if reqID == "" {
		reqID = reqctx.GetTraceID(ctx)
	}
	return &commonv1.ResponseMeta{
		Code: 200, Message: "success", Timestamp: time.Now().Unix(), RequestId: reqID,
	}
}
func badMeta(ctx context.Context, code int32, msg, reqID string) *commonv1.ResponseMeta {
	return &commonv1.ResponseMeta{
		Code: code, Message: strings.TrimSpace(msg), Timestamp: time.Now().Unix(), RequestId: reqID,
	}
}

/*************** ListProviders ***************/
func (s *SettingAIServiceServer) ListProviders(ctx context.Context, req *settingv1.ListProvidersRequest) (*settingv1.ListProvidersResponse, error) {
	items := s.svc.Providers(modalityToString(req.GetModality())) // []aiProviderItem{ID,Name}

	var out []*settingv1.AIProviderItem
	if err := copier.Copy(&out, items); err != nil {
		for _, it := range items {
			out = append(out, &settingv1.AIProviderItem{Id: it.ID, Name: it.Name})
		}
	}
	return &settingv1.ListProvidersResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.ListProvidersData{Items: out},
	}, nil
}

/*************** ListModels ***************/
func (s *SettingAIServiceServer) ListModels(ctx context.Context, req *settingv1.ListModelsRequest) (*settingv1.ListModelsResponse, error) {
	models, err := s.svc.Models(modalityToString(req.GetModality()), strings.TrimSpace(req.GetProvider()))
	if err != nil {
		return &settingv1.ListModelsResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.ListModelsData{Items: nil},
		}, nil
	}
	return &settingv1.ListModelsResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.ListModelsData{Items: models},
	}, nil
}

/*************** SaveSettings ***************/
func (s *SettingAIServiceServer) SaveSettings(ctx context.Context, req *settingv1.SaveSettingsRequest) (*settingv1.SaveSettingsResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.SaveSettingsResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.SaveSettingsData{Ok: false},
		}, nil
	}
	tenantRef := tenantUUID
	mod := modalityToString(req.GetModality())

	// 直连校验（以 LLM 为例）
	if req.GetLlm() != nil {
		base := req.GetLlm().GetBase()
		if err := s.svc.PingLLM(ctx, env, &tenantRef, base.GetProvider(), base.GetModel(), base.GetBaseUrl(), base.GetApiKey(), "", "", base.GetRegion(), ""); err != nil {
			return &settingv1.SaveSettingsResponse{
				Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
				Data: &settingv1.SaveSettingsData{Ok: false},
			}, nil
		}
	}
	cred, prof := buildCredentialAndProfileFromSaveReq(env, &tenantRef, mod, req)
	if cred == nil || prof == nil {
		return &settingv1.SaveSettingsResponse{
			Meta: badMeta(ctx, 400, "invalid setting payload", req.GetCtx().GetRequestId()),
			Data: &settingv1.SaveSettingsData{Ok: false},
		}, nil
	}
	if err := s.svc.SaveCredentialAndProfile(ctx, env, &tenantRef, cred, prof, true); err != nil {
		return &settingv1.SaveSettingsResponse{
			Meta: badMeta(ctx, 500, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.SaveSettingsData{Ok: false},
		}, nil
	}
	return &settingv1.SaveSettingsResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.SaveSettingsData{Ok: true},
	}, nil
}

/*************** TestConnection ***************/
func (s *SettingAIServiceServer) TestConnection(ctx context.Context, req *settingv1.TestConnectionRequest) (*settingv1.TestConnectionResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.TestConnectionResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.TestConnectionData{Ok: false},
		}, nil
	}
	tenantRef := tenantUUID
	provider, model, baseURL, apiKey := pickProviderModelConn(req.GetModality(), req)
	if err := s.svc.TestConnectionPreferInput(ctx, env, &tenantRef, modalityToString(req.GetModality()), provider, model, baseURL, apiKey, "", "", "", ""); err != nil {
		return &settingv1.TestConnectionResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.TestConnectionData{Ok: false},
		}, nil
	}
	return &settingv1.TestConnectionResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.TestConnectionData{Ok: true},
	}, nil
}

/*************** TestQuickCall（LLM） ***************/
func (s *SettingAIServiceServer) TestQuickCall(ctx context.Context, req *settingv1.TestQuickCallRequest) (*settingv1.TestQuickCallResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.TestQuickCallResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.TestQuickCallData{Ok: false},
		}, nil
	}
	tenantRef := tenantUUID
	if req.GetLlm() == nil {
		return &settingv1.TestQuickCallResponse{
			Meta: badMeta(ctx, 400, "llm setting required", req.GetCtx().GetRequestId()),
			Data: &settingv1.TestQuickCallData{Ok: false},
		}, nil
	}
	b := req.GetLlm().GetBase()
	out, err := s.svc.QuickCallLLM(
		ctx, env, &tenantRef,
		b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey(), "", "", b.GetRegion(), "",
		req.GetLlm().GetTemperature(), int(req.GetLlm().GetMaxTokens()),
		strings.TrimSpace(req.GetPrompt()),
	)
	if err != nil {
		return &settingv1.TestQuickCallResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.TestQuickCallData{Ok: false},
		}, nil
	}
	return &settingv1.TestQuickCallResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.TestQuickCallData{Ok: true, Result: out},
	}, nil
}

/*************** Profiles/Credentials ***************/
func (s *SettingAIServiceServer) GetActiveProfile(ctx context.Context, req *settingv1.GetActiveProfileRequest) (*settingv1.GetActiveProfileResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.GetActiveProfileResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
		}, nil
	}
	tenantRef := tenantUUID
	prof, err := s.svc.GetActiveProfile(ctx, env, &tenantRef, modalityToString(req.GetModality()))
	if err != nil || prof == nil {
		return &settingv1.GetActiveProfileResponse{
			Meta: badMeta(ctx, 404, "not found", req.GetCtx().GetRequestId()),
		}, nil
	}
	return &settingv1.GetActiveProfileResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.GetActiveProfileData{
			Env: env, Modality: prof.Modality, Profile: toProtoProfile(prof),
		},
	}, nil
}

func (s *SettingAIServiceServer) SetActiveProfile(ctx context.Context, req *settingv1.SetActiveProfileRequest) (*settingv1.SetActiveProfileResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.SetActiveProfileResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.SetActiveProfileData{Ok: false},
		}, nil
	}
	tenantRef := tenantUUID
	if err := s.svc.SetActiveProfile(ctx, env, &tenantRef,
		strings.TrimSpace(req.GetModality()),
		strings.TrimSpace(req.GetProvider()),
		strings.TrimSpace(req.GetModel()),
	); err != nil {
		return &settingv1.SetActiveProfileResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
			Data: &settingv1.SetActiveProfileData{Ok: false},
		}, nil
	}
	return &settingv1.SetActiveProfileResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.SetActiveProfileData{Ok: true},
	}, nil
}

func (s *SettingAIServiceServer) ListProfiles(ctx context.Context, req *settingv1.ListProfilesRequest) (*settingv1.ListProfilesResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.ListProfilesResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
		}, nil
	}
	tenantRef := tenantUUID
	var mods []string
	for _, m := range req.GetModalities() {
		mods = append(mods, modalityToString(m))
	}
	out, err := s.svc.ListProfiles(ctx, env, &tenantRef, mods...)
	if err != nil {
		return &settingv1.ListProfilesResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
		}, nil
	}
	resp := &settingv1.ListProfilesResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.ListProfilesData{Env: env},
	}
	for i := range out {
		p := out[i]
		resp.Data.Profiles = append(resp.Data.Profiles, toProtoProfile(&p))
	}
	return resp, nil
}

func (s *SettingAIServiceServer) ListCredentials(ctx context.Context, req *settingv1.ListCredentialsRequest) (*settingv1.ListCredentialsResponse, error) {
	env := utils.FirstNonEmpty(strings.TrimSpace(req.GetEnv()), utils.FirstNonEmpty(reqctx.GetEnv(ctx), "default"))
	tenantUUID, err := requireTenantUUID(ctx, req.GetCtx())
	if err != nil {
		return &settingv1.ListCredentialsResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
		}, nil
	}
	tenantRef := tenantUUID
	out, err := s.svc.ListCredentials(ctx, env, &tenantRef)
	if err != nil {
		return &settingv1.ListCredentialsResponse{
			Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId()),
		}, nil
	}
	resp := &settingv1.ListCredentialsResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &settingv1.ListCredentialsData{Env: env},
	}
	for i := range out {
		c := out[i]
		resp.Data.Credentials = append(resp.Data.Credentials, &settingv1.AIProviderCredential{
			Name: c.Name, Provider: c.Provider, AuthScheme: c.AuthScheme, Data: jsonMapToStruct(c.Data),
		})
	}
	return resp, nil
}

/* ************** helpers ************** */
func modalityToString(m settingv1.Modality) string {
	switch m {
	case settingv1.Modality_MODALITY_LLM:
		return "llm"
	case settingv1.Modality_MODALITY_IMAGE:
		return "image"
	case settingv1.Modality_MODALITY_EMBEDDING:
		return "embedding"
	case settingv1.Modality_MODALITY_VIDEO:
		return "video"
	default:
		return "llm"
	}
}

func jsonMapToStruct(m datatypes.JSONMap) *structpb.Struct {
	if m == nil {
		s, _ := structpb.NewStruct(map[string]any{})
		return s
	}
	s, _ := structpb.NewStruct(map[string]any(m))
	return s
}
func toProtoProfile(p *dbmodel.AIModelProfile) *settingv1.AIModelProfile {
	if p == nil {
		return nil
	}
	return &settingv1.AIModelProfile{
		Modality: p.Modality, Provider: p.Provider, Model: p.Model,
		Tags: []string(p.Tags), Defaults: jsonMapToStruct(p.Defaults),
	}
}
func buildCredentialAndProfileFromSaveReq(env string, tenantUUID *string, mod string, req *settingv1.SaveSettingsRequest) (*dbmodel.AIProviderCredential, *dbmodel.AIModelProfile) {
	var provider, model, baseURL, apiKey string
	data := datatypes.JSONMap{}
	defaults := datatypes.JSONMap{}

	switch mod {
	case "llm":
		if req.GetLlm() == nil || req.GetLlm().GetBase() == nil {
			return nil, nil
		}
		b := req.GetLlm().GetBase()
		provider, model, baseURL, apiKey = b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		if v := b.GetRegion(); v != "" {
			data["region"] = v
		}
		if v := b.GetOrganization(); v != "" {
			data["organization"] = v
		}
		if v := b.GetAzureDeployment(); v != "" {
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
		b := req.GetImage().GetBase()
		provider, model, baseURL, apiKey = b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
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
		b := req.GetEmbedding().GetBase()
		provider, model, baseURL, apiKey = b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
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
		b := req.GetVideo().GetBase()
		provider, model, baseURL, apiKey = b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
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

	if baseURL != "" {
		data["base_url"] = baseURL
	}
	if apiKey != "" {
		data["api_key"] = apiKey
	}

	cred := &dbmodel.AIProviderCredential{
		Env: env, TenantUUID: tenantUUID, Name: utils.Slug(env + "-" + provider),
		Provider: provider, AuthScheme: "bearer", Data: data,
	}
	prof := &dbmodel.AIModelProfile{
		Env: env, TenantUUID: tenantUUID, Modality: mod, Provider: provider, Model: model,
		Defaults: defaults, Tags: datatypes.JSONSlice[string]{mod},
	}
	return cred, prof
}
func pickProviderModelConn(mod settingv1.Modality, req *settingv1.TestConnectionRequest) (provider, model, baseURL, apiKey string) {
	switch mod {
	case settingv1.Modality_MODALITY_LLM:
		if b := req.GetLlm().GetBase(); b != nil {
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case settingv1.Modality_MODALITY_IMAGE:
		if b := req.GetImage().GetBase(); b != nil {
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case settingv1.Modality_MODALITY_EMBEDDING:
		if b := req.GetEmbedding().GetBase(); b != nil {
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	case settingv1.Modality_MODALITY_VIDEO:
		if b := req.GetVideo().GetBase(); b != nil {
			return b.GetProvider(), b.GetModel(), b.GetBaseUrl(), b.GetApiKey()
		}
	}
	return "", "", "", ""
}
