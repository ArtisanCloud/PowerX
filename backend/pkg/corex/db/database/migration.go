package database

import (
	"fmt"
	"strings"

	migration "github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelAgent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	modelAgentHub "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	modelAudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelCapability "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	modelCapabilityRegistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	modelCustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	modelDevHotload "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	modelEventFabric "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	modelFlow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	modelIAM "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelIntegrationGateway "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	modelKnowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	mediaModel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	modelMetadata "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	modelNotification "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	modelOps "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	modelPluginCompat "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_compat"
	modelPluginDebug "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_debug"
	modelPluginGovernance "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_governance"
	modelPluginRelease "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	modelPluginSandbox "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_sandbox"
	modelRuntimeScheduler "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	modelSetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modelSkills "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	modelTenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	modelWorkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	modelForm "github.com/ArtisanCloud/PowerX/pkg/dynamic_form/persistence/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Migrate 执行数据库迁移
func MigrateCoreModels(db *gorm.DB) (err error) {
	if err = ensurePostgresSchemas(db); err != nil {
		return err
	}

	if err = migration.EnsureAPIKeyProfileNamingMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMPermissionModuleRenameMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMPermissionKeyLengthMigration(db); err != nil {
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
	if err = migration.EnsureSkillsInstallTaskTenantUUIDMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMTenantDomainBackfillMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMMemberUsernameScopeMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureAPIKeyProfileTenantScopedKeyMigration(db); err != nil {
		return err
	}
	if err = migration.EnsureIAMUserLastTenantUUIDMigration(db); err != nil {
		return err
	}
	if err = migration.EnsurePluginDebugHostPermissionPathMigration(db); err != nil {
		return err
	}

	if err = migrateCustomerModels(db); err != nil {
		return err
	}
	if err = migrateMetadataModels(db); err != nil {
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
		&modelIAM.RootSupportSession{},
	)
	if err != nil {
		return err
	}
	if err = backfillIAMRoleUUID(db); err != nil {
		return err
	}
	if err = backfillIAMRelationshipUUIDs(db); err != nil {
		return err
	}
	if err = ensureIAMUserPhoneIndex(db); err != nil {
		return err
	}
	if err = ensureIAMRoleBindingTenantMemberIndex(db); err != nil {
		return err
	}

	err = db.AutoMigrate(
		&modelSetting.SystemSetting{},
		&modelSetting.TenantSetting{},
		&modelSetting.DomainBinding{},
		&modelSetting.TLSCertRef{},
		&modelSetting.AuthProviderConfig{},
		&modelSetting.PluginInstanceConfig{},
		&modelSetting.PluginDrainJob{},
	)
	if err != nil {
		return err
	}
	if err = normalizePluginInstanceStatusAfterMigrate(db); err != nil {
		return err
	}

	if err = db.AutoMigrate(&mediaModel.MediaAsset{}, &mediaModel.MediaAssetVariant{}); err != nil {
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
	if err = migration.EnsureCapabilityRecordUUIDMigration(db); err != nil {
		return err
	}

	if err = migrateIntegrationGatewayModels(db); err != nil {
		return err
	}

	if err = migrateEventFabricModels(db); err != nil {
		return err
	}

	if err = migrateRuntimeSchedulerModels(db); err != nil {
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

	if err = migrateSkillsModels(db); err != nil {
		return err
	}

	if err = migrateOpsModels(db); err != nil {
		return err
	}

	if err = migrateAgentModelHubModels(db); err != nil {
		return err
	}
	if err = migrateAgentA2AModels(db); err != nil {
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

func migrateMetadataModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelMetadata.DictionaryNamespace{},
		&modelMetadata.DictionaryItem{},
		&modelMetadata.Taxonomy{},
		&modelMetadata.TaxonomyNode{},
		&modelMetadata.Tag{},
		&modelMetadata.TagBinding{},
		&modelMetadata.ResourceType{},
		&modelMetadata.Reference{},
	)
}

func backfillIAMRoleUUID(db *gorm.DB) error {
	if db == nil || db.Dialector == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(db.Dialector.Name()), "postgres") {
		return nil
	}
	type roleRow struct {
		ID uint64 `gorm:"column:id"`
	}
	var rows []roleRow
	if err := db.Table((&modelIAM.Role{}).GetTableName(true)).
		Select("id").
		Where("uuid IS NULL OR uuid = ?", "00000000-0000-0000-0000-000000000000").
		Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == 0 {
			continue
		}
		if err := db.Table((&modelIAM.Role{}).GetTableName(true)).
			Where("id = ?", row.ID).
			Update("uuid", uuid.New()).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillIAMRelationshipUUIDs(db *gorm.DB) error {
	if db == nil || db.Dialector == nil {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(db.Dialector.Name()), "postgres") {
		return nil
	}

	userTable := (&modelIAM.User{}).GetTableName(true)
	memberTable := (&modelIAM.Member{}).GetTableName(true)
	roleTable := (&modelIAM.Role{}).GetTableName(true)
	roleBindingTable := (&modelIAM.RoleBinding{}).GetTableName(true)

	if err := db.Exec(fmt.Sprintf(`
		UPDATE %s AS m
		SET user_uuid = u.uuid::text
		FROM %s AS u
		WHERE m.user_id = u.id
		  AND (m.user_uuid IS NULL OR m.user_uuid = '' OR m.user_uuid = '00000000-0000-0000-0000-000000000000')
	`, memberTable, userTable)).Error; err != nil {
		return err
	}

	if err := db.Exec(fmt.Sprintf(`
		UPDATE %s AS rb
		SET role_uuid = r.uuid::text
		FROM %s AS r
		WHERE rb.role_id = r.id
		  AND (rb.role_uuid IS NULL OR rb.role_uuid = '' OR rb.role_uuid = '00000000-0000-0000-0000-000000000000')
	`, roleBindingTable, roleTable)).Error; err != nil {
		return err
	}

	if err := db.Exec(fmt.Sprintf(`
		UPDATE %s AS rb
		SET subject_uuid = m.uuid::text
		FROM %s AS m
		WHERE rb.subject_type = ?
		  AND rb.subject_id = m.id
		  AND rb.tenant_uuid = m.tenant_uuid
		  AND (rb.subject_uuid IS NULL OR rb.subject_uuid = '' OR rb.subject_uuid = '00000000-0000-0000-0000-000000000000')
	`, roleBindingTable, memberTable), modelIAM.SubMember).Error; err != nil {
		return err
	}

	var missingMemberUserUUID int64
	if err := db.Raw(fmt.Sprintf(`
		SELECT COUNT(1)
		FROM %s
		WHERE user_id > 0
		  AND (user_uuid IS NULL OR user_uuid = '' OR user_uuid = '00000000-0000-0000-0000-000000000000')
	`, memberTable)).Scan(&missingMemberUserUUID).Error; err != nil {
		return err
	}
	if missingMemberUserUUID > 0 {
		return fmt.Errorf("iam_member has %d row(s) without resolvable user_uuid", missingMemberUserUUID)
	}

	var missingRoleBindingRoleUUID int64
	if err := db.Raw(fmt.Sprintf(`
		SELECT COUNT(1)
		FROM %s
		WHERE role_id > 0
		  AND (role_uuid IS NULL OR role_uuid = '' OR role_uuid = '00000000-0000-0000-0000-000000000000')
	`, roleBindingTable)).Scan(&missingRoleBindingRoleUUID).Error; err != nil {
		return err
	}
	if missingRoleBindingRoleUUID > 0 {
		return fmt.Errorf("iam_role_binding has %d row(s) without resolvable role_uuid", missingRoleBindingRoleUUID)
	}

	var missingRoleBindingSubjectUUID int64
	if err := db.Raw(fmt.Sprintf(`
		SELECT COUNT(1)
		FROM %s
		WHERE subject_type = ?
		  AND subject_id > 0
		  AND (subject_uuid IS NULL OR subject_uuid = '' OR subject_uuid = '00000000-0000-0000-0000-000000000000')
	`, roleBindingTable), modelIAM.SubMember).Scan(&missingRoleBindingSubjectUUID).Error; err != nil {
		return err
	}
	if missingRoleBindingSubjectUUID > 0 {
		return fmt.Errorf("iam_role_binding has %d member binding row(s) without resolvable subject_uuid", missingRoleBindingSubjectUUID)
	}

	return nil
}

func ensurePostgresSchemas(db *gorm.DB) error {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "postgres" {
		return nil
	}

	// 历史模型和 SQL 同时依赖 public，且部分模型支持可配置 schema。
	for _, schemaName := range []string{"public", strings.TrimSpace(coremodel.PowerXSchema)} {
		schemaName = strings.TrimSpace(schemaName)
		if schemaName == "" {
			continue
		}
		if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quotePostgresIdentifier(schemaName))).Error; err != nil {
			return err
		}
	}
	return nil
}

func quotePostgresIdentifier(name string) string {
	escaped := strings.ReplaceAll(name, `"`, `""`)
	return `"` + escaped + `"`
}

func normalizePluginInstanceStatusAfterMigrate(db *gorm.DB) error {
	table := (&modelSetting.PluginInstanceConfig{}).TableName()
	if !db.Migrator().HasTable(&modelSetting.PluginInstanceConfig{}) {
		return nil
	}
	if !db.Migrator().HasColumn(&modelSetting.PluginInstanceConfig{}, "status") {
		return nil
	}
	if err := db.Exec(
		fmt.Sprintf("UPDATE %s SET status = ? WHERE enabled = ? AND (status IS NULL OR status = '' OR status = ?)", table),
		modelSetting.PluginInstanceStatusDisabled,
		false,
		modelSetting.PluginInstanceStatusEnabled,
	).Error; err != nil {
		return err
	}
	return db.Exec(
		fmt.Sprintf("UPDATE %s SET status = ? WHERE enabled = ? AND (status IS NULL OR status = '')", table),
		modelSetting.PluginInstanceStatusEnabled,
		true,
	).Error
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

func migrateCustomerModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelCustomer.Account{},
		&modelCustomer.AuthIdentity{},
		&modelCustomer.TenantMembership{},
		&modelCustomer.MiniAppEntry{},
		&modelCustomer.Session{},
		&modelCustomer.LoginEvent{},
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

func migrateAgentA2AModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelAgent.AgentTeam{},
		&modelAgent.AgentTeamMember{},
		&modelAgent.AgentHandoffTask{},
		&modelAgent.AgentSharedContextRef{},
	)
}

func migrateOpsModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelOps.DeployReleaseRecord{},
		&modelOps.PluginLifecycleAudit{},
		&modelOps.BackupPolicy{},
		&modelOps.BackupJob{},
		&modelOps.BackupArtifact{},
		&modelOps.BackupAlert{},
		&modelOps.RestoreDrillRecord{},
		&modelOps.ApprovalPolicyProfile{},
		&modelOps.MigrationRunbookRecord{},
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

func migrateRuntimeSchedulerModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelRuntimeScheduler.SchedulerJob{},
		&modelRuntimeScheduler.SchedulerJobRun{},
	)
}

