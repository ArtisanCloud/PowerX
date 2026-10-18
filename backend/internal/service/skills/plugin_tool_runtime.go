package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PluginRuntimeProvider func() plugin_mgr.Manager
type pluginToolRuntime struct {
	plugins PluginRuntimeProvider
	catalog *skillrepo.ToolCatalogRepository
	agents  *agentrepo.AgentRepository
	checks  *DependencyService
}

func newPluginToolRuntime(db *gorm.DB, plugins PluginRuntimeProvider) *pluginToolRuntime {
	return &pluginToolRuntime{plugins: plugins, catalog: skillrepo.NewToolCatalogRepository(db), agents: agentrepo.NewAgentRepository(db)}
}

type pinnedPluginTool struct {
	runtime       plugin_mgr.PinnedHTTPRuntime
	proof         plugin_mgr.PinnedHTTPIdentity
	contract      *compiledPluginTool
	protocol      registrymodel.ProtocolBinding
	adapter       registrymodel.AdapterEndpoint
	catalogSHA256 string
	timeout       time.Duration
}

func (p *pluginToolRuntime) inspect(ctx context.Context, tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) (*pinnedPluginTool, error) {
	fail := func() (*pinnedPluginTool, error) { return nil, fmt.Errorf("skill.tool_protocol_unavailable") }
	if p == nil || p.plugins == nil || tool.Binding.Protocol != "http" || tool.Permission.Actor != "admin_user" || snapshot == nil {
		return fail()
	}
	manager := p.plugins()
	if manager == nil {
		return fail()
	}
	runtime, ok := manager.(plugin_mgr.PinnedHTTPRuntime)
	if !ok {
		return fail()
	}
	plugin, err := manager.Get(ctx, snapshot.Record.PluginID)
	if err != nil {
		return fail()
	}
	proof, err := runtime.PinHTTP(ctx, plugin.ID, tool.Binding.ProviderVersion, tool.Binding.ImplementationSHA256)
	if err != nil {
		return nil, fmt.Errorf("skill.tool_dependency_version_or_contract_mismatch")
	}
	var protocols []registrymodel.ProtocolBinding
	if json.Unmarshal(snapshot.Record.Protocols, &protocols) != nil {
		return fail()
	}
	var selected *registrymodel.ProtocolBinding
	for _, binding := range protocols {
		if binding.Channel == "http" && binding.ActorContext == "admin_user" {
			if selected != nil {
				return fail()
			}
			copy := binding
			selected = &copy
		}
	}
	if selected == nil || selected.Method != "POST" || selected.AuthType != "core_internal_token" || selected.ResourceScope != "tenant" || selected.Endpoint == "" || selected.SchemaRef == "" {
		return fail()
	}
	var adapter *registrymodel.AdapterEndpoint
	for _, endpoint := range snapshot.Registration.Adapters {
		if endpoint.IsActive && endpoint.TransportType == "http" && endpoint.Endpoint == plugin_mgr.CatalogPluginEndpoint(plugin.ID, "http", selected.Endpoint) {
			if adapter != nil {
				return fail()
			}
			copy := endpoint
			adapter = &copy
		}
	}
	if adapter == nil || adapter.TimeoutMS < 1 || adapter.TimeoutMS > 30000 {
		return fail()
	}
	contract, err := readPluginToolContract(ctx, plugin.Paths.Root, selected.SchemaRef, tool)
	if err != nil {
		return nil, err
	}
	effective := *selected
	effective.Endpoint, err = plugin_mgr.PluginHTTPRoute(plugin.Endpoints.HTTPBasePath, selected.Endpoint)
	if err != nil {
		return fail()
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return fail()
	}
	return &pinnedPluginTool{runtime: runtime, proof: proof, contract: contract, protocol: effective, adapter: *adapter, catalogSHA256: pluginToolDigest(raw), timeout: time.Duration(adapter.TimeoutMS) * time.Millisecond}, nil
}
func (p *pluginToolRuntime) check(ctx context.Context, in DependencyCheckInput, tool ToolDependencyV2, snapshot *skillrepo.ToolCapabilitySnapshot) error {
	_, err := p.inspect(ctx, tool, snapshot)
	if err != nil {
		return dependencyError(in, err.Error(), tool.DependencyKey, err)
	}
	return nil
}

