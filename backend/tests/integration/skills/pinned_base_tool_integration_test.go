package skillsintegration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/supervisor"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	skills "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	evidencemodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
)

type tempSkillPackageStore struct{ root string }

func (s tempSkillPackageStore) PutSkillPackage(_ context.Context, key, _ string, raw []byte) (string, error) {
	path := filepath.Join(s.root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return "", err
	}
	return "local://" + key, nil
}

// 此回归实际构建并启动 base.local 的领域工具进程，使用隔离安装目录/Registry/DB。
// 不使用 httptest 假上游，不触碰用户运行的 Core、插件、数据库和 grant。
func TestPinnedBasePluginCustomerSkillActualProcess(t *testing.T) { runPinnedBasePlugin(t, false) }
func TestT205PinnedBasePluginEvidenceMatrix(t *testing.T)         { runPinnedBasePlugin(t, true) }
func runPinnedBasePlugin(t *testing.T, matrix bool) {
	if matrix {
		if path := os.Getenv("POWERX_TEST_EVIDENCE_EXPORT"); path != "" {
			info, err := os.Lstat(path)
			if err == nil {
				require.True(t, info.Mode().IsRegular() && info.Size() == 0, "use a fresh empty evidence export")
			} else {
				require.True(t, os.IsNotExist(err))
			}
		}
	}
	root := os.Getenv("POWERX_T204_BASE_ROOT")
	if root == "" {
		root = "../../../../../Plugins/PowerXPlugin"
		if _, err := os.Stat(root); err != nil {
			t.Skip("set POWERX_T204_BASE_ROOT for the real base plugin fixture")
		}
	}
	root, err := filepath.Abs(root)
	require.NoError(t, err)
	module := filepath.Join(root, "skeleton/backend/go-gin")
	binary := filepath.Join(t.TempDir(), "parcel-tool")
	buildCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/parcel-tool")
	command.Dir = module
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	image, err := os.ReadFile(binary)
	require.NoError(t, err)
	implementation := fmt.Sprintf("sha256:%x", sha256.Sum256(image))
	contractRaw, err := os.ReadFile(filepath.Join(root, "skeleton/contracts/tools/parcel-volume.v1.json"))
	require.NoError(t, err)
	var document skills.PluginToolContractV1
	require.NoError(t, json.Unmarshal(contractRaw, &document))
	manifestRaw, err := os.ReadFile(filepath.Join(root, "skeleton/plugin.yaml"))
	require.NoError(t, err)
	var manifest plugin_mgr.Manifest
	require.NoError(t, yaml.Unmarshal(manifestRaw, &manifest))
	require.NotEmpty(t, manifest.Version)
	for _, base := range []string{"/api/v1", "/api/v2", "/gateway/custom"} {
		t.Run(base, func(t *testing.T) {
			db := setupSkillsDB(t)
			db.DisableForeignKeyConstraintWhenMigrating = true
			require.NoError(t, db.AutoMigrate(&skillmodel.SkillDefinitionDraft{}, &skillmodel.SkillDefinitionRevision{}, &skillmodel.SkillPackageSource{}, &capmodel.CapabilityContract{}, &capmodel.CapabilityIOSchema{}, &capmodel.CapabilityTransportProfile{}, &registrymodel.CapabilityRecord{}, &registrymodel.CapabilityRegistration{}, &registrymodel.AdapterEndpoint{}, &agentmodel.AgentCapabilityGrant{}, &evidencemodel.EvidenceSnapshot{}))
			require.NoError(t, db.Exec(`CREATE TABLE agents(id INTEGER PRIMARY KEY,uuid TEXT,env TEXT,tenant_uuid TEXT,key TEXT,name TEXT,status TEXT,owner_plugin_id TEXT,source TEXT,meta JSON,deleted_at DATETIME)`).Error)
			ctx := context.Background()
			tenant, user, agentUUID := uuid.NewString(), uuid.NewString(), uuid.New()
			ctx = reqctx.WithEnv(reqctx.WithIsRoot(reqctx.WithUserUUID(reqctx.WithTenantUUID(ctx, tenant), user), true), "dev")
			pluginID, providerVersion := "com.powerx.plugins.base.local", manifest.Version
			installed := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(installed, "contracts/tools"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(installed, "contracts/tools/parcel-volume.v1.json"), contractRaw, 0600))
			installedBinary := filepath.Join(installed, "bin", "parcel-tool")
			require.NoError(t, os.MkdirAll(filepath.Dir(installedBinary), 0700))
			require.NoError(t, os.WriteFile(installedBinary, image, 0500))
			registry := manager.NewJSONRegistry(filepath.Join(installed, "registry.json"))
			sup := supervisor.New()
			port, err := sup.Start(ctx, pluginID, installedBinary, nil, map[string]string{"POWERX_INTERNAL_TOKEN": uuid.NewString(), "POWERX_PLUGIN_HTTP_BASE": base, "GIN_MODE": "release"}, supervisor.Options{PinExecutable: true, HealthPath: "/healthz", HealthInterval: 500 * time.Millisecond, HealthTimeout: time.Second})
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, sup.Stop(pluginID))
				connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
				if connection != nil {
					_ = connection.Close()
				}
				require.Error(t, err, "agent-owned port must be released")
			})
			require.Eventually(t, func() bool {
				info, ok := sup.Status(pluginID)
				return ok && info.State == supervisor.ProcRunning && info.Healthy
			}, 20*time.Second, 50*time.Millisecond)
			cwd, err := os.Getwd()
			require.NoError(t, err)
			info, _ := sup.Status(pluginID)
			processPin, err := sup.Pin(pluginID)
			require.NoError(t, err)
			t.Logf("agent-owned process PID=%d command=%s installed_entry=%s cwd=%s port=%d cleanup=Supervisor.Stop", info.PID, processPin.ImagePath, installedBinary, cwd, port)
			require.NoError(t, registry.Put(ctx, manager.Descriptor{Manifest: plugin_mgr.Manifest{ID: pluginID, Version: providerVersion, Runtime: plugin_mgr.RuntimeSpec{Kind: plugin_mgr.RuntimeKindProcess, Entry: installedBinary, PinExecutable: true}, Endpoints: plugin_mgr.EndpointSpec{HTTPBasePath: base}}, Paths: plugin_mgr.InstalledPaths{Root: installed, Entry: installedBinary}}, plugin_mgr.StateEnabled))
			pluginManager := manager.New(manager.Options{Registry: registry, Supervisor: sup})
			tool := skills.ToolDependencyV2{DependencyKey: "parcel", ToolKey: document.ToolKey, Version: document.Version, InputSchema: skills.ToolSchemaRef{ID: document.Input.ID, SHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(contractRaw))}, OutputSchema: skills.ToolSchemaRef{ID: document.Output.ID, SHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(contractRaw))}, Permission: skills.ToolPermission{Actor: "admin_user", Code: "parcel.volumes:compute"}, Binding: skills.ToolBinding{Kind: "capability", CapabilityUUID: uuid.NewString(), ContractUUID: uuid.NewString(), ProviderVersion: providerVersion, ImplementationSHA256: implementation, Protocol: "http"}}
			now := time.Now()
			protocol := registrymodel.ProtocolBinding{Channel: "http", Endpoint: "/internal/tools/parcel-volumes", Method: "POST", SchemaRef: "contracts/tools/parcel-volume.v1.json", ActorContext: "admin_user", AuthType: "core_internal_token", ResourceScope: "tenant"}
			protocols, _ := json.Marshal([]registrymodel.ProtocolBinding{protocol})
			record := registrymodel.CapabilityRecord{UUID: uuid.MustParse(tool.Binding.CapabilityUUID), CapabilityID: tool.ToolKey, PluginID: pluginID, PluginVersion: providerVersion, Title: document.Labels["en-US"]["parcel.volume"], Protocols: protocols, Annotations: datatypes.JSON(`{"permission_code":"parcel.volumes:compute","agent_usable":true,"risk_level":"low"}`), Status: "published", PublishedAt: &now}
			require.NoError(t, db.Create(&record).Error)
			contract := capmodel.CapabilityContract{TenantUUID: tenant, CapabilityKey: tool.ToolKey, Version: tool.Version, ProviderID: pluginID, ContractUUID: uuid.MustParse(tool.Binding.ContractUUID), LifecycleState: "published", Status: 1}
			require.NoError(t, db.Create(&contract).Error)
			for direction, ref := range map[string]skills.ToolSchemaRef{"input": tool.InputSchema, "output": tool.OutputSchema} {
				require.NoError(t, db.Create(&capmodel.CapabilityIOSchema{TenantUUID: tenant, ContractID: contract.ID, Direction: direction, Format: "json_schema", SchemaURI: ref.ID, SchemaHash: ref.SHA256, Status: 1}).Error)
			}
			require.NoError(t, db.Create(&capmodel.CapabilityTransportProfile{TenantUUID: tenant, ContractID: contract.ID, CapabilityKey: tool.ToolKey, Transport: "http", Mode: "sync", TimeoutMillis: 1000, Status: 1}).Error)
			registration := registrymodel.CapabilityRegistration{TenantUUID: tenant, CapabilityID: tool.ToolKey, ContractRef: tool.Version, Status: "published", Version: 1, PublishedAt: &now, RoutingPolicyID: uuid.New()}
			require.NoError(t, db.Create(&registration).Error)
			require.NoError(t, db.Create(&registrymodel.AdapterEndpoint{RegistrationID: registration.ID, TenantUUID: tenant, CapabilityID: tool.ToolKey, AdapterID: "parcel-http", TransportType: "http", Endpoint: plugin_mgr.CatalogPluginEndpoint(pluginID, "http", protocol.Endpoint), IsActive: true, Weight: 1, TimeoutMS: 1000}).Error)
			require.NoError(t, db.Table("agents").Create(map[string]any{"id": 1, "uuid": agentUUID, "env": "dev", "tenant_uuid": tenant, "key": "customer_parcels", "status": "active"}).Error)
			require.NoError(t, db.Create(&agentmodel.AgentCapabilityGrant{Env: "dev", TenantUUID: tenant, AgentUUID: agentUUID, CapabilityUUID: record.UUID, CapabilityID: tool.ToolKey, PluginID: pluginID, PermissionCode: tool.Permission.Code, Status: "enabled"}).Error)
			checks, tools := skills.NewRuntimeSkillTools(db, func() plugin_mgr.Manager { return pluginManager })
			repo := skillrepo.NewSkillDefinitionRepository(db)
			definitions := skills.NewDefinitionService(repo, checks)
			publisher := skills.NewPackagePublisher(tempSkillPackageStore{root: t.TempDir()})
			store := evidencesvc.NewService(db)
			executor := skills.NewManifestExecutor(skills.ManifestExecutorOptions{Dependencies: checks, Tools: tools, EvidenceStore: store})
			invoker := skills.NewDefinitionInvokeService(repo, executor, nil)
			var history []evidence.EvidenceReference
			for _, skillID := range []string{"customer.parcel_measurement", "customer.renamed_capacity"} {
				definition := map[string]any{"schema": skills.SkillDefinitionSchemaV2, "executor": map[string]any{"type": "capability", "capability_id": tool.ToolKey, "output_mode": "response_envelope", "response_contract": evidence.ReportSchemaV5}, "entrypoints": []string{"runbook.default"}, "tool_dependencies": skills.ToolRequirementsV2{Schema: skills.ToolDependencySchemaV2, Tools: []skills.ToolDependencyV2{tool}}}
				name := document.Labels["en-US"]["parcel.volume"]
				sourcePackage, err := publisher.PublishAuthoringSource(ctx, skills.SourceSkillPackageInput{TenantUUID: tenant, SkillID: skillID, SourceUUID: uuid.NewString(), DisplayName: name, Description: name, Definition: definition})
				require.NoError(t, err)
				source, err := definitions.CreatePackageSource(ctx, skills.CreatePackageSourceInput{TenantUUID: tenant, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, ArtifactURI: sourcePackage.ArtifactURI, Checksum: sourcePackage.Checksum, ContentType: sourcePackage.ContentType, StandardManifest: map[string]any{"name": name, "description": name}, PowerXExtension: map[string]any{"schema": skills.SkillDefinitionSchemaV2}, CreatedByMemberUUID: user})
				require.NoError(t, err)
				draft, revision, err := definitions.CreateDraft(ctx, skills.CreateDefinitionDraftInput{TenantUUID: tenant, SkillID: skillID, DisplayNameI18n: map[string]string{"en-US": name}, DescriptionI18n: map[string]string{"en-US": name}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: definition, AuthorMemberUUID: user})
				require.NoError(t, err)
				pkg, err := publisher.PublishCanonical(ctx, skills.CanonicalSkillPackageInput{TenantUUID: tenant, SkillID: skillID, RevisionUUID: revision.UUID.String(), DisplayName: name, Description: name, Definition: definition})
				require.NoError(t, err)
				_, _, err = definitions.PublishCurrentRevision(ctx, skills.PublishDefinitionInput{TenantUUID: tenant, DraftUUID: draft.UUID.String(), ArtifactURI: pkg.ArtifactURI, Checksum: pkg.Checksum, UpdatedByMemberUUID: user})
				require.NoError(t, err)
				require.NoError(t, definitions.CheckRunnableForAgent(ctx, tenant, skillID, "dev", 1))
				// Planner 输出不是用户来源；没有入口快照、或篡改已冻结数字均在 HTTP 前拒绝。
				original := evidence.StructuredToolInput{Schema: evidence.StructuredToolInputSchema, Values: map[string]string{"length_cm": "12.5", "width_cm": "4", "height_cm": "2"}}
				params := map[string]any{"length_cm": json.Number("12.5"), "width_cm": json.Number("4"), "height_cm": json.Number("2")}
				beforeInvalid, _ := sup.Logs(pluginID, 1<<20)
				_, err = invoker.Execute(ctx, skills.InvokeRequest{TenantUUID: tenant, SkillID: skillID, TraceID: uuid.NewString()}, params, map[string]any{"agent_id": 1, "tool_input": original})
				require.ErrorContains(t, err, "evidence.source_invalid")
				captured, err := evidence.WithStructuredToolInput(ctx, original)
				require.NoError(t, err)
				forged := map[string]any{"length_cm": json.Number("999"), "width_cm": json.Number("4"), "height_cm": json.Number("2")}
				_, err = invoker.Execute(captured, skills.InvokeRequest{TenantUUID: tenant, SkillID: skillID, TraceID: uuid.NewString()}, forged, map[string]any{"agent_id": 1})
				require.ErrorContains(t, err, "evidence.source_invalid")
				afterInvalid, _ := sup.Logs(pluginID, 1<<20)
				require.Equal(t, strings.Count(beforeInvalid, "parcel.volume.invoked"), strings.Count(afterInvalid, "parcel.volume.invoked"))

				samples := []struct{ key, length, width, height, want string }{{"original", "12.5", "4", "2", "100"}}
				if matrix {
					samples = append(samples, struct{ key, length, width, height, want string }{"decimal", "0.1", "0.2", "0.3", "0.006"}, struct{ key, length, width, height, want string }{"large_exact", "123456789012345678901234567890.125", "2", "1", "246913578024691357802469135780.25"})
				}
				var ref evidence.EvidenceReference
				var latestLive datatypes.JSONMap
				for _, sample := range samples {
					runCtx := evidence.WithInvocationScope(evidence.WithLedger(ctx), uuid.NewString(), "parcel")
					result, err := invoker.ExecuteStructuredInput(runCtx, skills.InvokeRequest{TenantUUID: tenant, SkillID: skillID, Version: revision.UUID.String(), TraceID: uuid.NewString()}, map[string]any{"length_cm": json.Number(sample.length), "width_cm": json.Number(sample.width), "height_cm": json.Number(sample.height)}, map[string]any{"agent_id": 1, "locale": "en-US"})
					require.NoError(t, err)
					raw, err := json.Marshal(result.Result["response_envelope"])
					require.NoError(t, err)
					var report evidence.ReportV5
					require.NoError(t, evidence.DecodeToolJSON(raw, &report))
					require.Equal(t, sample.want, report.Presentation.Computed[0].Value)
					require.Equal(t, "domain_algorithm", report.Presentation.Computed[0].Derivation.Algorithm.Kind)
					require.Equal(t, tool.ToolKey, report.Presentation.Computed[0].Derivation.Algorithm.AlgorithmID)
					require.Len(t, report.Receipts, 1)
					require.Equal(t, "succeeded", report.Receipts[0].Status)
					require.Equal(t, implementation, report.Receipts[0].Tool.Binding.ImplementationSHA256)
					require.Equal(t, user, report.Receipts[0].Authorization.SubjectUUID)
					for _, datum := range report.Presentation.Reported {
						require.Equal(t, "user_supplied_unverified", datum.InputState)
					}
					require.NoError(t, evidence.Verify(runCtx, result.Result["response_envelope"]))
					ref = result.Result["response_evidence"].(evidence.EvidenceReference)
					history = append(history, ref)
					require.Eventually(t, func() bool {
						logs, _ := sup.Logs(pluginID, 1<<20)
						return strings.Count(logs, "parcel.volume.invoked") >= len(history)
					}, time.Second, 10*time.Millisecond)
					a, err := evidencesvc.NewService(db).Read(ctx, ref)
					require.NoError(t, err)
					require.Equal(t, agentUUID.String(), a.AgentUUID)
					live := datatypes.JSONMap{}
					for key, value := range result.Result {
						live[key] = value
					}
					latestLive = live
					historyMeta := evidencesvc.NewService(db).ProjectHistory(ctx, live)
					require.Equal(t, "available", historyMeta["response_evidence_state"])
					if matrix {
						exportT205History(t, map[string]any{"case": sample.key, "skill_id": skillID, "http_base": base, "expected_value": sample.want, "live": live, "history": historyMeta})
					}
					t.Logf("tool_contract_fixture %s", mustIntegrationJSON(t, map[string]any{"definition": "report", "value": report}))
				}

				// 撤权后同一个用户/root 也不能消费旧归档；context map 自报权限不能复活授权。
				require.NoError(t, db.Model(&agentmodel.AgentCapabilityGrant{}).Where("agent_uuid = ?", agentUUID).Update("status", "disabled").Error)
				_, err = store.Read(ctx, ref)
				require.ErrorContains(t, err, "evidence.snapshot_unavailable")
				if matrix {
					denied := store.ProjectHistory(ctx, latestLive)
					require.Equal(t, "unavailable", denied["response_evidence_state"])
					require.NotContains(t, denied, "response_envelope")
					exportT205History(t, map[string]any{"case": "revoked", "skill_id": skillID, "http_base": base, "history": denied})
				}
				before, _ := sup.Logs(pluginID, 1<<20)
				_, err = invoker.Execute(ctx, skills.InvokeRequest{TenantUUID: tenant, SkillID: skillID, TraceID: uuid.NewString()}, map[string]any{"length_cm": 100, "width_cm": 4, "height_cm": 2}, map[string]any{"agent_id": 1, "locale": "en-US", "is_root": true})
				require.ErrorContains(t, err, "skill.tool_dependency_denied")
				after, _ := sup.Logs(pluginID, 1<<20)
				require.Equal(t, strings.Count(before, "parcel.volume.invoked"), strings.Count(after, "parcel.volume.invoked"))
				require.NoError(t, db.Model(&agentmodel.AgentCapabilityGrant{}).Where("agent_uuid = ?", agentUUID).Update("status", "enabled").Error)
			}
			logs, _ := sup.Logs(pluginID, 1<<20)
			expectedCalls := 2
			if matrix {
				expectedCalls = 6
			}
			require.Equal(t, expectedCalls, strings.Count(logs, "parcel.volume.invoked"))

			if matrix {
				// 三类真实目录缺失/漂移都走生产预检，不能用合法输入触发 HTTP。
				for _, test := range []struct {
					name, code, table, column string
					bad, good                 any
				}{
					{"missing_contract", "skill.tool_dependency_unavailable", contract.TableName(), "lifecycle_state", "draft", "published"},
					{"disabled_adapter", "skill.tool_protocol_unavailable", registrymodel.AdapterEndpoint{}.TableName(), "is_active", false, true},
					{"provider_drift", "skill.tool_dependency_version_or_contract_mismatch", record.TableName(), "plugin_version", "999.0.0", providerVersion},
				} {
					require.NoError(t, db.Table(test.table).Where("1 = 1").Update(test.column, test.bad).Error)
					out, err := invoker.ExecuteStructuredInput(ctx, skills.InvokeRequest{TenantUUID: tenant, SkillID: "customer.parcel_measurement", TraceID: uuid.NewString()}, map[string]any{"length_cm": 12, "width_cm": 4, "height_cm": 2}, map[string]any{"agent_id": 1, "locale": "en-US"})
					require.Nil(t, out)
					require.ErrorContains(t, err, test.code)
					after, _ := sup.Logs(pluginID, 1<<20)
					require.Equal(t, expectedCalls, strings.Count(after, "parcel.volume.invoked"), test.name)
					require.NoError(t, db.Table(test.table).Where("1 = 1").Update(test.column, test.good).Error)
				}
			}
			if matrix && base == "/api/v2" {
				// 暂停我们拥有的真实进程。TCP 请求仍由 Manager 发出，但响应超过正式 adapter deadline。
				func() {
					require.NoError(t, db.Model(&registrymodel.AdapterEndpoint{}).Where("adapter_id = ?", "parcel-http").Update("timeout_ms", 50).Error)
					require.NoError(t, syscall.Kill(info.PID, syscall.SIGSTOP))
					defer func() { require.NoError(t, syscall.Kill(info.PID, syscall.SIGCONT)) }()
					timeoutCtx := evidence.WithInvocationScope(evidence.WithLedger(ctx), uuid.NewString(), "parcel_timeout")
					trace := uuid.NewString()
					result, err := invoker.ExecuteStructuredInput(timeoutCtx, skills.InvokeRequest{TenantUUID: tenant, SkillID: "customer.parcel_measurement", TraceID: trace}, map[string]any{"length_cm": 12, "width_cm": 4, "height_cm": 2}, map[string]any{"agent_id": 1, "locale": "en-US"})
					require.Nil(t, result)
					require.Error(t, err)
					failure, ok := evidence.FailureDetails(err)
					require.True(t, ok)
					require.Equal(t, "skill.tool_timeout", failure.Code)
					require.Equal(t, "tool_invoke", failure.Stage)
					var row evidencemodel.EvidenceSnapshot
					require.NoError(t, db.Where("trace_id = ?", trace).First(&row).Error)
					archived, err := evidencesvc.NewService(db).Read(ctx, evidence.EvidenceReference{Schema: evidence.ArchiveSchema, SnapshotUUID: row.UUID.String(), SHA256: row.SHA256})
					require.NoError(t, err)
					require.Nil(t, archived.Report)
					require.Len(t, archived.Receipts, 1)
					require.Equal(t, "timed_out", archived.Receipts[0].Status)
					require.Nil(t, archived.Receipts[0].Result)
					t.Logf("tool_contract_fixture %s", mustIntegrationJSON(t, map[string]any{"definition": "receipt", "value": archived.Receipts[0]}))
				}()
			}
			// 旧进程停止后，只读历史仍用冻结原件和当前权限，不要求重新运行该实现。
			require.NoError(t, sup.Stop(pluginID))
			for _, ref := range history {
				_, err := evidencesvc.NewService(db).Read(ctx, ref)
				require.NoError(t, err)
			}
		})
	}
}
func mustIntegrationJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}

// 只导出隔离测试生成的 metadata，不接受手写报告替身；给前端刷新回归消费真实后端原件。
func exportT205History(t *testing.T, value map[string]any) {
	t.Helper()
	path := os.Getenv("POWERX_TEST_EVIDENCE_EXPORT")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	require.NoError(t, err)
	require.NoError(t, json.NewEncoder(file).Encode(value))
	require.NoError(t, file.Close())
}
