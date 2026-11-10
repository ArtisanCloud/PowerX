package repository

import (
	"context"
	"errors"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

// 下列方法只供 AI 配置的兼容读取使用。原有精确查询和写入方法继续保留物理环境语义。
func (r *AIProviderCredentialRepository) FindByEquivalentScopeNameProvider(ctx context.Context, env string, tenant *string, name, provider string) (*dbmodel.AIProviderCredential, error) {
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		item, err := r.FindByScopeNameProvider(ctx, source, tenant, dbmodel.AICredentialNameForEnvironment(name, source, utils.Slug(provider)), provider)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return item, err
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// ListByEquivalentScope 合并 AI 环境别名中的凭据，规范环境的同名记录优先。
func (r *AIProviderCredentialRepository) ListByEquivalentScope(ctx context.Context, env string, tenant *string) ([]dbmodel.AIProviderCredential, error) {
	out := []dbmodel.AIProviderCredential{}
	seen := map[string]bool{}
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		items, err := r.ListByScope(ctx, source, tenant)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			identity := item.Provider + "\x00" + dbmodel.AICredentialNameForEnvironment(item.Name, dbmodel.CanonicalAIEnvironment(env), utils.Slug(item.Provider))
			if !seen[identity] {
				seen[identity] = true
				out = append(out, item)
			}
		}
	}
	return out, nil
}

// FindByEquivalentScopeModalityProviderModel 按规范环境优先读取完整画像。
func (r *AIModelProfileRepository) FindByEquivalentScopeModalityProviderModel(ctx context.Context, env string, tenant *string, modality, provider, model string) (*dbmodel.AIModelProfile, error) {
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		item, err := r.FindByScopeModalityProviderModel(ctx, source, tenant, modality, provider, model)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return item, err
		}
	}
	return nil, gorm.ErrRecordNotFound
}

// ListByEquivalentScope 合并不同型号，规范环境的同一型号画像优先且不拼接参数。
func (r *AIModelProfileRepository) ListByEquivalentScope(ctx context.Context, env string, tenant *string, modalities ...string) ([]dbmodel.AIModelProfile, error) {
	out := []dbmodel.AIModelProfile{}
	seen := map[string]bool{}
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		items, err := r.ListByScope(ctx, source, tenant, modalities...)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			identity := item.Modality + "\x00" + item.Provider + "\x00" + item.Model
			if !seen[identity] {
				seen[identity] = true
				out = append(out, item)
			}
		}
	}
	return out, nil
}

// FindEquivalentDefaultByScopeModality 优先使用规范环境的默认路由。
func (r *AIRoutePolicyRepository) FindEquivalentDefaultByScopeModality(ctx context.Context, env string, tenant *string, modality string) (*dbmodel.AIRoutePolicy, error) {
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		item, err := r.FindDefaultByScopeModality(ctx, source, tenant, modality)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return item, err
		}
	}
	return nil, gorm.ErrRecordNotFound
}