func (p *pluginToolRuntime) prepare(ctx context.Context, id evidence.ToolRunIdentity, tool evidence.ToolDependency) (*evidence.BoundTool, error) {
	definition := map[string]any{"executor": map[string]any{"type": "capability", "capability_id": tool.ToolKey, "output_mode": "response_envelope", "response_contract": evidence.ReportSchemaV5}, "tool_dependencies": ToolRequirementsV2{Schema: ToolDependencySchemaV2, Tools: []ToolDependencyV2{tool}}}
	in := DependencyCheckInput{TenantUUID: id.TenantUUID, Stage: DependencyExecute, Env: reqctx.GetEnv(ctx), AgentID: evidence.ToolAgentID(ctx), Definition: definition}
	if err := p.checks.Check(ctx, in); err != nil {
		return nil, err
	}
	snapshot, err := p.catalog.LoadToolCapability(ctx, id.TenantUUID, tool.Binding.CapabilityUUID, tool.Binding.ContractUUID)
	if err != nil {
		return nil, err
	}
	bound, err := p.inspect(ctx, tool, snapshot)
	if err != nil {
		return nil, err
	}
	agent, err := p.agents.GetByID(ctx, in.AgentID)
	if err != nil || agent.UUID == uuid.Nil {
		return nil, fmt.Errorf("skill.tool_dependency_denied")
	}
	if id.AgentUUID != "" && id.AgentUUID != agent.UUID.String() {
		return nil, fmt.Errorf("skill.tool_dependency_denied")
	}
	description := bound.contract.description(agent.UUID.String())
	return &evidence.BoundTool{
		Description:   description,
		Binding:       evidence.ToolInvocationBinding{Kind: "capability", AdapterID: bound.adapter.AdapterID, RegistrationVersion: snapshot.Registration.Version, CatalogSHA256: bound.catalogSHA256},
		Authorization: evidence.ToolAuthorization{DecisionUUID: uuid.NewString(), SubjectUUID: reqctx.GetUserUUID(ctx), Actor: "admin_user"},
		ValidateRequest: func(ctx context.Context, raw []byte, inputs []evidence.ResolvedDatum) error {
			if err := validatePluginJSON(bound.contract.input, raw); err != nil {
				return err
			}
			var values map[string]json.RawMessage
			if evidence.DecodeToolJSON(raw, &values) != nil || len(inputs) != len(description.InputFields) {
				return fmt.Errorf("evidence.source_invalid")
			}
			seen := map[string]bool{}
			for _, datum := range inputs {
				field, ok := description.InputFields[datum.Source.Pointer]
				if !ok || seen[datum.Source.Pointer] || datum.Source.Kind != "run_input" || datum.FieldKey != field.FieldKey || datum.LabelKey != field.LabelKey || datum.DatumKind != "quantity" || datum.Unit != field.Unit || datum.Scope != field.Scope || string(values[datum.Source.Pointer[1:]]) != datum.Literal {
					return fmt.Errorf("evidence.source_invalid")
				}
				seen[datum.Source.Pointer] = true
			}
			return nil
		},
		Invoke: func(ctx context.Context, raw []byte) ([]byte, error) {
			callCtx, cancel := context.WithTimeout(ctx, bound.timeout)
			defer cancel()
			return bound.runtime.InvokePinnedHTTP(callCtx, bound.proof, plugin_mgr.PinnedHTTPRequest{Method: bound.protocol.Method, Path: bound.protocol.Endpoint, Body: raw, Headers: map[string]string{"X-PowerX-Tool-Tenant-UUID": id.TenantUUID, "X-PowerX-Tool-Subject-UUID": reqctx.GetUserUUID(ctx), "X-PowerX-Tool-Capability": tool.ToolKey, "X-PowerX-Trace-ID": id.TraceID}})
		},
		ValidateResult: func(_ context.Context, _, raw []byte) error { return validatePluginJSON(bound.contract.output, raw) },
		Recheck: func(ctx context.Context) error {
			if err := p.checks.Check(ctx, in); err != nil {
				return err
			}
			current, err := p.catalog.LoadToolCapability(ctx, id.TenantUUID, tool.Binding.CapabilityUUID, tool.Binding.ContractUUID)
			if err != nil {
				return err
			}
			now, err := p.inspect(ctx, tool, current)
			if err != nil {
				return err
			}
			if now.catalogSHA256 != bound.catalogSHA256 || now.proof != bound.proof {
				return dependencyError(in, "skill.tool_dependency_version_or_contract_mismatch", tool.DependencyKey, nil)
			}
			return nil
		},
		OutputFields: func([]byte, []byte) map[string]evidence.SourceField { return description.ResultFields },
	}, nil
}

func sortedToolPointers(fields map[string]evidence.SourceField) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
