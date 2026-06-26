package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ResponseMode string

const (
	ResponseModeCapabilityIntro ResponseMode = "capability_intro"
	ResponseModeCapabilityHowTo ResponseMode = "capability_howto"
	ResponseModeSkillExecution  ResponseMode = "skill_execution"
	ResponseModeClarifyParams   ResponseMode = "clarify_params"
	ResponseModeNormalChat      ResponseMode = "normal_chat"
	ResponseModeErrorExplain    ResponseMode = "error_explain"
)

type ResponseIntent string

const (
	ResponseIntentGreeting           ResponseIntent = "greeting"
	ResponseIntentAgentIdentity      ResponseIntent = "agent_identity"
	ResponseIntentCapabilityIntro    ResponseIntent = "capability_intro"
	ResponseIntentCapabilityHowTo    ResponseIntent = "capability_howto"
	ResponseIntentSkillExecution     ResponseIntent = "skill_execution"
	ResponseIntentClarifyParams      ResponseIntent = "clarify_params"
	ResponseIntentTestRecommendation ResponseIntent = "test_recommendation"
	ResponseIntentNormalChat         ResponseIntent = "normal_chat"
	ResponseIntentErrorExplain       ResponseIntent = "error_explain"
)

const (
	ErrCodeResponsePlanInvalid     = "agent.response_plan_invalid"
	ErrCodeContextCapabilityDenied = "agent.context_capability_denied"
)

type ResponsePlan struct {
	ResponsePlanID        string             `json:"response_plan_id"`
	TenantUUID            string             `json:"tenant_uuid,omitempty"`
	AgentID               string             `json:"agent_id,omitempty"`
	SessionID             string             `json:"session_id,omitempty"`
	MessageID             string             `json:"message_id,omitempty"`
	RunID                 string             `json:"run_id,omitempty"`
	TraceID               string             `json:"trace_id,omitempty"`
	ResponseMode          ResponseMode       `json:"response_mode"`
	ResponseIntents       []ResponseIntent   `json:"response_intents,omitempty"`
	AnswerRequirements    []string           `json:"answer_requirements,omitempty"`
	ShouldCallTool        bool               `json:"should_call_tool"`
	TargetCapabilityIDs   []string           `json:"target_capability_ids"`
	UseCapabilityCtx      bool               `json:"use_capability_context"`
	IncludeExamples       bool               `json:"include_examples"`
	IncludeSchema         bool               `json:"include_schema"`
	RepeatFullIntro       bool               `json:"repeat_full_intro"`
	RecentCapabilityIntro bool               `json:"recent_capability_intro"`
	NeedsClarification    bool               `json:"needs_clarification"`
	MissingFields         []string           `json:"missing_fields"`
	Reason                string             `json:"reason,omitempty"`
	ModelSelection        NodeModelSelection `json:"model_selection,omitempty"`
	CreatedAt             time.Time          `json:"created_at,omitempty"`
}

type AssistantMessageMeta struct {
	ResponseMode       ResponseMode        `json:"response_mode,omitempty"`
	CapabilityIDs      []string            `json:"capability_ids,omitempty"`
	ResponsePlanID     string              `json:"response_plan_id,omitempty"`
	UsedContextLayers  []string            `json:"used_context_layers,omitempty"`
	ToolCalls          []map[string]any    `json:"tool_calls,omitempty"`
	FinalResponseModel string              `json:"final_response_model,omitempty"`
	ModelSelection     *NodeModelSelection `json:"model_selection,omitempty"`
	TraceID            string              `json:"trace_id,omitempty"`
	RunID              string              `json:"run_id,omitempty"`
	PlanID             string              `json:"plan_id,omitempty"`
}

type ResponsePlanInput struct {
	UserMessage           string
	PlanHasExecutableNode bool
	ExecutionCompleted    bool
	ExecutionFailed       bool
	ErrorSummary          string
	AllowedCapabilities   []CapabilityContextItem
	RecentCapabilityIntro bool
	PendingTask           map[string]any
	ModelSelection        NodeModelSelection
	TenantUUID            string
	AgentID               string
	SessionID             string
	MessageID             string
	RunID                 string
	TraceID               string
	PlanID                string
}

type CapabilityContextItem struct {
	ID                 string              `json:"id"`
	Title              string              `json:"title,omitempty"`
	Description        string              `json:"description,omitempty"`
	RequiredArgs       []string            `json:"required_args,omitempty"`
	ActionRequiredArgs map[string][]string `json:"action_required_args,omitempty"`
	ActionOptionalArgs map[string][]string `json:"action_optional_args,omitempty"`
	SlotMapping        map[string]any      `json:"slot_mapping,omitempty"`
	PendingTaskPolicy  map[string]any      `json:"pending_task_policy,omitempty"`
	StateContract      map[string]any      `json:"state_contract,omitempty"`
	ResultPresentation map[string]any      `json:"result_presentation,omitempty"`
	OptionalArgs       []string            `json:"optional_args,omitempty"`
	Actions            []string            `json:"actions,omitempty"`
	Examples           []string            `json:"examples,omitempty"`
	ResponseGuidance   []string            `json:"response_guidance,omitempty"`
	NodeKind           string              `json:"node_kind,omitempty"`
	Source             string              `json:"source,omitempty"`
}

