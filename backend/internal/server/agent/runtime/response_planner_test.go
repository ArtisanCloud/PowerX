package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
)

func TestResponsePlanValidateRejectsUnauthorizedCapability(t *testing.T) {
	plan := &ResponsePlan{
		ResponseMode:        ResponseModeCapabilityIntro,
		TargetCapabilityIDs: []string{"skill.b"},
	}
	err := plan.Validate([]string{"skill.a"})
	if err == nil || !strings.Contains(err.Error(), ErrCodeContextCapabilityDenied) {
		t.Fatalf("expected capability denied error, got %v", err)
	}
}

func TestResponsePlannerCapabilityIntroUsesBoundCapabilities(t *testing.T) {
	planner := NewResponsePlanner()
	plan, err := planner.Plan(context.Background(), ResponsePlanInput{
		UserMessage: "你能做什么？",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "powerxplugin.template.basic", Title: "模板能力"},
		},
		ModelSelection: NodeModelSelection{Node: ModelPolicyNodeResponsePlanner, Mode: "inherit_default", Model: "qwen3:8b"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeCapabilityIntro {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if len(plan.TargetCapabilityIDs) != 1 || plan.TargetCapabilityIDs[0] != "powerxplugin.template.basic" {
		t.Fatalf("target ids=%v", plan.TargetCapabilityIDs)
	}
	if !plan.RepeatFullIntro {
		t.Fatalf("expected first intro to repeat full intro")
	}
	if plan.ModelSelection.Model != "qwen3:8b" {
		t.Fatalf("model selection not preserved: %#v", plan.ModelSelection)
	}
}

func TestResponsePlannerAgentIdentityUsesAgentProfileWithoutCapabilities(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "你好，你是什么智能体？",
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeCapabilityIntro {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if !plan.UseCapabilityCtx {
		t.Fatalf("expected identity question to use context")
	}
	if !hasResponseIntent(plan.ResponseIntents, ResponseIntentAgentIdentity) {
		t.Fatalf("missing agent identity intent: %#v", plan.ResponseIntents)
	}
	if len(plan.TargetCapabilityIDs) != 0 {
		t.Fatalf("unexpected target ids without allowed capabilities: %#v", plan.TargetCapabilityIDs)
	}
}

func TestResponsePlannerCapabilityIntroSupportsMultiIntent(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "你好，你是什么智能体？你能做什么？请只列出你已绑定的能力。你建议我先测试哪一个能力？",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "powerxplugin.template.basic", Title: "模板能力"},
		},
		RecentCapabilityIntro: true,
		ModelSelection:        NodeModelSelection{Node: ModelPolicyNodeResponsePlanner, Mode: "inherit_default", Model: "qwen3:8b"},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeCapabilityIntro {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	for _, want := range []ResponseIntent{ResponseIntentGreeting, ResponseIntentAgentIdentity, ResponseIntentCapabilityIntro, ResponseIntentTestRecommendation} {
		if !hasResponseIntent(plan.ResponseIntents, want) {
			t.Fatalf("missing intent %s in %#v", want, plan.ResponseIntents)
		}
	}
	requirementText := strings.Join(plan.AnswerRequirements, "\n")
	for _, want := range []string{"问候", "身份", "已绑定能力", "先测试"} {
		if !strings.Contains(requirementText, want) {
			t.Fatalf("missing answer requirement %q in %s", want, requirementText)
		}
	}
	prompt := BuildModeSpecificUserPrompt("你好，你是什么智能体？你能做什么？请只列出你已绑定的能力。你建议我先测试哪一个能力？", plan)
	if !strings.Contains(prompt, "response_intents: greeting, agent_identity, capability_intro, test_recommendation") {
		t.Fatalf("prompt missing response intents: %s", prompt)
	}
	if !strings.Contains(prompt, "answer_requirements:") || !strings.Contains(prompt, "基于当前已绑定能力推荐") {
		t.Fatalf("prompt missing answer requirements: %s", prompt)
	}
}

func TestResponsePlannerCapabilityIntroDoesNotRepeatRecentIntro(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "你能做什么？",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "powerxplugin.template.basic", Title: "模板能力"},
		},
		RecentCapabilityIntro: true,
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeCapabilityIntro {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if plan.RepeatFullIntro {
		t.Fatalf("expected repeated intro to be concise")
	}
	if !plan.RecentCapabilityIntro {
		t.Fatalf("expected recent intro marker")
	}
	if dbg := plan.ToDebugEvent(); dbg["recent_capability_intro"] != true {
		t.Fatalf("debug event missing recent intro marker: %#v", dbg)
	}
	prompt := BuildModeSpecificUserPrompt("你能做什么？", plan)
	if !strings.Contains(prompt, "[CURRENT_USER_MESSAGE]") || !strings.Contains(prompt, "你能做什么？") {
		t.Fatalf("prompt lost current user message: %s", prompt)
	}
	if !strings.Contains(prompt, "recent_capability_intro: true") {
		t.Fatalf("prompt lost recent intro marker: %s", prompt)
	}
	for _, want := range []string{"避免重复", "完整回应本轮", "Agent persona", "Skill metadata"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt lost repeated intro requirement %q: %s", want, prompt)
		}
	}
}

