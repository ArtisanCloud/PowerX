package skills

import (
	"context"
	"encoding/json"
	"fmt"
	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	evidencemodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type dependencyFixture struct {
	db             *gorm.DB
	ctx            context.Context
	tenant, member string
	agent          agentmodel.Agent
	tool           ToolDependencyV2
	checks         *DependencyService
}

func newDependencyFixture(t *testing.T) dependencyFixture {
	t.Helper()
	db := setupDefinitionServiceTestDB(t)
	// SQLite 不支持 REFERENCES schema.table；仓储行为由下方真实读写验证。
	db.DisableForeignKeyConstraintWhenMigrating = true
	require.NoError(t, db.AutoMigrate(&capmodel.CapabilityContract{}, &registrymodel.CapabilityRecord{},
		&registrymodel.CapabilityRegistration{}, &agentmodel.AgentCapabilityGrant{}, &evidencemodel.EvidenceSnapshot{},
		&capmodel.CapabilityIOSchema{}, &capmodel.CapabilityTransportProfile{}, &registrymodel.AdapterEndpoint{}))
	// Agent 的生产 JSONB 默认值带 PostgreSQL cast；本夹具只创建鉴权读取所需列。
	require.NoError(t, db.Exec(`CREATE TABLE agents (
		id INTEGER PRIMARY KEY, uuid TEXT, env TEXT, tenant_uuid TEXT, key TEXT, name TEXT,
		status TEXT, owner_plugin_id TEXT, source TEXT, meta JSON, deleted_at DATETIME
	)`).Error)
	tenant, member := uuid.NewString(), uuid.NewString()
	now := time.Now()
	tool := ToolDependencyV2{DependencyKey: "inventory", ToolKey: "com.example.inventory.turnover", Version: "2.1.0",
		InputSchema:  ToolSchemaRef{ID: "example.inventory.input/v2", SHA256: "sha256:" + strings.Repeat("a", 64)},
		OutputSchema: ToolSchemaRef{ID: "example.inventory.output/v2", SHA256: "sha256:" + strings.Repeat("b", 64)},
		Permission:   ToolPermission{Actor: "admin_user", Code: "com.example.inventory.stock:read"},
		Binding: ToolBinding{Kind: "capability", CapabilityUUID: uuid.NewString(), ContractUUID: uuid.NewString(),
			ProviderVersion: "0.4.0", Protocol: "http", ImplementationSHA256: "sha256:" + strings.Repeat("c", 64)}}
	protocols, err := json.Marshal([]registrymodel.ProtocolBinding{{Channel: "http", Endpoint: "/inventory/turnover", ActorContext: "admin_user", AuthType: "jwt"}})
	require.NoError(t, err)
	record := registrymodel.CapabilityRecord{UUID: uuid.MustParse(tool.Binding.CapabilityUUID), CapabilityID: tool.ToolKey,
		PluginID: "com.example.inventory", PluginVersion: tool.Binding.ProviderVersion, Title: "库存周转",
		Protocols: protocols, Annotations: datatypes.JSON(`{"permission_code":"com.example.inventory.stock:read"}`), Status: "published", PublishedAt: &now}
	require.NoError(t, db.Create(&record).Error)
	contract := capmodel.CapabilityContract{TenantUUID: tenant, CapabilityKey: tool.ToolKey, Version: tool.Version,
		ProviderID: record.PluginID, ContractUUID: uuid.MustParse(tool.Binding.ContractUUID), LifecycleState: "published", Status: 1}
	require.NoError(t, db.Create(&contract).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityTransportProfile{TenantUUID: tenant, ContractID: contract.ID, CapabilityKey: tool.ToolKey, Transport: "http", Mode: "sync", TimeoutMillis: 1000, Status: 1}).Error)
	for direction, schema := range map[string]ToolSchemaRef{"input": tool.InputSchema, "output": tool.OutputSchema} {
		require.NoError(t, db.Create(&capmodel.CapabilityIOSchema{TenantUUID: tenant, ContractID: contract.ID, Direction: direction, Format: "json_schema", SchemaURI: schema.ID, SchemaHash: schema.SHA256, Status: 1}).Error)
	}
	registration := registrymodel.CapabilityRegistration{TenantUUID: tenant, CapabilityID: tool.ToolKey, ContractRef: tool.Version,
		Status: "published", Version: 1, PublishedAt: &now, RoutingPolicyID: uuid.New()}
	require.NoError(t, db.Create(&registration).Error)
	require.NoError(t, db.Create(&registrymodel.AdapterEndpoint{RegistrationID: registration.ID, TenantUUID: tenant, CapabilityID: tool.ToolKey, AdapterID: "inventory-http", TransportType: "http", IsActive: true, Weight: 1, TimeoutMS: 1000}).Error)
	agent := agentmodel.Agent{Env: "dev", TenantUUID: &tenant, Key: "custom_inventory", Name: "库存助手", Status: "active"}
	agent.ID, agent.UUID = 1, uuid.New()
	require.NoError(t, db.Table("agents").Create(map[string]any{"id": agent.ID, "uuid": agent.UUID, "env": "dev", "tenant_uuid": tenant, "key": agent.Key, "status": "active"}).Error)
	require.NoError(t, db.Create(&agentmodel.AgentCapabilityGrant{Env: "dev", TenantUUID: tenant, AgentUUID: agent.UUID,
		CapabilityUUID: record.UUID, CapabilityID: tool.ToolKey, PluginID: record.PluginID, PermissionCode: tool.Permission.Code, Status: "enabled"}).Error)
	ctx := reqctx.WithIsRoot(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), uuid.NewString()), true)
	return dependencyFixture{db: db, ctx: ctx, tenant: tenant, member: member, agent: agent, tool: tool, checks: NewRuntimeDependencyService(db)}
}

