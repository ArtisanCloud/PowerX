package capabilityregistryintegration

import (
	"context"
	"errors"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	workflowengine "github.com/ArtisanCloud/PowerX/internal/workflow/engine"
	"github.com/stretchr/testify/require"
)

func TestWorkflowTelemetryCatalogAndExecution(t *testing.T) {
	env := newWorkflowIntegrationEnv(t)
	t.Cleanup(env.Close)

	artifact := buildWorkflowPluginArtifact(t, workflowPluginArtifactOptions{
		PluginID:              "demo.workflow.plugin",
		PluginVersion:         "1.0.0",
		CapabilityID:          "demo.workflow.capability",
		CapabilityTitle:       "Demo Workflow Capability",
		CapabilityDescription: "v1",
		TemplateID:            "tpl.demo.workflow",
		TemplateName:          "Workflow Demo",
		RequiresManualUpgrade: true,
	})
	require.NoError(t, env.worker.ProcessArtifact(env.ctx, artifact))

	snapshot := env.telemetry.Snapshot()
	require.Equal(t, 1, snapshot.TotalTemplates)
	require.Equal(t, 1, snapshot.NeedsUpgrade)
	require.Equal(t, int64(0), snapshot.ExecutionsTotal)

	fakeInvoker := &fakeInvocationService{}
	selector := capservice.NewSelector(capservice.SelectorOptions{
		Store: capservice.SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (capservice.SelectorPolicySnapshot, error) {
			return capservice.SelectorPolicySnapshot{
				TenantID:         tenant,
				CapabilitiesHash: "snapshot-demo",
				IntentMappings: map[string]map[string]string{
					"workflow.intent.demo": {"default": "demo.workflow.capability"},
				},
				PreferMatrix: map[string]capservice.ProtocolPreference{
					"demo.workflow.capability": {Prefer: "workflow", Fallback: []string{"mcp"}},
				},
			}, nil
		}),
		Invoker: fakeInvoker,
	})

	adapter := workflowengine.NewCapabilityStepAdapter(selector, env.telemetry)
	_, err := adapter.InvokeCapability(env.ctx, workflowengine.CapabilityStepInput{
		CapabilityID:          "demo.workflow.capability",
		TenantUUID:            "tenant-telemetry",
		Intent:                "workflow.intent.demo",
		ToolScope:             "default",
		PreferredProtocol:     "workflow",
		TemplateID:            "tpl.demo.workflow",
		RequiresManualUpgrade: true,
	})
	require.NoError(t, err)

	snapshot = env.telemetry.Snapshot()
	require.Equal(t, int64(1), snapshot.ExecutionsTotal)
	require.Equal(t, int64(0), snapshot.ExecutionsFailed)

	fakeInvoker.err = errors.New("invoke failed")
	_, err = adapter.InvokeCapability(env.ctx, workflowengine.CapabilityStepInput{
		CapabilityID:      "demo.workflow.capability",
		TenantUUID:        "tenant-telemetry",
		PreferredProtocol: "workflow",
		TemplateID:        "tpl.demo.workflow",
	})
	require.Error(t, err)

	snapshot = env.telemetry.Snapshot()
	require.Equal(t, int64(2), snapshot.ExecutionsTotal)
	require.Equal(t, int64(1), snapshot.ExecutionsFailed)
}