var errResponsePlanInvalid = errors.New(ErrCodeResponsePlanInvalid)

func (p *ResponsePlan) Validate(allowedIDs []string) error {
	if p == nil {
		return fmt.Errorf("%w: response plan is nil", errResponsePlanInvalid)
	}
	p.ResponseMode = normalizeResponseMode(p.ResponseMode)
	if p.ResponseMode == "" {
		return fmt.Errorf("%w: response_mode is required", errResponsePlanInvalid)
	}
	p.ResponseIntents = normalizeResponseIntents(p.ResponseIntents)
	p.AnswerRequirements = normalizeStringList(p.AnswerRequirements)
	p.TargetCapabilityIDs = normalizeStringList(p.TargetCapabilityIDs)
	p.MissingFields = normalizeStringList(p.MissingFields)
	allowed := map[string]struct{}{}
	for _, id := range normalizeStringList(allowedIDs) {
		allowed[strings.ToLower(id)] = struct{}{}
	}
	for _, id := range p.TargetCapabilityIDs {
		if len(allowed) == 0 {
			return fmt.Errorf("%s: capability %s is not allowed", ErrCodeContextCapabilityDenied, id)
		}
		if _, ok := allowed[strings.ToLower(id)]; !ok {
			return fmt.Errorf("%s: capability %s is not allowed", ErrCodeContextCapabilityDenied, id)
		}
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	if strings.TrimSpace(p.ResponsePlanID) == "" {
		p.ResponsePlanID = fmt.Sprintf("rp_%d", time.Now().UnixNano())
	}
	return nil
}

func (p ResponsePlan) ToDebugEvent() map[string]any {
	return map[string]any{
		"response_plan_id":        p.ResponsePlanID,
		"response_mode":           p.ResponseMode,
		"response_intents":        p.ResponseIntents,
		"answer_requirements":     p.AnswerRequirements,
		"should_call_tool":        p.ShouldCallTool,
		"target_capability_ids":   p.TargetCapabilityIDs,
		"use_capability_context":  p.UseCapabilityCtx,
		"include_examples":        p.IncludeExamples,
		"include_schema":          p.IncludeSchema,
		"repeat_full_intro":       p.RepeatFullIntro,
		"recent_capability_intro": p.RecentCapabilityIntro,
		"needs_clarification":     p.NeedsClarification,
		"missing_fields":          p.MissingFields,
		"reason":                  p.Reason,
		"model_selection":         p.ModelSelection,
	}
}

func (p ResponsePlan) ToContextValue() map[string]any {
	raw, _ := json.Marshal(p)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func normalizeResponseMode(mode ResponseMode) ResponseMode {
	switch ResponseMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case ResponseModeCapabilityIntro:
		return ResponseModeCapabilityIntro
	case ResponseModeCapabilityHowTo:
		return ResponseModeCapabilityHowTo
	case ResponseModeSkillExecution:
		return ResponseModeSkillExecution
	case ResponseModeClarifyParams:
		return ResponseModeClarifyParams
	case ResponseModeNormalChat:
		return ResponseModeNormalChat
	case ResponseModeErrorExplain:
		return ResponseModeErrorExplain
	default:
		return ""
	}
}

func normalizeResponseIntents(values []ResponseIntent) []ResponseIntent {
	if len(values) == 0 {
		return nil
	}
	seen := map[ResponseIntent]struct{}{}
	out := make([]ResponseIntent, 0, len(values))
	for _, value := range values {
		intent := normalizeResponseIntent(value)
		if intent == "" {
			continue
		}
		if _, ok := seen[intent]; ok {
			continue
		}
		seen[intent] = struct{}{}
		out = append(out, intent)
	}
	return out
}

func normalizeResponseIntent(intent ResponseIntent) ResponseIntent {
	switch ResponseIntent(strings.ToLower(strings.TrimSpace(string(intent)))) {
	case ResponseIntentGreeting:
		return ResponseIntentGreeting
	case ResponseIntentAgentIdentity:
		return ResponseIntentAgentIdentity
	case ResponseIntentCapabilityIntro:
		return ResponseIntentCapabilityIntro
	case ResponseIntentCapabilityHowTo:
		return ResponseIntentCapabilityHowTo
	case ResponseIntentSkillExecution:
		return ResponseIntentSkillExecution
	case ResponseIntentClarifyParams:
		return ResponseIntentClarifyParams
	case ResponseIntentTestRecommendation:
		return ResponseIntentTestRecommendation
	case ResponseIntentNormalChat:
		return ResponseIntentNormalChat
	case ResponseIntentErrorExplain:
		return ResponseIntentErrorExplain
	default:
		return ""
	}
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	return out
}
