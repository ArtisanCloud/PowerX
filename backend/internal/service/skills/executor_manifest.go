package skills

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
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
	ResponseSchema map[string]any
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
	LLM           ManifestLLMInvoker
	Capability    ManifestCapabilityInvoker
	Workflow      ManifestWorkflowInvoker
	Dependencies  *DependencyService
	Tools         *evidence.ToolService
	EvidenceStore evidence.EvidenceArchiveStore
}

// ManifestExecutor is the generic Skill dispatcher. It selects exclusively by
// `definition.executor.type`; skill IDs, agent names and team keys are never
// part of its routing decision.
type ManifestExecutor struct {
	llm           ManifestLLMInvoker
	capability    ManifestCapabilityInvoker
	workflow      ManifestWorkflowInvoker
	dependencies  *DependencyService
	tools         *evidence.ToolService
	evidenceStore evidence.EvidenceArchiveStore
}

func NewManifestExecutor(options ManifestExecutorOptions) *ManifestExecutor {
	if options.Tools == nil {
		options.Tools = NewRuntimeToolService()
	}
	if options.Dependencies == nil {
		options.Dependencies = NewDependencyService(DependencyServiceOptions{
			Executors: map[string]bool{"llm_prompt": options.LLM != nil, "capability": options.Capability != nil, "workflow": options.Workflow != nil},
		})
	}
	return &ManifestExecutor{
		llm:          options.LLM,
		capability:   options.Capability,
		workflow:     options.Workflow,
		dependencies: options.Dependencies,
		tools:        options.Tools, evidenceStore: options.EvidenceStore,
	}
}

func (e *ManifestExecutor) CanHandle(in ExecuteInput) bool {
	_, ok := normalizedExecutorType(in.Manifest)
	return ok
}

func (e *ManifestExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]any, error) {
	if e == nil {
		return nil, errors.New("skill.executor_unavailable")
	}
	var scope struct {
		AgentID uint64 `json:"agent_id"`
	}
	_ = utils.MapToStruct(map[string]any{"agent_id": in.Context["agent_id"]}, &scope)
	if err := e.dependencies.Check(ctx, DependencyCheckInput{TenantUUID: in.TenantUUID, Stage: DependencyExecute, Env: reqctx.GetEnv(ctx), AgentID: scope.AgentID, Definition: in.Manifest}); err != nil {
		return nil, err
	}
	ctx = evidence.WithToolAgentID(ctx, scope.AgentID)
	typ, ok := normalizedExecutorType(in.Manifest)
	if !ok {
		return nil, errors.New("skill.executor_definition_invalid")
	}
	executor := nestedManifestMap(in.Manifest, "executor")
	if asStringInterface(executor["response_contract"]) == evidence.ReportSchemaV5 {
		if e.evidenceStore == nil {
			return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.storage_unavailable", Stage: "dependency_check"}}
		}
		if err := e.evidenceStore.Ready(ctx); err != nil {
			return nil, &evidence.ToolEvidenceError{ToolFailure: evidence.ToolFailure{Code: "evidence.storage_unavailable", Stage: "dependency_check"}, Cause: err}
		}
	}
	switch typ {
	case "llm_prompt":
		if e == nil || e.llm == nil {
			return nil, errors.New("skill.executor_llm_unavailable")
		}
		prompt, err := localizedExecutorPrompt(executor, in.Context)
		if err != nil {
			return nil, err
		}
		outputMode := strings.ToLower(strings.TrimSpace(asStringInterface(executor["output_mode"])))
		if outputMode == "response_envelope" {
			return e.executeEvidenceReport(ctx, in, executor, prompt)
		}
		invocation := ManifestLLMInvocation{
			TenantUUID:     in.TenantUUID,
			TraceID:        in.TraceID,
			SkillID:        in.SkillID,
			Version:        in.Version,
			Entrypoint:     in.Entrypoint,
			PromptTemplate: prompt,
			ModelPolicy:    nestedManifestMap(executor, "model_policy"),
			Payload:        in.Payload,
			Context:        in.Context,
		}
		text, err := e.llm(ctx, invocation)
		if err != nil {
			return nil, err
		}
		if outputMode == "json" {
			var output map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &output); err != nil {
				return nil, errors.New("skill.executor_llm_json_output_invalid")
			}

			return output, nil
		}
		return map[string]any{"content": text, "format": "markdown"}, nil
	case "capability":
		return e.executeCapabilityV5(ctx, in)

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
