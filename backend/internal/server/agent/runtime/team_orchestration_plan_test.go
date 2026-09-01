package runtime

import (
	"context"
	"testing"

	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func TestTeamPlanFromPersistedOrchestration(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "agent_workspace_mode", "team")
	ctx = context.WithValue(ctx, "team_id", "2")
	ctx = context.WithValue(ctx, "team_key", "custom.review")
	ctx = context.WithValue(ctx, "parent_agent_id", uint64(10))
	ctx = context.WithValue(ctx, "team_user_message", "请基于本次材料形成结论。")
	ctx = context.WithValue(ctx, "locale", "zh-CN")
	ctx = context.WithValue(ctx, "agent_bound_skill_ids", []string{"team.synthesis"})
	ctx = context.WithValue(ctx, "team_orchestration", map[string]any{
		"schema": modelagent.TeamOrchestrationSchemaV1,
		"tasks": []any{
			map[string]any{"task_id": "analyse", "node_kind": "agent_handoff", "assignee_role": "retriever", "skill_id": "team.analyse", "stage": 1, "failure_policy": "fail-fast"},
			map[string]any{"task_id": "synthesis", "node_kind": "skill", "assignee_role": "planner", "skill_id": "team.synthesis", "stage": 2, "depends_on": []any{"analyse"}},
		},
	})
	ctx = context.WithValue(ctx, "team_members", []map[string]any{
		{"role": "retriever", "child_agent_id": uint64(11), "child_agent_key": "custom.researcher", "skill_ids": []string{"team.analyse"}},
	})

	plan, handled, err := builtInTeamPlanFromContext(ctx)
	if err != nil || !handled {
		t.Fatalf("team plan handled=%v err=%v", handled, err)
	}
	if plan == nil || len(plan.Tasks) != 2 {
		t.Fatalf("plan=%#v", plan)
	}
	if plan.Tasks[0].NodeKind != dto.NodeKindHandoff || plan.Tasks[0].NodeRef != "custom.researcher" {
		t.Fatalf("handoff task=%#v", plan.Tasks[0])
	}
	final := plan.Tasks[1]
	if final.NodeKind != dto.NodeKindSkill || final.NodeRef != "team.synthesis" || final.ParamRefs["upstream_analyse"] != "{{task.analyse.output.result}}" {
		t.Fatalf("final task=%#v", final)
	}
}

func TestTeamPlanRejectsUnboundConfiguredSkill(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "agent_workspace_mode", "team")
	ctx = context.WithValue(ctx, "team_id", "2")
	ctx = context.WithValue(ctx, "parent_agent_id", uint64(10))
	ctx = context.WithValue(ctx, "team_user_message", "材料")
	ctx = context.WithValue(ctx, "team_orchestration", map[string]any{
		"schema": modelagent.TeamOrchestrationSchemaV1,
		"tasks":  []any{map[string]any{"task_id": "analyse", "node_kind": "agent_handoff", "assignee_role": "retriever", "skill_id": "team.analyse", "stage": 1}},
	})
	ctx = context.WithValue(ctx, "team_members", []map[string]any{{"role": "retriever", "child_agent_id": uint64(11), "child_agent_key": "custom.researcher", "skill_ids": []string{"other.skill"}}})

	if _, handled, err := builtInTeamPlanFromContext(ctx); !handled || err == nil {
		t.Fatalf("expected a bound-skill validation error, handled=%v err=%v", handled, err)
	}
}
