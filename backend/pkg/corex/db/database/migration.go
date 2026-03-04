package database

import (
	migration "github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	modelAgentHub "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	modelAudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelCapability "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	modelCapabilityRegistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	modelDevHotload "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	modelEventFabric "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	modelFlow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelIntegrationGateway "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	modelKnowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	mediaModel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	modelNotification "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	modelPluginCompat "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	modelPluginDebug "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_debug"
	modelPluginGovernance "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	modelPluginRelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	modelPluginSandbox "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_sandbox"
	modelSetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modelTenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	modelWorkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	modelForm "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/model"
	"gorm.io/gorm"
)

// Migrate 执行数据库迁移
func MigrateCoreModels(db *gorm.DB) (err error) {
	if err = migration.EnsureAPIKeyProfileNamingMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMPermissionModuleRenameMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMPermissionAllowAPIKeyMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMPermissionAPIKeyDefaultOpenMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureAPIKeyProfileOwnerMemberMigration(db); err != nil {
		return err
	}

	// 迁移动态表单
	err = db.AutoMigrate(
		&modelForm.FormSchemaRecord{},
		&modelForm.FormSubmission{},
	)
	if err != nil {
		return err
	}

	// 迁移内置 Flow Agent 运行记录
	err = db.AutoMigrate(
		&modelFlow.AgentPlanRun{},
		&modelFlow.AgentTaskEvent{})
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
		&modelIAM.APIKeyProfile{},
		&modelIAM.APIKeyProfilePermission{},
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

	if err = db.AutoMigrate(&mediaModel.MediaAsset{}); err != nil {
		return err
	}
	if err = db.AutoMigrate(&modelNotification.Notification{}); err != nil {
		return err
	}

	if err = migrateCapabilityModels(db); err != nil {
		return err
	}

	if err = migrateCapabilityRegistryModels(db); err != nil {
		return err
	}

	if err = migrateIntegrationGatewayModels(db); err != nil {
		return err
	}

	if err = migrateEventFabricModels(db); err != nil {
		return err
	}

	if err = migrateWorkflowModels(db); err != nil {
		return err
	}

	if err = migratePluginReleaseModels(db); err != nil {
		return err
	}
	if err = migration.EnsurePluginReleaseCandidateUniqueIndex(db); err != nil {
		return err
	}

	if err = migrateDevHotloadModels(db); err != nil {
		return err
	}

	if err = migratePluginGovernanceModels(db); err != nil {
		return err
	}

	if err = migratePluginCompatModels(db); err != nil {
		return err
	}

	if err = migratePluginDebugModels(db); err != nil {
		return err
	}

	if err = migratePluginSandboxModels(db); err != nil {
		return err
	}

	if err = migrateKnowledgeModels(db); err != nil {
		return err
	}

	if err = migrateAgentModelHubModels(db); err != nil {
		return err
	}

	if err = migration.CreatePluginReleaseStatusView(db); err != nil {
		return err
	}

	if err = migration.CreatePluginReleasePartitions(db); err != nil {
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

func migrateCapabilityRegistryModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelCapabilityRegistry.CapabilityRegistration{},
		&modelCapabilityRegistry.AdapterEndpoint{},
		&modelCapabilityRegistry.RoutingPolicy{},
		&modelCapabilityRegistry.FallbackPlan{},
		&modelCapabilityRegistry.HealthProbeResult{},
		&modelCapabilityRegistry.DiscoveryCacheEntry{},
		&modelCapabilityRegistry.CapabilityRecord{},
		&modelCapabilityRegistry.WorkflowTemplateRef{},
		&modelCapabilityRegistry.WorkflowTemplateApproval{},
		&modelCapabilityRegistry.CapabilitySyncJob{},
		&modelCapabilityRegistry.InvocationTrace{},
		&modelCapabilityRegistry.CapabilityEventPublication{},
	)
}

func migrateIntegrationGatewayModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelIntegrationGateway.IntegrationRoute{},
		&modelIntegrationGateway.IntegrationRouteVersion{},
		&modelIntegrationGateway.IntegrationInvocationLog{},
		&modelIntegrationGateway.IntegrationEventPublication{},
		&modelIntegrationGateway.IntegrationGatewayAPIKey{},
		&modelIntegrationGateway.IntegrationGatewayAPIKeyPermission{},
		&modelIntegrationGateway.IntegrationGatewayAPIKeyAuditLog{},
	)
}

func migrateEventFabricModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&modelEventFabric.TopicDefinition{},
		&modelEventFabric.AclBinding{},
		&modelEventFabric.TopicManifestBinding{},
		&modelEventFabric.AclManifestBinding{},
		&modelEventFabric.TaskHistory{},
		&modelEventFabric.ScheduledTask{},
		&modelEventFabric.ScheduledTaskRun{},
		&modelEventFabric.AuthorizationCapability{},
		&modelEventFabric.AuthorizationGrantTemplate{},
		&modelEventFabric.AuthorizationGrant{},
		&modelEventFabric.AuthorizationGrantCapability{},
		&modelEventFabric.AuthorizationGrantCondition{},
		&modelEventFabric.AuthorizationApprovalTicket{},
	); err != nil {
		return err
	}

	if err := migration.CreateEventDeliveryTables(db); err != nil {
		return err
	}
	if err := migration.CreateEventReplayTables(db); err != nil {
		return err
	}
	if err := migration.CreateEventAuthorizationTables(db); err != nil {
		return err
	}
	return migration.EnsureEventTopicsGovernanceMigration(db)
}

func migrateWorkflowModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelWorkflow.WorkflowDefinition{},
		&modelWorkflow.WorkflowInstance{},
		&modelWorkflow.WorkflowStepRecord{},
		&modelWorkflow.WorkflowStepCompensation{},
		&modelWorkflow.AgentAssignment{},
		&modelWorkflow.WorkflowEvent{},
	)
}

func migrateKnowledgeModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelKnowledge.KnowledgeSpace{},
		&modelKnowledge.KnowledgeVectorIndex{},
		&modelKnowledge.PolicyTemplateVersion{},
		&modelKnowledge.IngestionProfileVersion{},
		&modelKnowledge.IndexProfileVersion{},
		&modelKnowledge.RAGProfileVersion{},
		&modelKnowledge.CorpusCheckJob{},
		&modelKnowledge.SourceCredential{},
		&modelKnowledge.SourceConnectorInstance{},
		&modelKnowledge.SpaceSyncJob{},
		&modelKnowledge.IngestionJob{},
		&modelKnowledge.ArtifactBundle{},
		&modelKnowledge.FusionStrategyVersion{},
		&modelKnowledge.FeedbackCase{},
		&modelKnowledge.IAMSyncTask{},
		&modelKnowledge.AuditTrailEntry{},
		&modelKnowledge.DeltaJob{},
		&modelKnowledge.DecayTask{},
		&modelKnowledge.TenantReleasePolicy{},
		&modelKnowledge.TenantReleaseBatch{},
	)
}

func migratePluginReleaseModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelPluginRelease.PluginReleaseCandidate{},
		&modelPluginRelease.ReleasePlan{},
		&modelPluginRelease.CanaryDeploymentRecord{},
		&modelPluginRelease.OfflineDistributionPackage{},
		&modelPluginRelease.MarketplaceListing{},
		&modelPluginRelease.LocalInstallSession{},
		&modelPluginRelease.PluginImportRun{},
	)
}

func migrateDevHotloadModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelDevHotload.DevHotloadSession{},
		&modelDevHotload.DevHotloadSessionEvent{},
	)
}

func migratePluginGovernanceModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelPluginGovernance.VersionGovernanceReport{},
	)
}

func migratePluginCompatModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelPluginCompat.CompatException{},
	)
}

func migratePluginDebugModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelPluginDebug.DiagnosticReport{},
	)
}

func migratePluginSandboxModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelPluginSandbox.SandboxValidationRun{},
	)
}

func migrateAgentModelHubModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelAgentHub.ProviderProfile{},
		&modelAgentHub.RoutingPolicy{},
		&modelAgentHub.ConnectorInstance{},
		&modelAgentHub.CostQuotaLedger{},
	)
}