func (f dependencyFixture) definition() map[string]any {
	return map[string]any{"schema": SkillDefinitionSchemaV2, "executor": map[string]any{"type": "capability", "capability_id": f.tool.ToolKey, "output_mode": "response_envelope", "response_contract": evidence.ReportSchemaV5},
		"tool_dependencies": ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{f.tool}}}
}

func (f dependencyFixture) input(stage DependencyStage) DependencyCheckInput {
	return DependencyCheckInput{TenantUUID: f.tenant, Env: "dev", AgentID: f.agent.ID, Stage: stage, Definition: f.definition()}
}

func TestDependencyPreflightReadsPublishedCatalogAndLiveGrantsAtAllStages(t *testing.T) {
	for _, stage := range []DependencyStage{DependencyPublish, DependencyBind, DependencyExecute} {
		t.Run(string(stage), func(t *testing.T) {
			f := newDependencyFixture(t)
			// 真实生产装配仍缺 pinning 端口，合法目录和 root 也不能将工具标成可运行。
			require.ErrorContains(t, f.checks.Check(f.ctx, f.input(stage)), "skill.tool_protocol_unavailable")
			pinned := 0
			f.checks.options.PinnedTools = func(_ context.Context, _ DependencyCheckInput, tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) error {
				pinned++
				require.Equal(t, f.tool.Binding.ImplementationSHA256, tool.Binding.ImplementationSHA256)
				require.Equal(t, uint64(1), snapshot.Registration.Version)
				return nil
			}
			require.NoError(t, f.checks.Check(f.ctx, f.input(stage)))
			require.Equal(t, 1, pinned)
			if stage == DependencyPublish {
				// 发布者 root 不能替代绑定/执行所需的目标 Agent grant。
				require.NoError(t, f.db.Model(&agentmodel.AgentCapabilityGrant{}).Where("agent_uuid = ?", f.agent.UUID).Update("status", "disabled").Error)
				require.ErrorContains(t, f.checks.Check(f.ctx, f.input(DependencyBind)), "skill.tool_dependency_denied")
				require.ErrorContains(t, f.checks.Check(f.ctx, f.input(DependencyExecute)), "skill.tool_dependency_denied")
			} else {
				require.NoError(t, f.db.Model(&agentmodel.AgentCapabilityGrant{}).Where("agent_uuid = ?", f.agent.UUID).Update("status", "disabled").Error)
				require.ErrorContains(t, f.checks.Check(f.ctx, f.input(stage)), "skill.tool_dependency_denied")
			}
			require.Equal(t, 1, pinned, "拒绝后不能进入固定版本的执行端口")
		})
	}
}

