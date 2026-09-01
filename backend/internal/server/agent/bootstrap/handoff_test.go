package bootstrap

import (
	"testing"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/stretchr/testify/require"
)

func TestSelectHandoffSkillRequiresExplicitSelectionForMultipleBindings(t *testing.T) {
	bindings := []agentmodel.AgentSkillBinding{
		{SkillID: "powerx.release.knowledge_analysis", Enabled: true},
		{SkillID: "powerx.release.workflow_planning", Enabled: true},
	}

	_, err := selectHandoffSkill(bindings, nil, nil)
	require.ErrorContains(t, err, "agent.handoff_skill_required")

	selected, err := selectHandoffSkill(bindings, map[string]any{
		"child_skill_id": "powerx.release.workflow_planning",
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "powerx.release.workflow_planning", selected)
}