func TestResponsePlannerCapabilityHowTo(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "第一个能力怎么用？",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "skill.a", Title: "能力 A", RequiredArgs: []string{"template_name"}},
			{ID: "skill.b", Title: "能力 B"},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeCapabilityHowTo {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if len(plan.TargetCapabilityIDs) != 1 || plan.TargetCapabilityIDs[0] != "skill.a" {
		t.Fatalf("target ids=%v", plan.TargetCapabilityIDs)
	}
	if !plan.IncludeSchema || !plan.IncludeExamples {
		t.Fatalf("expected howto to include schema and examples")
	}
}

func TestResponsePlannerClarifyParams(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "帮我创建一个模板",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "skill.template", RequiredArgs: []string{"action", "template_name"}},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeClarifyParams {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if len(plan.MissingFields) != 1 || plan.MissingFields[0] != "template_name" {
		t.Fatalf("missing=%v", plan.MissingFields)
	}
}

func TestEngineMissingRequiredArgsForActionPlan(t *testing.T) {
	mgr := agent.GetAgentManager()
	skillID := "test.skill.action.required.runtime"
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name:        skillID,
		NodeKind:    "skill",
		NodeRef:     skillID,
		SourceScope: "agent",
		Source:      "plugin",
		ActionRequiredArgs: map[string][]string{
			"create": []string{"template.title", "template.description", "template.content"},
		},
	})
	engine := NewEngine()
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{skillID})
	missing := engine.missingRequiredArgsForPlan(ctx, &flowschema.ExecutionPlan{
		PlanID: "plan_test",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:   "task_create",
				FlowID:   skillID,
				NodeKind: "skill",
				NodeRef:  skillID,
				Params: map[string]interface{}{
					"action": "create",
					"template": map[string]interface{}{
						"title": "测试模板",
					},
				},
			},
		},
	})
	if strings.Join(missing, ",") != "template.description,template.content" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestEngineMissingRequiredArgsMergesPendingTaskParams(t *testing.T) {
	mgr := agent.GetAgentManager()
	skillID := "test.skill.pending.merge.runtime"
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name:        skillID,
		NodeKind:    "skill",
		NodeRef:     skillID,
		SourceScope: "agent",
		Source:      "plugin",
		ActionRequiredArgs: map[string][]string{
			"create": []string{"template.title", "template.description", "template.content"},
		},
	})
	engine := NewEngine()
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{skillID})
	ctx = context.WithValue(ctx, "agent_pending_task", map[string]any{
		"node_ref": skillID,
		"action":   "create",
		"status":   "awaiting_params",
		"collected_params": map[string]any{
			"action": "create",
			"template": map[string]any{
				"title":       "测试模板",
				"description": "用于测试跨轮补参",
			},
		},
	})
	missing := engine.missingRequiredArgsForPlan(ctx, &flowschema.ExecutionPlan{
		PlanID: "plan_pending_merge",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:   "task_create",
				FlowID:   skillID,
				NodeKind: "skill",
				NodeRef:  skillID,
				Params: map[string]interface{}{
					"action": "create",
				},
			},
		},
	})
	if strings.Join(missing, ",") != "template.content" {
		t.Fatalf("missing=%v", missing)
	}
}