func TestDependencyPreflightRejectsCatalogDriftAndNewestDisabledRegistration(t *testing.T) {
	for _, test := range []struct {
		name, code string
		mutate     func(*testing.T, *dependencyFixture)
	}{
		{"missing_tool", "skill.tool_dependency_unavailable", func(t *testing.T, f *dependencyFixture) { f.tool.Binding.CapabilityUUID = uuid.NewString() }},
		{"foreign_tenant_contract", "skill.tool_dependency_unavailable", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&capmodel.CapabilityContract{}).Where("contract_uuid = ?", f.tool.Binding.ContractUUID).Update("tenant_uuid", uuid.NewString()).Error)
		}},
		{"version", "skill.tool_dependency_version_or_contract_mismatch", func(t *testing.T, f *dependencyFixture) { f.tool.Version = "2.2.0" }},
		{"provider_version", "skill.tool_dependency_version_or_contract_mismatch", func(t *testing.T, f *dependencyFixture) { f.tool.Binding.ProviderVersion = "0.5.0" }},
		{"schema_digest", "skill.tool_dependency_version_or_contract_mismatch", func(t *testing.T, f *dependencyFixture) {
			f.tool.InputSchema.SHA256 = "sha256:" + strings.Repeat("d", 64)
		}},
		{"wrong_key", "skill.tool_dependency_invalid", func(t *testing.T, f *dependencyFixture) { f.tool.ToolKey = "com.example.other" }},
		{"not_published", "skill.tool_dependency_unavailable", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&capmodel.CapabilityContract{}).Where("contract_uuid = ?", f.tool.Binding.ContractUUID).Update("lifecycle_state", "draft").Error)
		}},
		{"not_effective_yet", "skill.tool_dependency_unavailable", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&capmodel.CapabilityContract{}).Where("contract_uuid = ?", f.tool.Binding.ContractUUID).Update("effective_at", time.Now().Add(time.Hour)).Error)
		}},
		{"agent_unusable", "skill.tool_dependency_denied", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&registrymodel.CapabilityRecord{}).Where("uuid = ?", f.tool.Binding.CapabilityUUID).Update("annotations", datatypes.JSON(`{"permission_code":"com.example.inventory.stock:read","agent_usable":false}`)).Error)
		}},
		{"contract_transport_disabled", "skill.tool_protocol_unavailable", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&capmodel.CapabilityTransportProfile{}).Where("capability_key = ?", f.tool.ToolKey).Update("status", 0).Error)
		}},
		{"undeclared_permission", "skill.tool_dependency_denied", func(t *testing.T, f *dependencyFixture) {
			f.tool.Permission.Code = "com.example.inventory.stock:delete"
		}},
		{"protocol_not_installed", "skill.tool_protocol_unavailable", func(t *testing.T, f *dependencyFixture) { f.tool.Binding.Protocol = "mcp" }},
		{"disabled_adapter", "skill.tool_protocol_unavailable", func(t *testing.T, f *dependencyFixture) {
			require.NoError(t, f.db.Model(&registrymodel.AdapterEndpoint{}).Where("capability_id = ?", f.tool.ToolKey).Update("is_active", false).Error)
		}},
		{"newest_registration_disabled", "skill.tool_dependency_denied", func(t *testing.T, f *dependencyFixture) {
			now := time.Now()
			require.NoError(t, f.db.Create(&registrymodel.CapabilityRegistration{TenantUUID: f.tenant, CapabilityID: f.tool.ToolKey, ContractRef: f.tool.Version, Status: "disabled", Version: 2, PublishedAt: &now, RoutingPolicyID: uuid.New()}).Error)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newDependencyFixture(t)
			test.mutate(t, &f)
			for _, stage := range []DependencyStage{DependencyPublish, DependencyBind, DependencyExecute} {
				err := f.checks.Check(f.ctx, f.input(stage))
				require.ErrorContains(t, err, test.code)
				var detail *DependencyError
				require.ErrorAs(t, err, &detail)
				require.Equal(t, stage, detail.Stage)
			}
		})
	}
}

