package skillsintegration

import (
	"context"
	"testing"

	agentruntime "github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
)

func TestSkillAgentResponsePlanningBoundCapabilityOnly(t *testing.T) {
	plan, err := agentruntime.NewResponsePlanner().Plan(context.Background(), agentruntime.ResponsePlanInput{
		UserMessage: "你能做什么？",
		AllowedCapabilities: []agentruntime.CapabilityContextItem{
			{ID: "powerxplugin.template.basic", Title: "模板能力"},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != agentruntime.ResponseModeCapabilityIntro {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if len(plan.TargetCapabilityIDs) != 1 || plan.TargetCapabilityIDs[0] != "powerxplugin.template.basic" {
		t.Fatalf("target ids leaked or missing: %v", plan.TargetCapabilityIDs)
	}
}

func TestSkillAgentResponsePlanningModes(t *testing.T) {
	caps := []agentruntime.CapabilityContextItem{
		{ID: "powerxplugin.template.basic", Title: "模板能力", RequiredArgs: []string{"action", "template_name"}},
	}
	cases := []struct {
		name string
		msg  string
		want agentruntime.ResponseMode
	}{
		{name: "howto", msg: "这个能力怎么用？", want: agentruntime.ResponseModeCapabilityHowTo},
		{name: "clarify", msg: "帮我创建一个模板", want: agentruntime.ResponseModeClarifyParams},
		{name: "intro", msg: "你有哪些能力？", want: agentruntime.ResponseModeCapabilityIntro},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := agentruntime.NewResponsePlanner().Plan(context.Background(), agentruntime.ResponsePlanInput{
				UserMessage:         tc.msg,
				AllowedCapabilities: caps,
			})
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			if plan.ResponseMode != tc.want {
				t.Fatalf("mode=%s want=%s", plan.ResponseMode, tc.want)
			}
		})
	}
}

func TestSkillAgentResponsePlanningExecutionAndErrorExplain(t *testing.T) {
	caps := []agentruntime.CapabilityContextItem{{ID: "powerx.release.report_synthesis"}}
	execPlan, err := agentruntime.NewResponsePlanner().Plan(context.Background(), agentruntime.ResponsePlanInput{
		UserMessage:           "生成发布准备报告",
		PlanHasExecutableNode: true,
		AllowedCapabilities:   caps,
	})
	if err != nil {
		t.Fatalf("Plan executable: %v", err)
	}
	if execPlan.ResponseMode != agentruntime.ResponseModeSkillExecution {
		t.Fatalf("exec mode=%s", execPlan.ResponseMode)
	}

	errorPlan, err := agentruntime.NewResponsePlanner().Plan(context.Background(), agentruntime.ResponsePlanInput{
		UserMessage:         "生成发布准备报告",
		ExecutionFailed:     true,
		ErrorSummary:        "executor unavailable",
		AllowedCapabilities: caps,
	})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if errorPlan.ResponseMode != agentruntime.ResponseModeErrorExplain {
		t.Fatalf("error mode=%s", errorPlan.ResponseMode)
	}
}
