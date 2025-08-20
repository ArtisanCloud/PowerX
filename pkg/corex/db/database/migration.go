package database

import (
	modelAgent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
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

	// 迁移IAM
	err = db.AutoMigrate(
		&modelIAM.Tenant{},
		&modelIAM.User{},
		&modelIAM.Member{},
		&modelIAM.Credential{},
		&modelIAM.Role{},
		&modelIAM.Permission{},
		&modelIAM.RolePermission{},
		&modelIAM.MemberRole{},
		&modelIAM.RefreshToken{},
		&modelIAM.Department{},
		&modelIAM.MemberDepartment{},
	)
	if err != nil {
		return err
	}

	return nil
}
