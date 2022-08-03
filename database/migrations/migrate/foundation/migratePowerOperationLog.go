package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigratePowerOperationLog struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigratePowerOperationLog() *MigratePowerOperationLog {
	return &MigratePowerOperationLog{
		Migration: &migrate.Migration{
			Model: &database.PowerOperationLog{},
		},
	}
}
