package skills

import (
	"context"
	"encoding/json"
	"testing"

	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func v5Definition(t *testing.T) map[string]any {
	t.Helper()
	definition := evidenceDefinition()
	executor := definition["executor"].(map[string]any)
	policy := executor["calculation_policy"].(evidence.CalculationPolicy)
	policy.Schema = evidence.PolicySchemaV3
	for i := range policy.InputFields {
		policy.InputFields[i].ScopeI18n = map[string]string{"en-US": "s"}
	}
	policy.Formulas[0].Percent = false
	policy.Formulas[0].ResultUnit = "1"
	executor["calculation_policy"] = policy
	executor["response_contract"] = evidence.ReportSchemaV5
	tool, err := NewRuntimeToolService().CalculatorDependency("calculator")
	require.NoError(t, err)
	definition["tool_dependencies"] = ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{tool}}
	return definition
}

func TestV5PublishedDefinitionExecutesPersistsAndReadsWithoutCurrentRuntime(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.EvidenceSnapshot{}))
	tenant, user := uuid.NewString(), uuid.NewString()
	ctx := evidence.WithInvocationScope(evidence.WithLedger(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), user)), "run_v5", "review")
	repository := skillrepo.NewSkillDefinitionRepository(db)
	definitions := NewDefinitionService(repository, NewRuntimeDependencyService(db))
	source := createAgentAuthoringSource(t, definitions, tenant, user)
	draft, _, err := definitions.CreateDraft(ctx, CreateDefinitionDraftInput{TenantUUID: tenant, SkillID: "customer.warehouse_ratio", DisplayNameI18n: map[string]string{"en-US": "Warehouse ratio"}, DescriptionI18n: map[string]string{"en-US": "Use supplied quantities"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: v5Definition(t), AuthorMemberUUID: user})
	require.NoError(t, err)
	_, _, err = definitions.PublishCurrentRevision(ctx, PublishDefinitionInput{TenantUUID: tenant, DraftUUID: draft.UUID.String(), ArtifactURI: "s3://powerx-skills/fixed-revision.tgz", Checksum: "sha256:fixture", UpdatedByMemberUUID: user})
	require.NoError(t, err)
	calls := 0
	store := evidencesvc.NewService(db)
	executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: NewRuntimeDependencyService(db), EvidenceStore: store, LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		if calls == 1 {
			return fixtureFactsInventory(in.ResponseSchema), nil
		}
		if calls == 2 {
			return `{"schema":"powerx.agent.evidence-source/v1","data":{"a":{"scope":"s","token_ref":"token_0"},"b":{"scope":"s","token_ref":"token_1"}}}`, nil
		}
		return `{"schema":"powerx.agent.evidence-notes/v1","hypotheses":[],"gaps":[],"actions":[]}`, nil
	}})
	result, err := NewDefinitionInvokeService(repository, executor, nil).Execute(ctx, InvokeRequest{TenantUUID: tenant, SkillID: draft.SkillID, TraceID: "trace_v5"}, map[string]any{"message": "test a:30; b:12"}, map[string]any{"locale": "en-US"})
	require.NoError(t, err)
	require.Equal(t, 3, calls)
	var report evidence.ReportV5
	raw, _ := json.Marshal(result.Result["response_envelope"])
	require.NoError(t, evidence.DecodeToolJSON(raw, &report))
	require.Equal(t, "2.5", report.Presentation.Computed[0].Value)
	require.Equal(t, "completed", report.Outcome)
	require.Equal(t, "run_v5", report.RunID)
	require.NoError(t, evidence.Verify(ctx, result.Result["response_envelope"]))
	require.Error(t, evidence.Verify(evidence.WithLedger(ctx), result.Result["response_envelope"]))
	ref := result.Result["response_evidence"].(evidence.EvidenceReference)
	// 新建 Service 只读 DB 原件；不安装新实现、再次调用 LLM/工具或导入历史 ledger。
	a, err := evidencesvc.NewService(db).Read(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, "2.5", a.Report.Presentation.Computed[0].Value)
	require.Equal(t, 3, calls)
	_, err = store.Save(ctx, a)
	require.ErrorContains(t, err, "evidence.tool_receipt_untrusted")
	meta := datatypes.JSONMap{}
	for key, value := range result.Result {
		meta[key] = value
	}
	var serialized datatypes.JSONMap
	bytes, _ := json.Marshal(meta)
	require.NoError(t, json.Unmarshal(bytes, &serialized))
	projected := store.ProjectHistory(ctx, serialized)
	require.Equal(t, "available", projected["response_evidence_state"])
	// 消息标签不作为可信来源；按原 Revision 的归档标签恢复。
	serialized["response_labels"] = map[string]any{"schema": "forged"}
	require.Equal(t, a.Labels, store.ProjectHistory(ctx, serialized)["response_labels"])
	badMeta := datatypes.JSONMap{}
	require.NoError(t, json.Unmarshal(bytes, &badMeta))
	badMeta["response_envelope"].(map[string]any)["outcome"] = "needs_action"
	require.Equal(t, "unavailable", store.ProjectHistory(ctx, badMeta)["response_evidence_state"])
	badRef := ref
	badRef.SHA256 = "sha256:forged"
	_, err = store.Read(ctx, badRef)
	require.ErrorContains(t, err, "evidence.snapshot_unavailable")
	foreign := reqctx.WithUserUUID(ctx, uuid.NewString())
	require.Equal(t, "unavailable", store.ProjectHistory(foreign, serialized)["response_evidence_state"])
	foreignTenant := reqctx.WithTenantUUID(ctx, uuid.NewString())
	require.Equal(t, "unavailable", store.ProjectHistory(foreignTenant, serialized)["response_evidence_state"])
	require.NoError(t, db.Model(&model.EvidenceSnapshot{}).Where("uuid = ?", ref.SnapshotUUID).Updates(map[string]any{"status": 0, "payload": []byte(`{}`)}).Error)
	projected = store.ProjectHistory(ctx, serialized)
	require.Equal(t, "unavailable", projected["response_evidence_state"])
	require.NotContains(t, projected, "response_envelope")
	require.Equal(t, 3, calls)
	t.Logf("tool_contract_fixture %s", mustV5JSON(t, map[string]any{"definition": "report", "value": report}))
}

