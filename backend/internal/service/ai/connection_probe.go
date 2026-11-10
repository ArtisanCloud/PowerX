package ai

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/connectionprobe"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const (
	ConnectionTestCapability = "com.corex.ai.connection.test"
	ConnectionTestEndpoint   = "core://ai/connection"
)

// ConnectionTestOperation 不接收供应商凭据、地址、prompt 或生成参数。
type ConnectionTestOperation struct {
	ContractVersion int    `json:"contract_version"`
	Operation       string `json:"operation"`
	Modality        string `json:"modality"`
	Provider        string `json:"provider"`
	Model           string `json:"model,omitempty"`
}

type ConnectionTestResult struct {
	ContractVersion int                     `json:"contract_version"`
	OK              bool                    `json:"ok"`
	Env             string                  `json:"env"`
	Modality        string                  `json:"modality"`
	Verification    *connectionprobe.Result `json:"verification"`
}

func (s *Service) testSavedConnection(ctx context.Context, body any) (map[string]any, error) {
	var request ConnectionTestOperation
	if strictMediaBody(body, &request) != nil || request.ContractVersion != 1 || request.Operation != "test_connection" || strings.TrimSpace(request.Provider) == "" || request.Provider != strings.TrimSpace(request.Provider) {
		return nil, connectionTestError(400, "AI_OPERATION_INVALID")
	}
	switch contract.Modality(request.Modality) {
	case contract.ModLLM, contract.ModVLM, contract.ModImage, contract.ModVideo, contract.ModEmbed, contract.ModAudioTTS, contract.ModAudioASR, contract.ModModel3D, contract.ModRerank:
	default:
		return nil, connectionTestError(400, "AI_OPERATION_INVALID")
	}
	provider := catalog.GetGlobalAIRegister().CanonicalProvider(request.Provider)
	if _, ok := catalog.GetGlobalAIRegister().Manifest(provider); !ok {
		return nil, connectionTestError(422, "AI_SETTINGS_PROVIDER_INVALID")
	}
	tenant := reqctx.GetTenantUUID(ctx)
	env, configured, err := s.ResolveTenantEnv(ctx, tenant)
	if err != nil {
		return nil, connectionTestError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	if !configured {
		return nil, connectionTestError(409, "AI_ENV_NOT_CONFIGURED")
	}
	verification, err := s.settings.VerifySavedConnection(ctx, env, tenant, contract.Modality(request.Modality), provider, request.Model)
	if err != nil {
		return nil, err
	}
	return map[string]any{"result": ConnectionTestResult{ContractVersion: 1, OK: true, Env: env, Modality: request.Modality, Verification: verification}}, nil
}

func connectionTestError(status int, code string) *dto.AppError {
	return &dto.AppError{HTTPCode: status, Code: code, Message: code, Details: map[string]any{"generation_submitted": false}}
}
