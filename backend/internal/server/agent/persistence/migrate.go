package persistence

// internal/server/agent/persistence/migrate.go

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

func MigrateAgentModels(db *gorm.DB) error {

	db.AutoMigrate(
		&dbmodel.AIProviderCredential{},
		&dbmodel.AIModelProfile{},
		&dbmodel.AIRoutePolicy{},
		&dbmodel.AIUsageLog{},

		&dbmodel.Agent{},
		&dbmodel.AgentSetting{},
		&dbmodel.AgentKBBinding{},
		&dbmodel.AgentPluginLink{},

		&dbmodel.AgentChatSession{},
		&dbmodel.AgentChatMessage{},
		&dbmodel.AgentRuntimeConfig{},

		&dbmodel.AgentProfileLifecycle{},
		&dbmodel.AgentLifecycleEventRecord{},
		&dbmodel.AgentHealthSnapshotRecord{},
		&dbmodel.AgentShareRecord{},
		&dbmodel.AgentTenantForm{},
	)

	// 可以顺手确认一下（开发期）：
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_global"); !ok {
		logger.WarnF(context.Background(), "warn: ai_cred_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_tenant"); !ok {
		logger.WarnF(context.Background(), "warn: ai_cred_uniq_tenant not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_global"); !ok {
		logger.WarnF(context.Background(), "warn: ai_model_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_tenant"); !ok {
		logger.WarnF(context.Background(), "warn: ai_model_uniq_tenant not created")
	}
	return nil

}
