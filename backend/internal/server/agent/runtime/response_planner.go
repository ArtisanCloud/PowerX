package runtime

import (
	"context"
	"fmt"
	"strings"
)

type ResponsePlanner struct{}

func NewResponsePlanner() *ResponsePlanner {
	return &ResponsePlanner{}
}

func (p *ResponsePlanner) Plan(_ context.Context, in ResponsePlanInput) (*ResponsePlan, error) {
	allowedIDs := capabilityIDs(in.AllowedCapabilities)
	mode := ResponseModeNormalChat
	reason := "normal chat"
	useCapabilityContext := false
	includeExamples := false
	includeSchema := false
	needsClarification := false
	shouldCallTool := in.PlanHasExecutableNode
	missingFields := []string(nil)
	targetIDs := []string(nil)
	intents := classifyResponseIntents(in.UserMessage)
	answerRequirements := answerRequirementsForIntents(intents)

	if in.ExecutionFailed {
		mode = ResponseModeErrorExplain
		reason = "execution failed"
		intents = appendIntent(intents, ResponseIntentErrorExplain)
	} else if in.ExecutionCompleted || in.PlanHasExecutableNode {
		mode = ResponseModeSkillExecution
		reason = "planner produced executable task"
		targetIDs = allowedIDs
		intents = appendIntent(intents, ResponseIntentSkillExecution)
	} else if pendingTaskAwaitingParams(in.PendingTask) {
		intents = appendIntent(intents, ResponseIntentClarifyParams)
		if strings.TrimSpace(in.UserMessage) == "" {
			mode = ResponseModeClarifyParams
			reason = "pending task is awaiting parameters"
			useCapabilityContext = true
			includeExamples = true
			includeSchema = true
			needsClarification = true
			missingFields = pendingTaskMissingFields(in.PendingTask)
		} else {
			mode = ResponseModeSkillExecution
			reason = "user supplied parameters for pending task"
			intents = appendIntent(intents, ResponseIntentSkillExecution)
			shouldCallTool = true
		}
		targetIDs = pendingTaskTargetCapabilityIDs(in.PendingTask, allowedIDs)
	} else {
		switch primaryResponseMode(intents) {
		case ResponseModeCapabilityHowTo:
			mode = ResponseModeCapabilityHowTo
			reason = "user asks how to use a capability"
			useCapabilityContext = true
			includeExamples = true
			includeSchema = true
			if len(in.AllowedCapabilities) > 0 {
				targetIDs = firstCapabilityID(allowedIDs)
				if len(targetIDs) == 0 {
					targetIDs = allowedIDs
				}
			}
		case ResponseModeCapabilityIntro:
			mode = ResponseModeCapabilityIntro
			reason = "user asks current agent identity or capabilities"
			useCapabilityContext = true
			includeExamples = true
			if len(in.AllowedCapabilities) > 0 {
				targetIDs = allowedIDs
			}
		case ResponseModeClarifyParams:
			if len(in.AllowedCapabilities) > 0 {
				mode = ResponseModeClarifyParams
				reason = "user asks to execute but required parameters are missing"
				useCapabilityContext = true
				includeExamples = true
				includeSchema = true
				targetIDs = firstCapabilityID(allowedIDs)
				missingFields = missingRequiredFields(in.UserMessage, in.AllowedCapabilities)
				needsClarification = len(missingFields) > 0
				if !needsClarification {
					reason = "user asks to execute but runtime needs capability metadata to clarify parameters"
				}
			}
		}
	}
	if mode == ResponseModeNormalChat && len(intents) == 0 {
		intents = []ResponseIntent{ResponseIntentNormalChat}
		answerRequirements = answerRequirementsForIntents(intents)
	}
	if len(answerRequirements) == 0 {
		answerRequirements = answerRequirementsForMode(mode)
	}

	plan := &ResponsePlan{
		TenantUUID:            strings.TrimSpace(in.TenantUUID),
		AgentID:               strings.TrimSpace(in.AgentID),
		SessionID:             strings.TrimSpace(in.SessionID),
		MessageID:             strings.TrimSpace(in.MessageID),
		RunID:                 strings.TrimSpace(in.RunID),
		TraceID:               strings.TrimSpace(in.TraceID),
		ResponseMode:          mode,
		ResponseIntents:       intents,
		AnswerRequirements:    answerRequirements,
		ShouldCallTool:        shouldCallTool,
		TargetCapabilityIDs:   targetIDs,
		UseCapabilityCtx:      useCapabilityContext,
		IncludeExamples:       includeExamples,
		IncludeSchema:         includeSchema,
		RepeatFullIntro:       mode == ResponseModeCapabilityIntro && !in.RecentCapabilityIntro,
		RecentCapabilityIntro: in.RecentCapabilityIntro,
		NeedsClarification:    needsClarification,
		MissingFields:         missingFields,
		Reason:                reason,
		ModelSelection:        in.ModelSelection,
	}
	if err := plan.Validate(allowedIDs); err != nil {
		return nil, err
	}
	return plan, nil
}

func pendingTaskAwaitingParams(task map[string]any) bool {
	if len(task) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(task["status"])), "awaiting_params")
}

func pendingTaskMissingCount(task map[string]any) int {
	return len(pendingTaskMissingFields(task))
}

func pendingTaskMissingFields(task map[string]any) []string {
	if len(task) == 0 {
		return nil
	}
	return normalizeStringList(anyStringSlice(task["missing_fields"]))
}

