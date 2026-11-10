package monitorlogs

import (
	"context"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"gorm.io/gorm"
)

// DBRetentionProvider 保留旧配置兼容入口，但永久禁止通用按时间 DELETE。
// 请通过 Root 存储治理页面使用有终态、引用及迁移检查的固定类型适配器。
type DBRetentionProvider struct{ configured bool }

func NewDBRetentionProvider(_ *gorm.DB, _ int, _ int, tables []logcfg.RetentionDBTable) *DBRetentionProvider {
	return &DBRetentionProvider{configured: len(tables) > 0}
}
func (p *DBRetentionProvider) Cleanup(context.Context, int) (int64, []string) {
	if p == nil || !p.configured {
		return 0, nil
	}
	return 0, []string{"legacy_database_retention_disabled_use_typed_storage_policies"}
}
func (p *DBRetentionProvider) Preview(ctx context.Context, days int) (int64, []string) {
	n, _, errs := p.PreviewBreakdown(ctx, days)
	return n, errs
}
func (p *DBRetentionProvider) PreviewBreakdown(context.Context, int) (int64, map[string]int64, []string) {
	if p == nil || !p.configured {
		return 0, map[string]int64{}, nil
	}
	return 0, map[string]int64{}, []string{"legacy_database_retention_disabled_use_typed_storage_policies"}
}
