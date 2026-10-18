package skills

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDependencyPreflightPreservesLegacyBuiltinAndRejectsUnpinnedExternalTools(t *testing.T) {
	service := NewRuntimeDependencyService(nil)
	for _, stage := range []DependencyStage{DependencyPublish, DependencyBind, DependencyExecute} {
		in := DependencyCheckInput{TenantUUID: uuid.NewString(), Stage: stage, Definition: evidenceDefinition()}
		require.NoError(t, service.Check(context.Background(), in))
		in.Definition = map[string]any{"executor": map[string]any{"type": "capability", "capability_id": "customer.example"}}
		require.ErrorContains(t, service.Check(context.Background(), in), "skill.tool_dependencies_required")
		in.Definition["tool_dependencies"] = ToolRequirements{Schema: ToolDependencySchema, Tools: []ToolDependency{{ToolKey: "customer.example", Version: "1.0.0", InputSchema: "input/v1", OutputSchema: "output/v1", Permission: "example:read"}}}
		require.ErrorContains(t, service.Check(context.Background(), in), "skill.tool_contract_upgrade_required")
	}
}

func TestDependencyPreflightRejectsInvalidV2DeclarationsBeforeAnyPort(t *testing.T) {
	base, err := json.Marshal(ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{typedCalculatorDependency()}})
	require.NoError(t, err)
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"unknown_field", func(m map[string]any) { m["allow_admin"] = true }},
		{"missing_permission", func(m map[string]any) { delete(m["tools"].([]any)[0].(map[string]any), "permission") }},
		{"latest", func(m map[string]any) { m["tools"].([]any)[0].(map[string]any)["version"] = "latest" }},
		{"range", func(m map[string]any) { m["tools"].([]any)[0].(map[string]any)["version"] = "^2.0.0" }},
		{"unknown_builtin_binding_field", func(m map[string]any) {
			m["tools"].([]any)[0].(map[string]any)["binding"].(map[string]any)["capability_uuid"] = nil
		}},
		{"empty_tools", func(m map[string]any) { m["tools"] = []any{} }},
		{"duplicate_tool", func(m map[string]any) { m["tools"] = append(m["tools"].([]any), m["tools"].([]any)[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requirement map[string]any
			require.NoError(t, json.Unmarshal(base, &requirement))
			test.change(requirement)
			called := false
			checks := NewDependencyService(DependencyServiceOptions{Executors: map[string]bool{"llm_prompt": true}, PinnedTools: func(context.Context, DependencyCheckInput, ToolDependencyV2, *skillrepo.ToolCapabilitySnapshot) error {
				called = true
				return nil
			}})
			definition := llmPromptDefinition("test")
			definition["tool_dependencies"] = requirement
			require.ErrorContains(t, checks.Check(context.Background(), DependencyCheckInput{TenantUUID: uuid.NewString(), Stage: DependencyPublish, Definition: definition}), "skill.tool_dependency_invalid")
			require.False(t, called)
		})
	}
}

func TestDependencyPreflightBlocksUnavailableExecutorsBeforePublishing(t *testing.T) {
	service := NewDependencyService(DependencyServiceOptions{Executors: map[string]bool{"llm_prompt": true}})
	for _, executor := range []string{"workflow", "script", "capability"} {
		t.Run(executor, func(t *testing.T) {
			err := service.Check(context.Background(), DependencyCheckInput{
				TenantUUID: uuid.NewString(), Stage: DependencyPublish,
				Definition: map[string]any{"executor": map[string]any{"type": executor}},
			})
			require.ErrorContains(t, err, "skill.executor_unavailable")
		})
	}
}