func TestV5BusinessGapsConflictsAndSystemFailuresAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, message, reported, outcome, value     string
		fields, gaps, conflicts, computed, receipts int
		failure                                     bool
	}{
		{name: "missing", message: "test a:30", fields: 1, gaps: 1, outcome: "needs_action"},
		{name: "conflict", message: "test a:30; b:12; claimed:3", reported: "3", fields: 3, conflicts: 1, computed: 1, receipts: 1, outcome: "needs_action"},
		{name: "equivalent_decimal", message: "test a:30; b:12; claimed:2.50", reported: "2.50", fields: 3, computed: 1, receipts: 1, outcome: "completed"},
		{name: "divide_zero", message: "test a:30; b:0", fields: 2, failure: true},
		{name: "changed_values", message: "test a:18; b:4", fields: 2, computed: 1, receipts: 1, outcome: "completed", value: "4.5"},
		{name: "precision", message: "test a:1; b:3", fields: 2, computed: 1, receipts: 1, outcome: "completed", value: "0.33"},
		{name: "large_exact", message: "test a:12345678901234567890; b:2", fields: 2, computed: 1, receipts: 1, outcome: "completed", value: "6172839450617283945"},
		{name: "percent_without_counts", message: "test claimed:27.8%", reported: "27.8", fields: 1, gaps: 1, outcome: "needs_action"},
		{name: "different_scope", message: "test a:30; b:12", fields: 2, gaps: 1, outcome: "needs_action"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDefinitionServiceTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.EvidenceSnapshot{}))
			definition := v5Definition(t)
			exec := definition["executor"].(map[string]any)
			policy := exec["calculation_policy"].(evidence.CalculationPolicy)
			if tc.reported != "" {
				f := policy.InputFields[0]
				f.Key, f.Kind = "claimed", "reported"
				f.LabelI18n = map[string]string{"en-US": "Claimed ratio"}
				f.DescriptionI18n = map[string]string{"en-US": "Claimed ratio"}
				f.EvidenceTermsI18n = map[string][]string{"en-US": {"claimed"}}
				policy.InputFields = append(policy.InputFields, f)
				policy.Formulas[0].CompareTo = "claimed"
				if tc.name == "percent_without_counts" {
					policy.InputFields[len(policy.InputFields)-1].UnitTokens = []string{"%"}
					policy.Formulas[0].WhenAnyPresent = append(policy.Formulas[0].WhenAnyPresent, "claimed")
				}
			}
			if tc.name == "different_scope" {
				policy.InputFields[1].ScopeI18n = map[string]string{"en-US": "other"}
			}
			exec["calculation_policy"] = policy
			tenant, user, revision := uuid.NewString(), uuid.NewString(), uuid.NewString()
			ctx := evidence.WithInvocationScope(evidence.WithLedger(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), user)), "run_"+tc.name, "review")
			calls := 0
			store := evidencesvc.NewService(db)
			executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: NewRuntimeDependencyService(db), EvidenceStore: store, LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
				calls++
				if calls == 1 {
					return fixtureFactsInventory(in.ResponseSchema), nil
				}
				if calls == 2 {
					selected := map[string]evidence.SelectedValue{"a": {Scope: "s", TokenRef: "token_0"}}
					if tc.fields >= 2 {
						scope := "s"
						if tc.name == "different_scope" {
							scope = "other"
						}
						selected["b"] = evidence.SelectedValue{Scope: scope, TokenRef: "token_1"}
					}
					if tc.name == "percent_without_counts" {
						selected = map[string]evidence.SelectedValue{"claimed": {Scope: "s", TokenRef: "token_0"}}
					} else if tc.reported != "" {
						selected["claimed"] = evidence.SelectedValue{Scope: "s", TokenRef: "token_2"}
					}
					return string(mustV5JSON(t, evidence.SourceSelection{Schema: evidence.ExtractionSchema, Data: selected})), nil
				}
				return string(mustV5JSON(t, map[string]any{"schema": evidence.NotesSchema, "hypotheses": []string{}, "gaps": in.Payload["allowed_gaps"], "actions": []string{}})), nil
			}})
			out, err := executor.Execute(ctx, ExecuteInput{TenantUUID: tenant, SkillID: "customer.inventory", Version: revision, TraceID: "trace_" + tc.name, Manifest: definition, Payload: map[string]any{"message": tc.message}, Context: map[string]any{"locale": "en-US"}})
			if tc.failure {
				require.Nil(t, out)
				require.Error(t, err)
				failure, ok := evidence.FailureDetails(err)
				require.True(t, ok)
				require.Equal(t, "evidence.calculation_failed", failure.Code)
				require.Equal(t, "tool_invoke", failure.Stage)
				require.Equal(t, 2, calls)
				var rows []model.EvidenceSnapshot
				require.NoError(t, db.Find(&rows).Error)
				require.Len(t, rows, 1)
				a, readErr := store.Read(ctx, evidence.EvidenceReference{Schema: evidence.ArchiveSchema, SnapshotUUID: rows[0].UUID.String(), SHA256: rows[0].SHA256})
				require.NoError(t, readErr)
				require.Nil(t, a.Report)
				require.Equal(t, failure, *a.Failure)
				require.Len(t, a.Receipts, 1)
				require.Equal(t, "failed", a.Receipts[0].Status)
				require.Equal(t, failure, *a.Receipts[0].Error)
				require.Nil(t, a.Receipts[0].Result)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 3, calls)
			var report evidence.ReportV5
			require.NoError(t, evidence.DecodeToolJSON(mustV5JSON(t, out["response_envelope"]), &report))
			require.Equal(t, tc.outcome, report.Outcome)
			require.Len(t, report.Presentation.Gaps, tc.gaps)
			require.Len(t, report.Presentation.Conflicts, tc.conflicts)
			require.Len(t, report.Presentation.Computed, tc.computed)
			require.Len(t, report.Receipts, tc.receipts)
			if tc.value != "" {
				require.Equal(t, tc.value, report.Presentation.Computed[0].Value)
			}
			if tc.name == "percent_without_counts" {
				require.Len(t, report.Presentation.Reported, 1)
				require.Equal(t, "reported", report.Presentation.Reported[0].DatumKind)
				require.Equal(t, "27.8", report.Presentation.Reported[0].Value)
				require.Equal(t, "%", report.Presentation.Reported[0].Unit)
			}
			var row model.EvidenceSnapshot
			require.NoError(t, db.First(&row).Error)
			a, readErr := evidencesvc.NewService(db).Read(ctx, evidence.EvidenceReference{Schema: evidence.ArchiveSchema, SnapshotUUID: row.UUID.String(), SHA256: row.SHA256})
			require.NoError(t, readErr)
			require.Equal(t, report, *a.Report)
			require.NoError(t, evidence.Verify(ctx, out["response_envelope"]))
			t.Logf("tool_contract_fixture %s", mustV5JSON(t, map[string]any{"definition": "report", "value": report}))
		})
	}
}

