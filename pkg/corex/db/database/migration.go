package database

import (
	modelAgent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	modelForm "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/model"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移
func MigrateCoreModels(db *gorm.DB) (err error) {
	// 迁移动态表单
	err = db.AutoMigrate(
		&modelForm.FormSchemaRecord{},
		&modelForm.FormSubmission{},
	)
	if err != nil {
		return err
	}

	// 迁移Agent
	err = db.AutoMigrate(
		&modelAgent.AgentPlanRun{},
		&modelAgent.AgentTaskEvent{})
	if err != nil {
		return err
	}

	return nil
}
