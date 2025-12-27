package capabilityregistryintegration

import "testing"

import "github.com/stretchr/testify/require"

func TestWorkflowTemplateUpgradeFlow(t *testing.T) {
	env := newWorkflowIntegrationEnv(t)
	t.Cleanup(env.Close)

	artifactV1 := buildWorkflowPluginArtifact(t, workflowPluginArtifactOptions{
		PluginID:              "demo.workflow.plugin",
		PluginVersion:         "1.0.0",
		CapabilityID:          "demo.workflow.capability",
		CapabilityTitle:       "Demo Workflow Capability",
		CapabilityDescription: "v1",
		TemplateID:            "tpl.demo.workflow",
		TemplateName:          "Workflow Demo",
		RequiresManualUpgrade: true,
	})
	require.NoError(t, env.worker.ProcessArtifact(env.ctx, artifactV1))

	engine := env.newAdminEngine()
	templates, _ := listWorkflowTemplates(t, engine)
	tplV1 := templates[0]
	approveWorkflowTemplate(t, engine, tplV1.TemplateID, tplV1.CapabilitiesHash)

	artifactV2 := buildWorkflowPluginArtifact(t, workflowPluginArtifactOptions{
		PluginID:              "demo.workflow.plugin",
		PluginVersion:         "1.1.0",
		CapabilityID:          "demo.workflow.capability",
		CapabilityTitle:       "Demo Workflow Capability v2",
		CapabilityDescription: "v2",
		TemplateID:            "tpl.demo.workflow",
		TemplateName:          "Workflow Demo",
		RequiresManualUpgrade: true,
	})
	require.NoError(t, env.worker.ProcessArtifact(env.ctx, artifactV2))

	templates, _ = listWorkflowTemplates(t, engine)
	tplV2 := templates[0]
	require.NotEqual(t, tplV1.CapabilitiesHash, tplV2.CapabilitiesHash)
	require.True(t, tplV2.NeedsUpgrade)
	require.Equal(t, tplV1.CapabilitiesHash, tplV2.Approved.CapabilitiesHash)

	approval := approveWorkflowTemplate(t, engine, tplV2.TemplateID, tplV2.CapabilitiesHash)
	require.Equal(t, tplV2.CapabilitiesHash, approval.CapabilitiesHash)

	templates, _ = listWorkflowTemplates(t, engine)
	require.False(t, templates[0].NeedsUpgrade)
	require.Equal(t, tplV2.CapabilitiesHash, templates[0].Approved.CapabilitiesHash)
}
