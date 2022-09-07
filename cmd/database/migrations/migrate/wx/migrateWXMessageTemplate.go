package wx

import (
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/cmd/database/migrations/migrate/foundation"
)

type MigrateWXMessageTemplate struct {
	*foundation.Migration
	foundation.MigrationInterface
}

type MigrateWXMessageTemplateTask struct {
	*foundation.Migration
	foundation.MigrationInterface
}

type MigrateWXMessageTemplateSend struct {
	*foundation.Migration
	foundation.MigrationInterface
}

func NewMigrateWXMessageTemplate() *MigrateWXMessageTemplate {
	return &MigrateWXMessageTemplate{
		Migration: &foundation.Migration{
			Model: &wx.WXMessageTemplate{},
		},
	}
}

func NewMigrateWXMessageTemplateTask() *MigrateWXMessageTemplateTask {
	return &MigrateWXMessageTemplateTask{
		Migration: &foundation.Migration{
			Model: &wx.WXMessageTemplateTask{},
		},
	}
}

func NewMigrateWXMessageTemplateSend() *MigrateWXMessageTemplateSend {
	return &MigrateWXMessageTemplateSend{
		Migration: &foundation.Migration{
			Model: &wx.WXMessageTemplateSend{},
		},
	}
}
