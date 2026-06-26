package persistence

// internal/server/agent/persistence/migrate.go

import (
	"context"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
	"strings"
)

func MigrateAgentModels(db *gorm.DB) error {

	if err := db.AutoMigrate(
		&dbmodel.AIProviderCredential{},
		&dbmodel.AIModelProfile{},
		&dbmodel.AIRoutePolicy{},
		&dbmodel.AIUsageLog{},

		&dbmodel.Agent{},
		&dbmodel.AgentSetting{},
		&dbmodel.AgentKBBinding{},
		&dbmodel.AgentSkillBinding{},
		&dbmodel.AgentKnowledgeBinding{},
		&dbmodel.AgentPluginLink{},

		&dbmodel.AgentChatSession{},
		&dbmodel.AgentChatMessage{},
		&dbmodel.AgentChatContextSummary{},
		&dbmodel.AgentSessionSkillState{},
		&dbmodel.AgentRuntimeConfig{},

		&dbmodel.AgentProfileLifecycle{},
		&dbmodel.AgentLifecycleEventRecord{},
		&dbmodel.AgentHealthSnapshotRecord{},
		&dbmodel.AgentShareRecord{},
		&dbmodel.AgentTenantForm{},
	); err != nil {
		return err
	}

	// 可以顺手确认一下（开发期）：
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_global"); !ok {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.persistence.migrate"}), "warn: ai_cred_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIProviderCredential{}, "ai_cred_uniq_tenant"); !ok {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.persistence.migrate"}), "warn: ai_cred_uniq_tenant not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_global"); !ok {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.persistence.migrate"}), "warn: ai_model_uniq_global not created")
	}
	if ok := db.Migrator().HasIndex(&dbmodel.AIModelProfile{}, "ai_model_uniq_tenant"); !ok {
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.persistence.migrate"}), "warn: ai_model_uniq_tenant not created")
	}
	if err := backfillAgentStructuredFieldsFromMeta(db); err != nil {
		return err
	}
	return nil

}

func backfillAgentStructuredFieldsFromMeta(db *gorm.DB) error {
	if db == nil || db.Dialector == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(db.Dialector.Name()), "postgres") {
		return nil
	}
	sql := `
UPDATE public.agents
SET
  type_id = COALESCE(NULLIF(type_id, ''), NULLIF(meta->>'type_id', ''), NULLIF(meta->>'typeId', '')),
  scene = COALESCE(NULLIF(scene, ''), NULLIF(meta->>'scene', '')),
  prompt_seed = COALESCE(NULLIF(prompt_seed, ''), NULLIF(meta->>'prompt_seed', ''), NULLIF(meta->>'promptSeed', '')),
  persona = COALESCE(NULLIF(persona, ''), NULLIF(meta->>'persona', ''), NULLIF(meta#>>'{parameters,persona}', ''))
WHERE
  COALESCE(type_id, '') = '' OR
  COALESCE(scene, '') = '' OR
  COALESCE(prompt_seed, '') = '' OR
  COALESCE(persona, '') = ''
`
	return db.Exec(sql).Error
}
