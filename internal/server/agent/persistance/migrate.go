package persistance

import (
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistance/model"
	"gorm.io/gorm"
)

func MigrateAgentModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&dbmodel.AIProviderCredential{},
		&dbmodel.AIModelProfile{},
		&dbmodel.AIRoutePolicy{},
		&dbmodel.AIUsageLog{},
	)
}