func TestEnginePendingTaskExtractsMissingSlotFromUserMessage(t *testing.T) {
	mgr := agent.GetAgentManager()
	skillID := "test.skill.pending.slot.runtime"
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name:        skillID,
		NodeKind:    "skill",
		NodeRef:     skillID,
		SourceScope: "agent",
		Source:      "plugin",
		ActionRequiredArgs: map[string][]string{
			"create": []string{"template.title", "template.description", "template.content"},
		},
		SlotMapping: map[string]any{
			"template.title": map[string]any{
				"labels": []any{"标题", "名称", "模板标题"},
			},
			"template.description": map[string]any{
				"labels": []any{"描述", "用途", "说明"},
			},
			"template.content": map[string]any{
				"labels": []any{"内容", "正文", "模板内容"},
			},
		},
	})
	engine := NewEngine()
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{skillID})
	ctx = context.WithValue(ctx, "agent_pending_task", map[string]any{
		"node_ref": skillID,
		"action":   "create",
		"status":   "awaiting_params",
		"collected_params": map[string]any{
			"action": "create",
			"template": map[string]any{
				"title":       "测试模板",
				"description": "用于验证插件 CRUD",
			},
		},
	})
	plan := &flowschema.ExecutionPlan{
		PlanID: "plan_pending_slot",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:   "task_create",
				FlowID:   skillID,
				NodeKind: "skill",
				NodeRef:  skillID,
				Params: map[string]interface{}{
					"action":       "create",
					"user_message": "内容可以是“这是一条测试内容”",
				},
			},
		},
	}
	plan = engine.applyRuntimeParamState(ctx, plan)
	missing := engine.missingRequiredArgsForPlan(ctx, plan)
	if len(missing) != 0 {
		t.Fatalf("missing=%v plan=%#v", missing, plan.Tasks[0].Params)
	}
	if !hasPlanParamPath(plan.Tasks[0].Params, "template.content") {
		t.Fatalf("content not extracted: %#v", plan.Tasks[0].Params)
	}
}

func TestEnginePendingTaskConfirmationMergesCollectedParams(t *testing.T) {
	skillID := "test.skill.pending.confirmation.runtime"
	ctx := context.WithValue(context.Background(), "agent_pending_task", map[string]any{
		"node_ref":       skillID,
		"skill_id":       skillID,
		"action":         "delete",
		"status":         "awaiting_params",
		"missing_fields": []any{"confirmation"},
		"collected_params": map[string]any{
			"action":        "delete",
			"template_id":   5,
			"template_ref":  "测试模板2",
			"template_name": "测试模板2",
		},
	})

	detected, ok := pendingDetectedTaskFromContext(ctx, "确认")
	if !ok {
		t.Fatal("pending task was not resumed")
	}
	if detected.Params["confirmation"] != "确认" {
		t.Fatalf("confirmation=%#v params=%#v", detected.Params["confirmation"], detected.Params)
	}
	if detected.Params["confirmed"] != true {
		t.Fatalf("confirmed=%#v params=%#v", detected.Params["confirmed"], detected.Params)
	}

	task := mergePendingTaskParams(ctx, flowschema.PlanTask{
		TaskID:   "task_delete",
		FlowID:   skillID,
		NodeKind: "skill",
		NodeRef:  skillID,
		Params: map[string]interface{}{
			"action":       "delete",
			"user_message": "确认",
		},
	})
	if task.Params["template_id"] != 5 {
		t.Fatalf("template_id=%#v params=%#v", task.Params["template_id"], task.Params)
	}
	if task.Params["template_ref"] != "测试模板2" || task.Params["template_name"] != "测试模板2" {
		t.Fatalf("template labels lost: %#v", task.Params)
	}
	if task.Params["confirmation"] != "确认" || task.Params["confirmed"] != true {
		t.Fatalf("confirmation not merged: %#v", task.Params)
	}
}