func TestDependencyPreflightPublisherContextCannotBeForgedByDefinition(t *testing.T) {
	f := newDependencyFixture(t)
	in := f.input(DependencyPublish)
	in.Definition["context"] = map[string]any{"is_root": true, "user_uuid": uuid.NewString(), "member_id": 1}
	require.ErrorContains(t, f.checks.Check(context.Background(), in), "skill.tool_dependency_denied")
	foreign := reqctx.WithTenantUUID(f.ctx, uuid.NewString())
	require.ErrorContains(t, f.checks.Check(foreign, in), "skill.tool_dependency_denied")
}

func TestServiceActorToolCannotBorrowPublisherUserPermissions(t *testing.T) {
	f := newDependencyFixture(t)
	f.tool.Permission.Actor = "service_actor"
	protocols, err := json.Marshal([]registrymodel.ProtocolBinding{{Channel: "http", Endpoint: "/inventory/turnover", ActorContext: "service_actor", AuthType: "api_key"}})
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&registrymodel.CapabilityRecord{}).Where("uuid = ?", f.tool.Binding.CapabilityUUID).Update("protocols", protocols).Error)
	for _, stage := range []DependencyStage{DependencyPublish, DependencyBind, DependencyExecute} {
		require.ErrorContains(t, f.checks.Check(f.ctx, f.input(stage)), "skill.tool_dependency_denied")
	}
}

func TestDependencyDraftPublishAndExecutionUseTheSameChecks(t *testing.T) {
	f := newDependencyFixture(t)
	repo := skillrepo.NewSkillDefinitionRepository(f.db)
	definitions := NewDefinitionService(repo, f.checks)
	source := createAgentAuthoringSource(t, definitions, f.tenant, f.member)
	draft, revision, err := definitions.CreateDraft(f.ctx, CreateDefinitionDraftInput{TenantUUID: f.tenant, SkillID: "customer.inventory_review",
		DisplayNameI18n: map[string]string{"zh-CN": "库存复核"}, DescriptionI18n: map[string]string{"zh-CN": "自建库存复核"},
		SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: f.definition(), AuthorMemberUUID: f.member})
	require.NoError(t, err)
	_, _, err = definitions.PublishCurrentRevision(f.ctx, PublishDefinitionInput{TenantUUID: f.tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://fixture/inventory.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: f.member})
	require.ErrorContains(t, err, "skill.tool_protocol_unavailable")
	current, err := repo.GetCurrentRevision(f.ctx, f.tenant, draft.UUID.String())
	require.NoError(t, err)
	require.Equal(t, skillmodel.SkillDefinitionRevisionStatusDraft, current.Status)
	require.Empty(t, current.PublishedArtifactURI)
	// 模拟之前发布但当前环境缺少端口的版本；禁止仅依赖历史 published 标志运行。
	_, _, err = repo.PublishCurrentRevision(f.ctx, f.tenant, draft.UUID.String(), "local://fixture/previous.tgz", "sha256:fixture", f.member, revision.UUID.String())
	require.NoError(t, err)
	require.ErrorContains(t, definitions.CheckRunnableForAgent(f.ctx, f.tenant, draft.SkillID, "dev", f.agent.ID), "skill.tool_protocol_unavailable")
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: f.checks, Capability: func(context.Context, ManifestCapabilityInvocation) (map[string]any, error) {
		calls++
		return nil, fmt.Errorf("must not call")
	}})
	runtime := NewDefinitionInvokeService(repo, executor, nil)
	_, err = runtime.Execute(f.ctx, InvokeRequest{TenantUUID: f.tenant, SkillID: draft.SkillID, TraceID: "fixture"}, nil, map[string]any{"agent_id": f.agent.ID})
	require.ErrorContains(t, err, "skill.tool_protocol_unavailable")
	require.Zero(t, calls)
}

