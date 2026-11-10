package agent

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
)

// 读取兼容旧环境的快照；新的同步仍仅写规范环境。
func findOnlineCatalogSetting(ctx context.Context, repo *settingrepo.TenantSettingRepository, tenant, env, provider, app string, lock bool) (*dbsetting.TenantSetting, error) {
	for _, source := range dbmodel.AIEnvironmentReadOrder(env) {
		key := onlineCatalogKey(source, provider, app)
		var item *dbsetting.TenantSetting
		var err error
		if lock {
			item, err = repo.GetByTenantAndKeyForUpdate(ctx, tenant, key)
		} else {
			item, err = repo.GetByTenantAndKey(ctx, tenant, key)
		}
		if err != nil || item != nil {
			return item, err
		}
	}
	return nil, nil
}
