package agent

import (
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
)

// ResolveFormBaseURL 只按 Provider 与接入方式解析默认地址，不查询历史配置。
// 表单空值表示恢复默认地址；运行时只有已保存地址为空时才解析默认值。
func ResolveFormBaseURL(provider, authMode, baseURL string) (string, error) {
	if value := strings.TrimSpace(baseURL); value != "" {
		return value, nil
	}
	reg := catalog.GetGlobalAIRegister()
	provider = reg.CanonicalProvider(provider)
	manifest, ok := reg.Manifest(provider)
	if !ok || manifest == nil {
		return "", nil // 未知 Provider 仍由原有型号／协议校验拒绝。
	}
	fields := manifest.Auth.Fields
	modeID := strings.TrimSpace(authMode)
	if len(manifest.Auth.Modes) > 0 {
		if modeID == "" {
			modeID = manifest.Auth.Modes[0].ID
		}
		found := false
		for _, mode := range manifest.Auth.Modes {
			if strings.EqualFold(mode.ID, modeID) {
				found = true
				fields = mode.Fields
				if value := strings.TrimSpace(mode.Defaults["base_url"]); value != "" {
					return value, nil
				}
				break
			}
		}
		if !found {
			return "", connectionConfigError()
		}
	}
	if value := catalog.AuthReqFromCatalog(provider).DefaultBaseURL; value != "" {
		return value, nil
	}
	if value := catalog.DefaultBaseURLForModel(provider, ""); value != "" {
		return value, nil
	}
	for _, field := range fields {
		if strings.EqualFold(strings.TrimSpace(field), "base_url") {
			return "", connectionConfigError() // 自定义服务无默认地址，不能偷偷沿用旧值。
		}
	}
	return "", nil
}

func normalizeCredentialEndpoint(cred *dbmodel.AIProviderCredential) error {
	if cred == nil {
		return connectionConfigError()
	}
	value, provided := cred.Data["base_url"]
	if !provided {
		return nil // 内部仅更新凭据的调用可省略字段，保留原连接。
	}
	base, _ := value.(string)
	mode, _ := cred.Data["auth_mode"].(string)
	_, err := ResolveFormBaseURL(cred.Provider, mode, base)
	if err != nil {
		return err
	}
	// 保留空值作为“使用默认地址”的持久化语义，不固定为当前默认值。
	cred.Data["base_url"] = strings.TrimSpace(base)
	return nil
}
