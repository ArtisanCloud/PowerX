package foundation

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
)

type MigratePowerOperationLog struct {
	*Migration
	MigrationInterface
}

func NewMigratePowerOperationLog() *MigratePowerOperationLog {
	return &MigratePowerOperationLog{
		Migration: &Migration{
			Model: &database.PowerOperationLog{},
		},
	}
}
