package skills

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ManifestLLMInvocation and its siblings are intentionally narrow platform
// ports. They describe *how* to call an executor without embedding a business
// skill identifier in Core.
type ManifestLLMInvocation struct {
	TenantUUID     string
	TraceID        string
	SkillID        string
	Version        string
	Entrypoint     string
	PromptTemplate string
	ModelPolicy    map[string]any
	Payload        map[string]any
	Context        map[string]any
}

type ManifestCapabilityInvocation struct {
	TenantUUID   string
	TraceID      string
	SkillID      string
	Version      string
	Entrypoint   string
	CapabilityID string
	Payload      map[string]any
	Context      map[string]any
}

type ManifestWorkflowInvocation struct {
	TenantUUID   string
	TraceID      string
	SkillID      string
	Version      string
	Entrypoint   string
	WorkflowUUID string
	Payload      map[string]any
	Context      map[string]any
}

type ManifestLLMInvoker func(context.Context, ManifestLLMInvocation) (string, error)
type ManifestCapabilityInvoker func(context.Context, ManifestCapabilityInvocation) (map[string]any, error)
type ManifestWorkflowInvoker func(context.Context, ManifestWorkflowInvocation) (map[string]any, error)

type ManifestExecutorOptions struct {
	LLM        ManifestLLMInvoker
	Capability ManifestCapabilityInvoker
	Workflow   ManifestWorkflowInvoker
}

// ManifestExecutor is the generic Skill dispatcher. It selects exclusively by
// `definition.executor.type`; skill IDs, agent names and team keys are never
// part of its routing decision.
type ManifestExecutor struct {
	llm        ManifestLLMInvoker
	capability ManifestCapabilityInvoker
	workflow   ManifestWorkflowInvoker
}

func NewManifestExecutor(options ManifestExecutorOptions) *ManifestExecutor {
	return &ManifestExecutor{
		llm:        options.LLM,
		capability: options.Capability,
		workflow:   options.Workflow,
	}
}

func (e *ManifestExecutor) CanHandle(in ExecuteInput) bool {
	_, ok := normalizedExecutorType(in.Manifest)
	return ok
}

func (e *ManifestExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]any, error) {
	typ, ok := normalizedExecutorType(in.Manifest)
	if !ok {
		return nil, errors.New("skill.executor_definition_invalid")
	}
	executor := nestedManifestMap(in.Manifest, "executor")
	switch typ {
	case "llm_prompt":
		if e == nil || e.llm == nil {
			return nil, errors.New("skill.executor_llm_unavailable")
		}
		prompt, err := localizedExecutorPrompt(executor, in.Context)
		if err != nil {
			return nil, err
		}
		text, err := e.llm(ctx, ManifestLLMInvocation{
			TenantUUID:     in.TenantUUID,
			TraceID:        in.TraceID,
			SkillID:        in.SkillID,
			Version:        in.Version,
			Entrypoint:     in.Entrypoint,
			PromptTemplate: prompt,
			ModelPolicy:    nestedManifestMap(executor, "model_policy"),
			Payload:        in.Payload,
			Context:        in.Context,
		})
		if err != nil {
			return nil, err
		}
		outputMode := strings.ToLower(strings.TrimSpace(asStringInterface(executor["output_mode"])))
		if outputMode == "json" || outputMode == "response_envelope" {
			var output map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &output); err != nil {
				return nil, errors.New("skill.executor_llm_json_output_invalid")
			}
			if outputMode == "response_envelope" {
				return map[string]any{"response_envelope": output}, nil
			}
			return output, nil
		}
		return map[string]any{"content": text, "format": "markdown"}, nil
	case "capability":
		if e == nil || e.capability == nil {
			return nil, errors.New("skill.executor_capability_unavailable")
		}
		capabilityID := strings.TrimSpace(asStringInterface(executor["capability_id"]))
		if capabilityID == "" {
			return nil, errors.New("skill.executor_capability_id_required")
		}
		return e.capability(ctx, ManifestCapabilityInvocation{
			TenantUUID: in.TenantUUID, TraceID: in.TraceID, SkillID: in.SkillID, Version: in.Version,
			Entrypoint: in.Entrypoint, CapabilityID: capabilityID, Payload: in.Payload, Context: in.Context,
		})
	case "workflow":
		if e == nil || e.workflow == nil {
			return nil, errors.New("skill.executor_workflow_unavailable")
		}
		workflowUUID := strings.TrimSpace(asStringInterface(executor["workflow_uuid"]))
		if workflowUUID == "" {
			return nil, errors.New("skill.executor_workflow_uuid_required")
		}
		return e.workflow(ctx, ManifestWorkflowInvocation{
			TenantUUID: in.TenantUUID, TraceID: in.TraceID, SkillID: in.SkillID, Version: in.Version,
			Entrypoint: in.Entrypoint, WorkflowUUID: workflowUUID, Payload: in.Payload, Context: in.Context,
		})
	case "instruction_only":
		return nil, errors.New("skill.executor_instruction_only_not_runnable")
	default:
		return nil, errors.New("skill.executor_type_invalid")
	}
}

// localizedExecutorPrompt deliberately has no default language. A published
// definition must declare the instruction for the requesting UI locale, so an
// agent never silently runs a different-language business prompt.
func localizedExecutorPrompt(executor, contextMap map[string]any) (string, error) {
	locale := strings.TrimSpace(asStringInterface(contextMap["locale"]))
	if locale == "" {
		return "", errors.New("skill.executor_locale_required")
	}
	prompts := nestedManifestMap(executor, "prompt_template_i18n")
	prompt := strings.TrimSpace(asStringInterface(prompts[locale]))
	if prompt == "" {
		return "", errors.New("skill.executor_locale_not_supported")
	}
	return prompt, nil
}

func normalizedExecutorType(manifest map[string]any) (string, bool) {
	executor := nestedManifestMap(manifest, "executor")
	typ := strings.TrimSpace(strings.ToLower(asStringInterface(executor["type"])))
	switch typ {
	case "llm_prompt", "capability", "workflow", "instruction_only":
		return typ, true
	default:
		return "", false
	}
}

func nestedManifestMap(values map[string]any, key string) map[string]any {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	if typed, ok := raw.(map[string]any); ok {
		return typed
	}
	if typed, ok := raw.(map[string]interface{}); ok {
		return typed
	}
	return nil
}