func TestEngineInitialTaskExtractsRequiredSlotsFromUserMessage(t *testing.T) {
	mgr := agent.GetAgentManager()
	skillID := "test.skill.initial.slot.runtime"
	mgr.UpsertUnifiedCandidate(agent.ToolCallCandidate{
		Name:        skillID,
		NodeKind:    "skill",
		NodeRef:     skillID,
		SourceScope: "agent",
		Source:      "plugin",
		ActionRequiredArgs: map[string][]string{
			"create": []string{"template.title", "template.description", "template.content"},
		},
		SlotMapping: map[string]any{
			"template.title": map[string]any{
				"labels": []any{"标题", "名称", "模板标题"},
			},
			"template.description": map[string]any{
				"labels": []any{"描述", "用途", "说明"},
			},
			"template.content": map[string]any{
				"labels": []any{"内容", "正文", "模板内容"},
			},
		},
	})
	engine := NewEngine()
	ctx := context.WithValue(context.Background(), "agent_bound_skill_ids", []string{skillID})
	plan := &flowschema.ExecutionPlan{
		PlanID: "plan_initial_slot",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:   "task_create",
				FlowID:   skillID,
				NodeKind: "skill",
				NodeRef:  skillID,
				Params: map[string]interface{}{
					"action":       "create",
					"user_message": "创建标题为'测试模板'的条目，描述是验证CRUD，内容是测试文本",
				},
			},
		},
	}
	plan = engine.applyRuntimeParamState(ctx, plan)
	missing := engine.missingRequiredArgsForPlan(ctx, plan)
	if len(missing) != 0 {
		t.Fatalf("missing=%v plan=%#v", missing, plan.Tasks[0].Params)
	}
	template, ok := plan.Tasks[0].Params["template"].(map[string]interface{})
	if !ok {
		t.Fatalf("template params missing: %#v", plan.Tasks[0].Params)
	}
	if template["title"] != "测试模板" {
		t.Fatalf("title=%#v params=%#v", template["title"], plan.Tasks[0].Params)
	}
	if template["description"] != "验证CRUD" {
		t.Fatalf("description=%#v params=%#v", template["description"], plan.Tasks[0].Params)
	}
	if template["content"] != "测试文本" {
		t.Fatalf("content=%#v params=%#v", template["content"], plan.Tasks[0].Params)
	}
}

func TestExtractSlotValueFromTextHandlesQuotedChineseValue(t *testing.T) {
	got := extractSlotValueFromText("创建标题为'测试模板'的条目，描述是验证CRUD，内容是测试文本", []string{"标题"})
	if got != "测试模板" {
		t.Fatalf("title=%q", got)
	}
}

func TestResponsePlannerPendingTaskResumeExecutesWithUserMessage(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "内容是：这是一条测试用的模板内容",
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "powerxplugin.template.basic.local", Title: "模板能力"},
		},
		PendingTask: map[string]any{
			"status":         "awaiting_params",
			"node_ref":       "powerxplugin.template.basic.local",
			"skill_id":       "powerxplugin.template.basic.local",
			"action":         "create",
			"missing_fields": []any{"template.content"},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeSkillExecution {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if !hasResponseIntent(plan.ResponseIntents, ResponseIntentSkillExecution) {
		t.Fatalf("missing skill execution intent: %#v", plan.ResponseIntents)
	}
	if !plan.ShouldCallTool {
		t.Fatalf("pending task resume must call tool")
	}
	if len(plan.TargetCapabilityIDs) != 1 || plan.TargetCapabilityIDs[0] != "powerxplugin.template.basic.local" {
		t.Fatalf("target ids=%v", plan.TargetCapabilityIDs)
	}
}

func TestClarifyParamsFallbackUsesUserFacingLanguage(t *testing.T) {
	plan := &ResponsePlan{
		ResponseMode:  ResponseModeClarifyParams,
		MissingFields: []string{"template.name", "template.description", "template.content"},
	}
	content := BuildFinalResponseContent(plan, "", nil)
	for _, forbidden := range []string{"template.name", "template.description", "template.content", "JSON"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("fallback exposed technical field %q in %s", forbidden, content)
		}
	}
	for _, want := range []string{"名称", "描述", "内容", "自然语言"} {
		if !strings.Contains(content, want) {
			t.Fatalf("fallback missing user-facing text %q in %s", want, content)
		}
	}
}