func migrateWorkflowModels(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&modelWorkflow.WorkflowDefinition{},
		&modelWorkflow.WorkflowInstance{},
		&modelWorkflow.WorkflowStepRecord{},
		&modelWorkflow.WorkflowStepCompensation{},
		&modelWorkflow.AgentAssignment{},
		&modelWorkflow.WorkflowEvent{},
		&modelWorkflow.HumanReviewTask{},
		&modelWorkflow.WorkflowPackSeedRecord{},
		&modelWorkflow.WorkflowPackInstallation{},
	); err != nil {
		return err
	}
	if err := migration.EnsureWorkflowDefinitionPackColumnsMigration(db); err != nil {
		return err
	}
	return ensureWorkflowPackInstallationBackfill(db)
}

func ensureWorkflowPackInstallationBackfill(db *gorm.DB) error {
	var records []modelWorkflow.WorkflowPackSeedRecord
	if err := db.
		Order("tenant_uuid ASC, workflow_key ASC, version DESC, id DESC").
		Find(&records).Error; err != nil {
		return fmt.Errorf("list workflow pack seed records for installation backfill failed: %w", err)
	}
	latestByKey := make(map[string]modelWorkflow.WorkflowPackSeedRecord, len(records))
	for _, record := range records {
		tenantUUID := strings.ToLower(strings.TrimSpace(record.TenantUUID))
		if tenantUUID == "" {
			return fmt.Errorf("workflow_pack_seed_records contains tenantless record workflow_key=%s uuid=%s; remove or migrate it before enabling workflow pack installation state", record.WorkflowKey, record.UUID)
		}
		workflowKey := strings.TrimSpace(record.WorkflowKey)
		if workflowKey == "" {
			return fmt.Errorf("workflow_pack_seed_records contains empty workflow_key uuid=%s", record.UUID)
		}
		key := tenantUUID + "\x00" + workflowKey
		if _, ok := latestByKey[key]; ok {
			continue
		}
		latestByKey[key] = record
	}
	for _, record := range latestByKey {
		seededAt := record.SeededAt
		installation := modelWorkflow.WorkflowPackInstallation{
			TenantUUID:        strings.ToLower(strings.TrimSpace(record.TenantUUID)),
			WorkflowKey:       strings.TrimSpace(record.WorkflowKey),
			Version:           record.Version,
			Checksum:          record.Checksum,
			Status:            modelWorkflow.WorkflowPackInstallationStatusEnabled,
			DefinitionUUID:    record.DefinitionUUID,
			DefinitionVersion: record.DefinitionVersion,
			Source:            record.Source,
			InstalledAt:       &seededAt,
			LastSeededAt:      &seededAt,
			LastAction:        "backfill",
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_uuid"},
				{Name: "workflow_key"},
			},
			DoNothing: true,
		}).Create(&installation).Error; err != nil {
			return fmt.Errorf("backfill workflow pack installation tenant_uuid=%s workflow_key=%s failed: %w", installation.TenantUUID, installation.WorkflowKey, err)
		}
	}
	return nil
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