func pendingTaskTargetCapabilityIDs(task map[string]any, allowedIDs []string) []string {
	if len(task) == 0 {
		return nil
	}
	target := strings.TrimSpace(firstNonEmpty(
		fmt.Sprint(task["skill_id"]),
		fmt.Sprint(task["node_ref"]),
	))
	if target == "" || target == "<nil>" {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, id := range allowedIDs {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	if _, ok := allowed[strings.ToLower(target)]; ok {
		return []string{target}
	}
	return nil
}

func anyStringSlice(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return out
	default:
		return nil
	}
}

func classifyResponseIntents(message string) []ResponseIntent {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return nil
	}
	out := make([]ResponseIntent, 0, 4)
	if containsAny(msg, "你好", "您好", "hello", "hi ") || msg == "hi" || msg == "hello" {
		out = appendIntent(out, ResponseIntentGreeting)
	}
	if containsAny(msg, "你是谁", "是什么智能体", "什么智能体", "介绍一下", "who are you") {
		out = appendIntent(out, ResponseIntentAgentIdentity)
	}
	if containsAny(msg, "能做什么", "有什么能力", "有哪些能力", "已绑定", "bound skills", "bound capabilities", "what can you do", "capabilities") {
		out = appendIntent(out, ResponseIntentCapabilityIntro)
	}
	if containsAny(msg, "怎么用", "如何使用", "怎么调用", "需要什么", "第一个能力", "how to", "how do i", "usage") {
		out = appendIntent(out, ResponseIntentCapabilityHowTo)
	}
	if containsAny(msg, "建议", "先测试", "测试哪", "先用哪个", "先试", "recommend", "suggest") {
		out = appendIntent(out, ResponseIntentTestRecommendation)
	}
	if containsAny(msg, "帮我", "创建", "新建", "更新", "删除", "查询", "执行", "生成", "安排", "准备", "create", "update", "delete", "list", "run", "generate") {
		out = appendIntent(out, ResponseIntentClarifyParams)
	}
	return normalizeResponseIntents(out)
}

func primaryResponseMode(intents []ResponseIntent) ResponseMode {
	has := func(target ResponseIntent) bool {
		for _, intent := range intents {
			if intent == target {
				return true
			}
		}
		return false
	}
	switch {
	case has(ResponseIntentClarifyParams):
		return ResponseModeClarifyParams
	case has(ResponseIntentCapabilityHowTo):
		return ResponseModeCapabilityHowTo
	case has(ResponseIntentAgentIdentity), has(ResponseIntentCapabilityIntro), has(ResponseIntentTestRecommendation):
		return ResponseModeCapabilityIntro
	default:
		return ResponseModeNormalChat
	}
}

func answerRequirementsForIntents(intents []ResponseIntent) []string {
	out := make([]string, 0, len(intents)+1)
	for _, intent := range normalizeResponseIntents(intents) {
		switch intent {
		case ResponseIntentGreeting:
			out = append(out, "简短回应用户问候。")
		case ResponseIntentAgentIdentity:
			out = append(out, "说明当前 Agent 的身份和服务对象。")
		case ResponseIntentCapabilityIntro:
			out = append(out, "只列出当前 Agent 已绑定能力，不能列出全局平台能力或未绑定能力。")
		case ResponseIntentCapabilityHowTo:
			out = append(out, "说明目标能力如何使用、需要哪些信息。")
		case ResponseIntentClarifyParams:
			out = append(out, "如果用户要执行但缺少必要参数，先追问缺失信息。")
		case ResponseIntentTestRecommendation:
			out = append(out, "基于当前已绑定能力推荐一个最适合先测试的能力或动作。")
		case ResponseIntentSkillExecution:
			out = append(out, "总结本轮执行状态和下一步。")
		case ResponseIntentErrorExplain:
			out = append(out, "解释本轮错误并给出可操作下一步。")
		case ResponseIntentNormalChat:
			out = append(out, "直接回答用户当前问题。")
		}
	}
	return normalizeStringList(out)
}

func answerRequirementsForMode(mode ResponseMode) []string {
	switch mode {
	case ResponseModeCapabilityIntro:
		return []string{"说明当前 Agent 身份，并只基于当前已绑定能力回答。"}
	case ResponseModeCapabilityHowTo:
		return []string{"说明目标能力如何使用、需要哪些信息。"}
	case ResponseModeClarifyParams:
		return []string{"询问用户补充缺失的必要参数。"}
	case ResponseModeSkillExecution:
		return []string{"总结执行状态和下一步。"}
	case ResponseModeErrorExplain:
		return []string{"解释错误并给出可操作下一步。"}
	default:
		return []string{"直接回答用户当前问题。"}
	}
}

func appendIntent(values []ResponseIntent, next ResponseIntent) []ResponseIntent {
	next = normalizeResponseIntent(next)
	if next == "" {
		return values
	}
	for _, value := range values {
		if normalizeResponseIntent(value) == next {
			return values
		}
	}
	return append(values, next)
}

func containsAny(msg string, hints ...string) bool {
	for _, hint := range hints {
		if strings.Contains(msg, strings.ToLower(strings.TrimSpace(hint))) {
			return true
		}
	}
	return false
}

func capabilityIDs(items []CapabilityContextItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out = append(out, id)
		}
	}
	return normalizeStringList(out)
}

func firstCapabilityID(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	return []string{ids[0]}
}

func missingRequiredFields(message string, caps []CapabilityContextItem) []string {
	if len(caps) == 0 {
		return nil
	}
	msg := strings.ToLower(message)
	missing := []string{}
	for _, field := range caps[0].RequiredArgs {
		f := strings.TrimSpace(field)
		if f == "" {
			continue
		}
		if strings.Contains(msg, strings.ToLower(f)) {
			continue
		}
		if strings.EqualFold(f, "action") {
			continue
		}
		missing = append(missing, f)
	}
	return normalizeStringList(missing)
}