func TestSkillExecutionFallbackDoesNotClaimCompletionWithoutResult(t *testing.T) {
	content := BuildFinalResponseContent(&ResponsePlan{ResponseMode: ResponseModeSkillExecution}, "", nil)
	for _, forbidden := range []string{"已创建", "已完成", "请稍候", "大约", "正在创建"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("fallback claimed execution state %q in %s", forbidden, content)
		}
	}
	for _, want := range []string{"没有收到", "执行结果", "不能确认"} {
		if !strings.Contains(content, want) {
			t.Fatalf("fallback missing safe wording %q in %s", want, content)
		}
	}
}

func TestNormalChatFiltersBusinessCompletionWithoutExecutionResult(t *testing.T) {
	content := BuildFinalResponseContent(&ResponsePlan{ResponseMode: ResponseModeNormalChat}, "已为您创建标题为“测试模板”的模板，操作已完成。", nil)
	for _, forbidden := range []string{"已为您创建", "操作已完成", "创建成功"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("normal chat claimed execution state %q in %s", forbidden, content)
		}
	}
	for _, want := range []string{"没有生成", "执行结果", "不能确认"} {
		if !strings.Contains(content, want) {
			t.Fatalf("safe fallback missing %q in %s", want, content)
		}
	}
}

func TestResponsePlannerKeepsClarifyParamsWhenRuntimeNeedsMetadata(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage: "那我现在想要创建一个模板",
		AllowedCapabilities: []CapabilityContextItem{
			{
				ID:           "powerxplugin.template.basic",
				Title:        "模板能力",
				RequiredArgs: []string{"action"},
				OptionalArgs: []string{"template"},
				Actions:      []string{"create", "get", "update", "delete", "list"},
				ResponseGuidance: []string{
					"clarify_params: 用户只说创建模板时，追问模板名称、用途描述和模板内容。",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeClarifyParams {
		t.Fatalf("mode=%s reason=%s", plan.ResponseMode, plan.Reason)
	}
	if !plan.UseCapabilityCtx || !plan.IncludeExamples || !plan.IncludeSchema {
		t.Fatalf("expected clarify mode to include capability context: %#v", plan)
	}
	if plan.ShouldCallTool {
		t.Fatalf("clarify mode should not call tool")
	}
}

func TestRuntimeExecutesExistingPlanEvenWhenResponsePlannerDoesNotCallTool(t *testing.T) {
	responsePlan := &ResponsePlan{
		ResponseMode:   ResponseModeClarifyParams,
		ShouldCallTool: false,
	}
	execPlan := &flowschema.ExecutionPlan{
		PlanID: "plan_skill_prepare",
		Tasks: []flowschema.PlanTask{
			{
				TaskID:   "task_skill_prepare",
				FlowID:   "powerxplugin.template.basic",
				NodeKind: "skill",
				NodeRef:  "powerxplugin.template.basic",
			},
		},
	}
	if !shouldExecutePlanForResponse(responsePlan, execPlan) {
		t.Fatalf("runtime must execute Tool/Skill Planner output; Response Planner only controls final wording")
	}
}

func TestResponsePlannerExecutableOverridesChat(t *testing.T) {
	plan, err := NewResponsePlanner().Plan(context.Background(), ResponsePlanInput{
		UserMessage:           "准备发布报告",
		PlanHasExecutableNode: true,
		AllowedCapabilities: []CapabilityContextItem{
			{ID: "powerx.release.report_synthesis"},
		},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if plan.ResponseMode != ResponseModeSkillExecution {
		t.Fatalf("mode=%s", plan.ResponseMode)
	}
	if !plan.ShouldCallTool {
		t.Fatalf("expected should_call_tool")
	}
}

func hasResponseIntent(values []ResponseIntent, target ResponseIntent) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
