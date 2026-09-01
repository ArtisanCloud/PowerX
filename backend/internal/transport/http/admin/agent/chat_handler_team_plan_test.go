package agent

import "testing"

func TestTeamRuntimeContextHasExecutablePlan(t *testing.T) {
	if teamRuntimeContextHasExecutablePlan(map[string]any{"team_key": "any", "agent_workspace_mode": "team"}) {
		t.Fatal("a team key alone must not make a plan executable")
	}
	if !teamRuntimeContextHasExecutablePlan(map[string]any{"team_key": "any", "team_orchestration": map[string]any{"schema": "powerx.agent.team-orchestration/v1", "tasks": []any{}}}) {
		t.Fatal("a validated orchestration context must make a plan executable")
	}
}
