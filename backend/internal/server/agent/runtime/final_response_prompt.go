package runtime

import (
	"fmt"
	"strings"
)

func BuildModeSpecificSystemPrompt(base string, plan *ResponsePlan) string {
	base = strings.TrimSpace(base)
	if plan == nil {
		return base
	}
	modePrompt := finalResponseModePrompt(plan.ResponseMode)
	if modePrompt == "" {
		return base
	}
	if base == "" {
		return modePrompt
	}
	return strings.TrimSpace(base + "\n\n[FINAL_RESPONSE_MODE]\n" + modePrompt)
}

func BuildModeSpecificUserPrompt(userMessage string, plan *ResponsePlan) string {
	userMessage = strings.TrimSpace(userMessage)
	if plan == nil {
		return userMessage
	}
	mode := normalizeResponseMode(plan.ResponseMode)
	if mode == "" {
		return userMessage
	}
	parts := []string{
		"[CURRENT_USER_MESSAGE]",
		userMessage,
		"",
		"[RESPONSE_PLAN]",
		fmt.Sprintf("response_mode: %s", mode),
		fmt.Sprintf("repeat_full_intro: %t", plan.RepeatFullIntro),
		fmt.Sprintf("recent_capability_intro: %t", plan.RecentCapabilityIntro),
	}
	if len(plan.TargetCapabilityIDs) > 0 {
		parts = append(parts, "target_capability_ids: "+strings.Join(plan.TargetCapabilityIDs, ", "))
	}
	if len(plan.ResponseIntents) > 0 {
		intentNames := make([]string, 0, len(plan.ResponseIntents))
		for _, intent := range plan.ResponseIntents {
			if normalized := normalizeResponseIntent(intent); normalized != "" {
				intentNames = append(intentNames, string(normalized))
			}
		}
		if len(intentNames) > 0 {
			parts = append(parts, "response_intents: "+strings.Join(intentNames, ", "))
		}
	}
	if len(plan.AnswerRequirements) > 0 {
		parts = append(parts, "answer_requirements:")
		for _, req := range plan.AnswerRequirements {
			if trimmed := strings.TrimSpace(req); trimmed != "" {
				parts = append(parts, "- "+trimmed)
			}
		}
	}
	if len(plan.MissingFields) > 0 {
		parts = append(parts, "missing_fields: "+strings.Join(plan.MissingFields, ", "))
	}
	parts = append(parts, "", "[ANSWER_TASK]")
	switch mode {
	case ResponseModeCapabilityIntro:
		if plan.RepeatFullIntro {
			parts = append(parts, "回答用户当前询问的 Agent 定位和已绑定能力。只基于系统上下文中的当前 Agent 能力，不要输出内部 ID 或 schema。")
		} else {
			parts = append(parts, "用户正在重复询问能力。请避免重复上一轮已讲过的长篇背景，但必须完整回应本轮用户明确提出的要求；具体表达方式、业务重点和示例应遵循当前 Agent persona、prompt seed 与 Skill metadata。")
		}
	case ResponseModeCapabilityHowTo:
		parts = append(parts, "回答用户追问的能力使用方法。说明需要用户提供什么信息，并给自然语言示例；不要重新介绍全部能力。")
	case ResponseModeClarifyParams:
		parts = append(parts, "用户想执行任务但缺少必要信息。请只询问缺失信息，不要把它当成执行失败。")
		parts = append(parts, "用当前 Agent 和 Skill metadata 中的业务术语提问，不要要求用户输入 JSON、schema、字段路径或内部参数名。")
		parts = append(parts, "如果缺失字段包含对象字段，请转成普通用户能理解的信息项，例如名称、描述、内容、ID、筛选条件。")
	case ResponseModeSkillExecution:
		parts = append(parts, "根据执行结果回答用户本轮请求的状态和下一步。不要暴露内部 trace、skill id、stack。")
	case ResponseModeErrorExplain:
		parts = append(parts, "把本轮执行错误解释成人能理解的话，并给出可操作下一步。不要暴露内部实现细节。")
	case ResponseModeNormalChat:
		parts = append(parts, "直接回答用户当前问题。不要主动重复介绍能力，除非用户明确询问。")
		parts = append(parts, "如果本轮没有 tool/skill 执行结果，不得声称已经创建、更新、删除、发布、同步或完成任何业务对象；只能说明尚未执行，并引导用户补齐信息或确认执行。")
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func BuildFinalResponseContent(plan *ResponsePlan, rawContent string, execErr error) string {
	content := SanitizeAssistantVisibleText(rawContent)
	if plan == nil {
		if content != "" {
			return content
		}
		return "任务已执行完成。"
	}
	switch plan.ResponseMode {
	case ResponseModeErrorExplain:
		if content != "" {
			return content
		}
		if execErr != nil {
			return humanizeExecutionError(execErr)
		}
		return "执行失败，请检查输入后重试。"
	case ResponseModeClarifyParams:
		if content != "" {
			return content
		}
		if len(plan.MissingFields) > 0 {
			return fmt.Sprintf("我还需要你补充这些信息：%s。请直接用自然语言说明。", strings.Join(humanizeMissingFields(plan.MissingFields), "、"))
		}
		return "我还需要你补充必要信息后才能继续。请直接用自然语言说明。"
	case ResponseModeSkillExecution:
		if content != "" {
			return content
		}
		return "本轮没有收到技能或能力的执行结果，因此不能确认任务已经完成。请查看运行跟踪，确认是否已生成 Skill/Capability 执行节点。"
	default:
		if content != "" {
			if plan.ResponseMode == ResponseModeNormalChat && claimsBusinessCompletion(content) {
				return "本轮没有生成技能或能力执行结果，因此不能确认任务已经完成。请重新发送明确的执行指令，或在运行跟踪中确认是否出现 Skill/Capability 执行节点。"
			}
			return content
		}
		return "本轮没有生成技能或能力执行结果，因此不能确认任务已经完成。"
	}
}

func claimsBusinessCompletion(content string) bool {
	normalized := strings.TrimSpace(content)
	if normalized == "" {
		return false
	}
	completionWords := []string{
		"已创建",
		"创建成功",
		"成功创建",
		"已成功创建",
		"已更新",
		"更新成功",
		"已删除",
		"删除成功",
		"已发布",
		"发布成功",
		"已同步",
		"同步成功",
		"已完成",
		"操作已完成",
		"任务已执行完成",
	}
	for _, word := range completionWords {
		if strings.Contains(normalized, word) {
			return true
		}
	}
	return false
}

func finalResponseModePrompt(mode ResponseMode) string {
	switch mode {
	case ResponseModeCapabilityIntro:
		return strings.Join([]string{
			"你正在回答用户关于当前 Agent 能力的问题。",
			"只基于 [CONTEXT-L1 CAPABILITIES] 中的当前已绑定能力回答。",
			"不要输出机器 ID、ref、schema 字段名、executor path。",
			"先一句话说明当前 Agent 的定位，再用简短分点说明能力。",
			"每项能力给 1-2 个用户可以直接说的示例。",
			"如果上下文提示最近已经完整介绍过能力，仍需完整回应用户本轮明确要求；不要重复上一轮长篇背景，具体业务表述以当前 Agent persona、prompt seed 与 Skill metadata 为准。",
		}, "\n")
	case ResponseModeCapabilityHowTo:
		return strings.Join([]string{
			"用户在追问某项能力如何使用。",
			"只说明目标能力需要哪些信息、支持哪些动作，以及用户可以怎么说。",
			"不要重新完整介绍所有能力。",
			"如果缺少必要参数，用问题引导用户补充。",
		}, "\n")
	case ResponseModeSkillExecution:
		return strings.Join([]string{
			"用户请求已经执行或正在执行。",
			"请总结执行结果：成功、失败、排队或部分成功。",
			"成功时告诉用户结果和下一步；失败时给出可操作建议。",
			"不要暴露内部 trace、skill id、stack 或 executor path。",
		}, "\n")
	case ResponseModeClarifyParams:
		return strings.Join([]string{
			"用户想执行任务，但缺少必要参数。",
			"请基于缺失字段自然地向用户提问，使用业务术语，不要输出字段路径或 schema 名称。",
			"不要要求用户输入 JSON；只有用户主动要求技术格式时，才可以给结构化示例。",
			"不要把缺参数当成执行失败。",
		}, "\n")
	case ResponseModeErrorExplain:
		return strings.Join([]string{
			"执行链路发生错误。",
			"请把错误转成用户能理解的话，并给出可操作下一步。",
			"不要暴露 stack、raw payload、prompt 或敏感上下文。",
		}, "\n")
	case ResponseModeNormalChat:
		return strings.Join([]string{
			"正常对话。不要重复介绍能力，除非用户明确询问。",
			"没有 tool/skill 执行结果时，不得声称已经创建、更新、删除、发布、同步或完成任何业务对象。",
		}, "\n")
	default:
		return ""
	}
}

func humanizeMissingFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		label := humanizeMissingField(field)
		if label == "" {
			continue
		}
		out = append(out, label)
	}
	return out
}

func humanizeMissingField(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	normalized := strings.ToLower(strings.ReplaceAll(field, "-", "_"))
	if idx := strings.LastIndex(normalized, "."); idx >= 0 && idx < len(normalized)-1 {
		normalized = normalized[idx+1:]
	}
	switch normalized {
	case "title":
		return "标题"
	case "content":
		return "内容"
	case "name":
		return "名称"
	case "description", "desc":
		return "描述"
	case "id":
		return "ID"
	case "q", "keyword", "query":
		return "查询关键词"
	case "action":
		return "要执行的操作"
	default:
		parts := strings.FieldsFunc(normalized, func(r rune) bool {
			return r == '.' || r == '_' || r == '/'
		})
		if len(parts) == 0 {
			return field
		}
		return strings.Join(parts, " ")
	}
}
