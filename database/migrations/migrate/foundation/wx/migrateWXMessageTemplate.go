package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/database/migrations/migrate"
)

type MigrateWXMessageTemplate struct {
	*migrate.Migration
	migrate.MigrationInterface
}

type MigrateWXMessageTemplateTask struct {
	*migrate.Migration
	migrate.MigrationInterface
}

type MigrateWXMessageTemplateSend struct {
	*migrate.Migration
	migrate.MigrationInterface
}

func NewMigrateWXMessageTemplate() *MigrateWXMessageTemplate {
	return &MigrateWXMessageTemplate{
		Migration: &migrate.Migration{
			Model: &models.WXMessageTemplate{},
		},
	}
}

func NewMigrateWXMessageTemplateTask() *MigrateWXMessageTemplateTask {
	return &MigrateWXMessageTemplateTask{
		Migration: &migrate.Migration{
			Model: &models.WXMessageTemplateTask{},
		},
	}
}

func NewMigrateWXMessageTemplateSend() *MigrateWXMessageTemplateSend {
	return &MigrateWXMessageTemplateSend{
		Migration: &migrate.Migration{
			Model: &models.WXMessageTemplateSend{},
		},
	}
}