func ensureIAMUserPhoneIndex(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	table := (&modelIAM.User{}).TableName()
	if err := db.Exec(`DROP INDEX IF EXISTS uk_user_phone`).Error; err != nil {
		return fmt.Errorf("drop legacy iam user phone index: %w", err)
	}
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		if parts := strings.Split(strings.TrimSpace(table), "."); len(parts) == 2 {
			schema := strings.Trim(parts[0], `"`)
			if schema != "" {
				stmt := fmt.Sprintf(`DROP INDEX IF EXISTS "%s".uk_user_phone`, schema)
				if err := db.Exec(stmt).Error; err != nil {
					return fmt.Errorf("drop schema legacy iam user phone index: %w", err)
				}
			}
		}
		stmt := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_user_phone_nonempty ON %s (phone) WHERE phone IS NOT NULL AND btrim(phone) <> ''`, table)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("create iam user phone nonempty index: %w", err)
		}
		return nil
	}
	sqliteTable := table
	if parts := strings.Split(strings.TrimSpace(table), "."); len(parts) == 2 {
		sqliteTable = parts[1]
	}
	stmt := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_user_phone_nonempty ON %s (phone) WHERE phone IS NOT NULL AND trim(phone) <> ''`, sqliteTable)
	if err := db.Exec(stmt).Error; err != nil {
		return fmt.Errorf("create iam user phone nonempty index: %w", err)
	}
	return nil
}

