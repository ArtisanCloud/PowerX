package persistence

import (
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"gorm.io/gorm"
	"log"
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
	)

	// 可以顺手确认一下（开发期）：
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_global"); !ok {
		log.Println("warn: ai_cred_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_tenant"); !ok {
		log.Println("warn: ai_cred_uniq_tenant not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_global"); !ok {
		log.Println("warn: ai_model_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_tenant"); !ok {
		log.Println("warn: ai_model_uniq_tenant not created")
	}
	return nil

}