func TestV5PreflightRejectsUnmigratedStorageAndOldPolicyBeforeLLM(t *testing.T) {
	for _, tc := range []string{"storage", "old_policy", "missing_scope", "unsupported_locale", "anonymous"} {
		t.Run(tc, func(t *testing.T) {
			db := setupDefinitionServiceTestDB(t)
			if tc != "storage" {
				require.NoError(t, db.AutoMigrate(&model.EvidenceSnapshot{}))
			}
			definition := v5Definition(t)
			exec := definition["executor"].(map[string]any)
			policy := exec["calculation_policy"].(evidence.CalculationPolicy)
			if tc == "old_policy" {
				policy.Schema = evidence.PolicySchema
				policy.Formulas[0].ResultUnit = ""
			}
			if tc == "missing_scope" {
				policy.InputFields[0].ScopeI18n = nil
			}
			if tc == "unsupported_locale" {
				exec["prompt_template_i18n"].(map[string]any)["fr-FR"] = "fixture"
			}
			exec["calculation_policy"] = policy
			calls := 0
			executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: NewRuntimeDependencyService(db), EvidenceStore: evidencesvc.NewService(db), LLM: func(context.Context, ManifestLLMInvocation) (string, error) { calls++; return "", nil }})
			tenant := uuid.NewString()
			ctx := reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), uuid.NewString())
			if tc == "anonymous" {
				ctx = context.Background()
			}
			_, err := executor.Execute(ctx, ExecuteInput{TenantUUID: tenant, Version: uuid.NewString(), TraceID: "preflight", Manifest: definition, Context: map[string]any{"locale": "en-US"}})
			if tc == "storage" {
				require.ErrorContains(t, err, "evidence.storage_unavailable")
			}
			if tc == "anonymous" {
				require.ErrorContains(t, err, "skill.tool_dependency_denied")
			}
			require.Error(t, err)
			require.Zero(t, calls)
		})
	}
}
func mustV5JSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