func TestPublishedDefinitionReauthorizesBeforeEveryInvocation(t *testing.T) {
	f := newDependencyFixture(t)
	// 这里只替换尚未交付的 pinning 端口；目录、租户合同及授权均走真实仓储/服务。
	f.checks.options.PinnedTools = func(_ context.Context, _ DependencyCheckInput, tool ToolDependencyV2, _ *skillrepo.ToolCapabilitySnapshot) error {
		if tool.Binding.ImplementationSHA256 != f.tool.Binding.ImplementationSHA256 {
			return fmt.Errorf("fixture implementation changed")
		}
		return nil
	}
	repo := skillrepo.NewSkillDefinitionRepository(f.db)
	definitions := NewDefinitionService(repo, f.checks)
	source := createAgentAuthoringSource(t, definitions, f.tenant, f.member)
	draft, revision, err := definitions.CreateDraft(f.ctx, CreateDefinitionDraftInput{TenantUUID: f.tenant, SkillID: "customer.renamed_inventory",
		DisplayNameI18n: map[string]string{"zh-CN": "自建库存"}, DescriptionI18n: map[string]string{"zh-CN": "自建库存"},
		SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), AuthorMemberUUID: f.member, Definition: f.definition()})
	require.NoError(t, err)
	_, _, err = definitions.PublishCurrentRevision(f.ctx, PublishDefinitionInput{TenantUUID: f.tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://fixture/renamed.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: f.member})
	require.NoError(t, err)
	require.NoError(t, definitions.CheckRunnableForAgent(f.ctx, f.tenant, draft.SkillID, "dev", f.agent.ID))
	calls := 0
	runtime := NewDefinitionInvokeService(repo, NewManifestExecutor(ManifestExecutorOptions{Dependencies: f.checks, EvidenceStore: evidencesvc.NewService(f.db), Tools: evidence.NewToolService(evidence.ToolServiceOptions{Capability: func(context.Context, evidence.ToolRunIdentity, evidence.ToolDependency) (*evidence.BoundTool, error) {
		calls++
		return nil, fmt.Errorf("fixture.prepare_reached")
	}})}), nil)
	request := InvokeRequest{TenantUUID: f.tenant, SkillID: draft.SkillID, Version: revision.UUID.String(), TraceID: "fixture"}
	_, err = runtime.Execute(f.ctx, request, nil, map[string]any{"agent_id": f.agent.ID})
	require.ErrorContains(t, err, "fixture.prepare_reached")
	require.Equal(t, 1, calls)
	require.NoError(t, f.db.Model(&agentmodel.AgentCapabilityGrant{}).Where("agent_uuid = ?", f.agent.UUID).Update("status", "disabled").Error)
	require.ErrorContains(t, definitions.CheckRunnableForAgent(f.ctx, f.tenant, draft.SkillID, "dev", f.agent.ID), "skill.tool_dependency_denied")
	_, err = runtime.Execute(f.ctx, request, nil, map[string]any{"agent_id": f.agent.ID, "is_root": true, "permission_code": "ignored"})
	require.ErrorContains(t, err, "skill.tool_dependency_denied")
	require.Equal(t, 1, calls, "撤权后不得重复调用模拟工具")
}
