package database

import (
	migration "github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	modelAudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelCapability "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	modelEventFabric "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	modelAgent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	mediamodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	modelSetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modelTenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
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
		&modelTenant.Tenant{},
		&modelTenant.TenantKeyPair{},
		&modelIAM.User{},
		&modelIAM.Member{},
		&modelIAM.Credential{},
		&modelIAM.Role{},
		&modelIAM.Permission{},
		&modelIAM.RolePermission{},
		&modelIAM.RoleBinding{},
		&modelIAM.RefreshToken{},
		&modelIAM.Department{},
		&modelIAM.MemberDepartment{},
		&modelIAM.Department{},
		&modelIAM.DepartmentClosure{},
		&modelIAM.Group{},
		&modelIAM.MemberAssignment{},
		&modelIAM.Position{},
		&modelIAM.Team{},
		&modelIAM.ServiceAccount{},
		&modelIAM.APIKey{},
	)
	if err != nil {
		return err
	}

	err = db.AutoMigrate(
		&modelSetting.SystemSetting{},
		&modelSetting.TenantSetting{},
		&modelSetting.DomainBinding{},
		&modelSetting.TLSCertRef{},
		&modelSetting.AuthProviderConfig{},
		&modelSetting.PluginInstanceConfig{},
	)
	if err != nil {
		return err
	}

	if err = db.AutoMigrate(&mediamodel.MediaAsset{}); err != nil {
		return err
	}

	if err = migrateCapabilityModels(db); err != nil {
		return err
	}

	if err = migrateEventFabricModels(db); err != nil {
		return err
	}

	// 迁移审计
	err = db.AutoMigrate(
		&modelAudit.AuditEvent{},
	)
	if err != nil {
		return err
	}
	return nil
}

func migrateCapabilityModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelCapability.CapabilityContract{},
		&modelCapability.CapabilityIOSchema{},
		&modelCapability.CapabilityVersionPolicy{},
		&modelCapability.CapabilityTransportProfile{},
		&modelCapability.CapabilityErrorTaxonomy{},
		&modelCapability.CapabilityContractErrorTaxonomy{},
	)
}

func migrateEventFabricModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&modelEventFabric.TopicDefinition{},
		&modelEventFabric.AclBinding{},
	); err != nil {
		return err
	}

	if err := migration.CreateEventDeliveryTables(db); err != nil {
		return err
	}
	return migration.CreateEventReplayTables(db)
}