func TestUnsupportedExecutorsRemainDraftAndCannotPublish(t *testing.T) {
	for _, kind := range []string{"workflow", "script", "instruction_only"} {
		t.Run(kind, func(t *testing.T) {
			db := setupDefinitionServiceTestDB(t)
			svc := NewDefinitionService(skillrepo.NewSkillDefinitionRepository(db), NewRuntimeDependencyService(db))
			tenant, member := uuid.NewString(), uuid.NewString()
			source := createAgentAuthoringSource(t, svc, tenant, member)
			executor := map[string]any{"type": kind}
			if kind == "workflow" {
				executor["workflow_uuid"] = uuid.NewString()
			}
			draft, revision, err := svc.CreateDraft(context.Background(), CreateDefinitionDraftInput{
				TenantUUID: tenant, SkillID: "customer.unsupported", DisplayNameI18n: map[string]string{"zh-CN": "待接入"},
				DescriptionI18n: map[string]string{"zh-CN": "待接入"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
				PackageSourceUUID: source.UUID.String(), AuthorMemberUUID: member,
				Definition: map[string]any{"schema": SkillDefinitionSchemaV2, "executor": executor},
			})
			require.NoError(t, err)
			require.Equal(t, skillmodel.SkillDefinitionRevisionStatusDraft, revision.Status)
			_, _, err = svc.PublishCurrentRevision(context.Background(), PublishDefinitionInput{TenantUUID: tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://fixture/unsupported.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: member})
			require.Error(t, err)
			require.Error(t, svc.CheckRunnable(context.Background(), tenant, draft.SkillID))
			current, err := svc.repo.GetCurrentRevision(context.Background(), tenant, draft.UUID.String())
			require.NoError(t, err)
			require.Equal(t, skillmodel.SkillDefinitionRevisionStatusDraft, current.Status)
		})
	}
}

func typedCalculatorDependency() ToolDependencyV2 {
	return ToolDependencyV2{DependencyKey: "calculator", ToolKey: "powerx.calculation.evaluate", Version: "2.0.0",
		InputSchema:  ToolSchemaRef{ID: "powerx.calculation.request/v1", SHA256: "sha256:" + strings.Repeat("a", 64)},
		OutputSchema: ToolSchemaRef{ID: "powerx.calculation.result/v1", SHA256: "sha256:" + strings.Repeat("b", 64)},
		Permission:   ToolPermission{Actor: "runtime_internal", Code: "pure_calculation"},
		Binding:      ToolBinding{Kind: "builtin", ImplementationSHA256: "sha256:" + strings.Repeat("c", 64)}}
}

func TestTypedCalculatorPinsImplementationAndWaitsForResponseConsumer(t *testing.T) {
	definition := llmPromptDefinition("test")
	definition["tool_dependencies"] = ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{typedCalculatorDependency()}}
	for _, stage := range []DependencyStage{DependencyPublish, DependencyBind, DependencyExecute} {
		require.ErrorContains(t, NewRuntimeDependencyService(nil).Check(context.Background(), DependencyCheckInput{TenantUUID: uuid.NewString(), Stage: stage, Definition: definition}), "skill.tool_dependency_version_or_contract_mismatch")
		actual, err := NewRuntimeToolService().CalculatorDependency("calculator")
		require.NoError(t, err)
		definition["tool_dependencies"] = ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{actual}}
		require.ErrorContains(t, NewRuntimeDependencyService(nil).Check(context.Background(), DependencyCheckInput{TenantUUID: uuid.NewString(), Stage: stage, Definition: definition}), "skill.executor_unavailable")
		definition["tool_dependencies"] = ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{typedCalculatorDependency()}}
	}
}

func TestDependencyPublishRejectsRevisionChangedAfterPreflight(t *testing.T) {
	f := newDependencyFixture(t)
	repo := skillrepo.NewSkillDefinitionRepository(f.db)
	svc := NewDefinitionService(repo, f.checks)
	source := createAgentAuthoringSource(t, svc, f.tenant, f.member)
	draft, first, err := svc.CreateDraft(f.ctx, CreateDefinitionDraftInput{TenantUUID: f.tenant, SkillID: "customer.racing_publish",
		DisplayNameI18n: map[string]string{"zh-CN": "并发发布"}, DescriptionI18n: map[string]string{"zh-CN": "并发发布"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring,
		PackageSourceUUID: source.UUID.String(), AuthorMemberUUID: f.member, Definition: f.definition()})
	require.NoError(t, err)
	f.checks.options.PinnedTools = func(ctx context.Context, _ DependencyCheckInput, _ ToolDependencyV2, _ *skillrepo.ToolCapabilitySnapshot) error {
		_, _, err := svc.AppendRevision(ctx, f.tenant, draft.UUID.String(), f.member, llmPromptDefinition("not checked"), "concurrent edit", "")
		return err
	}
	_, _, err = svc.PublishCurrentRevision(f.ctx, PublishDefinitionInput{TenantUUID: f.tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "local://fixture/checked.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: f.member})
	require.ErrorIs(t, err, skillrepo.ErrSkillDefinitionRevisionConflict)
	current, err := repo.GetCurrentRevision(f.ctx, f.tenant, draft.UUID.String())
	require.NoError(t, err)
	require.NotEqual(t, first.UUID, current.UUID)
	require.Equal(t, skillmodel.SkillDefinitionRevisionStatusDraft, current.Status)
	require.Empty(t, current.PublishedArtifactURI)
}