func ensureIAMRoleBindingTenantMemberIndex(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	table := (&modelIAM.RoleBinding{}).TableName()
	indexName := "idx_iam_role_binding_member_tenant_role"
	if db.Dialector != nil && db.Dialector.Name() == "postgres" {
		stmt := fmt.Sprintf(
			`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (tenant_uuid, subject_type, subject_uuid, role_uuid, data_scope) WHERE subject_type = 'MEMBER' AND data_scope = 'TENANT' AND role_uuid IS NOT NULL AND btrim(role_uuid) <> '' AND subject_uuid IS NOT NULL AND btrim(subject_uuid) <> ''`,
			indexName,
			table,
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("create iam role binding tenant member index: %w", err)
		}
		return nil
	}
	sqliteTable := table
	if parts := strings.Split(strings.TrimSpace(table), "."); len(parts) == 2 {
		sqliteTable = parts[1]
	}
	stmt := fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (tenant_uuid, subject_type, subject_uuid, role_uuid, data_scope) WHERE subject_type = 'MEMBER' AND data_scope = 'TENANT' AND role_uuid IS NOT NULL AND trim(role_uuid) <> '' AND subject_uuid IS NOT NULL AND trim(subject_uuid) <> ''`,
		indexName,
		sqliteTable,
	)
	if err := db.Exec(stmt).Error; err != nil {
		return fmt.Errorf("create iam role binding tenant member index: %w", err)
	}
	return nil
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

func migrateSkillsModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&modelSkills.SkillRegistryRecord{},
		&modelSkills.OfficialSkillCatalogEntry{},
		&modelSkills.SkillCapabilityBinding{},
		&modelSkills.SkillExecutionTrace{},
		&modelSkills.SkillLifecycleAudit{},
		&modelSkills.SkillInstallTask{},
	)
}
