package skills

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// InvokeRequest describes resolved invoke input for service layer.
type InvokeRequest struct {
	TenantUUID string
	SkillID    string
	Version    string
	Entrypoint string
	InvokePath string
	TraceID    string
}

// InvokeResolution returns selected version and normalized routing metadata.
type InvokeResolution struct {
	SkillID    string
	Version    string
	Entrypoint string
	InvokePath string
	TraceID    string
}

// InvokeExecutionResult returns execute stage output.
type InvokeExecutionResult struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	SkillID      string
	Version      string
	Result       map[string]interface{}
}

// InvokeService resolves versions and validates invocation prerequisites.
type InvokeService struct {
	registryRepo *skillrepo.SkillRegistryRepository
	auditService *AuditTraceService
	executors    []SkillExecutor
}

func NewInvokeService(
	registryRepo *skillrepo.SkillRegistryRepository,
	auditService *AuditTraceService,
) *InvokeService {
	if registryRepo == nil {
		panic("invoke service requires registry repository")
	}
	return &InvokeService{
		registryRepo: registryRepo,
		auditService: auditService,
		executors: []SkillExecutor{
			newIncidentTriageExecutor(),
			newPromptTemplateExecutor(),
			newVideoFramesExecutor(),
			newMarketingKnowledgeExecutor(),
		},
	}
}

func (s *InvokeService) WithExecutors(executors ...SkillExecutor) *InvokeService {
	if s == nil {
		return nil
	}
	s.executors = append([]SkillExecutor(nil), executors...)
	return s
}

func (s *InvokeService) AddExecutor(executor SkillExecutor) *InvokeService {
	if s == nil || executor == nil {
		return s
	}
	s.executors = append(s.executors, executor)
	return s
}

func (s *InvokeService) Resolve(ctx context.Context, req InvokeRequest) (*InvokeResolution, error) {
	req.TenantUUID = strings.ToLower(strings.TrimSpace(req.TenantUUID))
	req.SkillID = strings.ToLower(strings.TrimSpace(req.SkillID))
	req.Version = strings.TrimSpace(req.Version)
	req.Entrypoint = strings.TrimSpace(req.Entrypoint)
	req.InvokePath = strings.TrimSpace(strings.ToLower(req.InvokePath))

	if req.TenantUUID == "" {
		return nil, errors.New("tenant_uuid is required")
	}
	if req.SkillID == "" {
		return nil, errors.New("skill_id is required")
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "runbook.default"
	}
	if req.InvokePath == "" {
		req.InvokePath = "tenant.skills.invoke"
	}
	if strings.TrimSpace(req.TraceID) == "" {
		req.TraceID = uuid.NewString()
	}

	selectedVersion := req.Version
	if selectedVersion == "" {
		latest, err := s.registryRepo.GetLatestPublished(ctx, req.SkillID)
		if err != nil {
			return nil, err
		}
		selectedVersion = latest.Version
	}

	rec, err := s.registryRepo.GetBySkillVersion(ctx, req.SkillID, selectedVersion)
	if err != nil {
		return nil, err
	}
	if rec.Status != skillmodel.SkillStatusPublished {
		return nil, errors.New("skill version is not published")
	}

	resolution := &InvokeResolution{
		SkillID:    req.SkillID,
		Version:    selectedVersion,
		Entrypoint: req.Entrypoint,
		InvokePath: req.InvokePath,
		TraceID:    req.TraceID,
	}

	if s.auditService != nil {
		_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
			TraceID:      req.TraceID,
			TenantUUID:   req.TenantUUID,
			SkillID:      resolution.SkillID,
			Version:      resolution.Version,
			Entrypoint:   resolution.Entrypoint,
			InvokePath:   resolution.InvokePath,
			ProtocolUsed: "skill",
			Status:       "resolved",
		})
	}

	return resolution, nil
}

func (s *InvokeService) Execute(
	ctx context.Context,
	req InvokeRequest,
	payload map[string]interface{},
	contextMap map[string]interface{},
) (*InvokeExecutionResult, error) {
	startAt := time.Now()
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if contextMap == nil {
		contextMap = map[string]interface{}{}
	}

	resolved, err := s.Resolve(ctx, req)
	if err != nil {
		return nil, err
	}

	rec, err := s.registryRepo.GetBySkillVersion(ctx, resolved.SkillID, resolved.Version)
	if err != nil {
		s.recordFailedTrace(ctx, startAt, resolved, req.TenantUUID, payload, err)
		return nil, err
	}

	manifest, err := NormalizeManifestJSON(rec.ManifestJSON, resolved.Version)
	if err != nil {
		s.recordFailedTrace(ctx, startAt, resolved, req.TenantUUID, payload, err)
		return nil, err
	}
	if err := validateEntrypointAllowed(manifest, resolved.Entrypoint); err != nil {
		s.recordFailedTrace(ctx, startAt, resolved, req.TenantUUID, payload, err)
		return nil, err
	}

	in := ExecuteInput{
		TenantUUID:   req.TenantUUID,
		TraceID:      resolved.TraceID,
		SkillID:      resolved.SkillID,
		Version:      resolved.Version,
		Entrypoint:   resolved.Entrypoint,
		Payload:      payload,
		Context:      contextMap,
		Manifest:     manifest,
		Source:       rec.Source,
		CapabilityID: strings.TrimSpace(asStringInterface(contextMap["capability_id"])),
	}
	outcome := &InvokeExecutionResult{
		TraceID:      resolved.TraceID,
		Status:       "completed",
		ProtocolUsed: "skill",
		FallbackUsed: false,
		SkillID:      resolved.SkillID,
		Version:      resolved.Version,
		Result:       payload,
	}

	executor := pickExecutor(s.executors, in)
	if executor != nil {
		result, execErr := executor.Execute(ctx, in)
		if execErr != nil {
			s.recordFailedTraceWithInput(ctx, startAt, resolved, req.TenantUUID, payload, execErr, in)
			return nil, execErr
		}
		if result == nil {
			result = map[string]interface{}{}
		}
		outcome.Result = result
	} else {
		execErr := errNoExecutorMatched
		s.recordFailedTrace(ctx, startAt, resolved, req.TenantUUID, payload, execErr)
		return nil, execErr
	}

	if s.auditService != nil {
		_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
			TraceID:          outcome.TraceID,
			TenantUUID:       req.TenantUUID,
			SkillID:          outcome.SkillID,
			Version:          outcome.Version,
			Entrypoint:       resolved.Entrypoint,
			InvokePath:       resolved.InvokePath,
			ProtocolUsed:     "skill",
			Status:           outcome.Status,
			LatencyMS:        int(time.Since(startAt).Milliseconds()),
			RequestPayload:   payload,
			ResponsePayload:  outcome.Result,
			FallbackUsed:     outcome.FallbackUsed,
			AuthPass:         true,
			CapabilityID:     firstTraceString(in.CapabilityID, in.Context["capability_id"], in.Context["capability"]),
			ProviderPluginID: firstTraceString(in.Manifest["provider"], in.Context["provider_plugin_id"], in.Context["plugin_id"]),
			AgentID:          firstTraceString(in.Context["agent_id"]),
			SessionID:        firstTraceString(in.Context["session_id"]),
			MessageID:        firstTraceString(in.Context["message_id"]),
			ExecutorPath:     manifestExecutorPath(in.Manifest),
			PluginTaskID:     firstTraceString(outcome.Result["task_id"], outcome.Result["plugin_task_id"]),
		})
	}

	return outcome, nil
}

func (s *InvokeService) recordFailedTraceWithInput(
	ctx context.Context,
	startAt time.Time,
	resolved *InvokeResolution,
	tenantUUID string,
	payload map[string]interface{},
	err error,
	in ExecuteInput,
) {
	if s == nil || s.auditService == nil || resolved == nil || err == nil {
		return
	}
	_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
		TraceID:          resolved.TraceID,
		TenantUUID:       tenantUUID,
		SkillID:          resolved.SkillID,
		Version:          resolved.Version,
		Entrypoint:       resolved.Entrypoint,
		InvokePath:       resolved.InvokePath,
		ProtocolUsed:     "skill",
		Status:           "failed",
		LatencyMS:        int(time.Since(startAt).Milliseconds()),
		ErrorCode:        errorCodeFromError(err),
		ErrorSummary:     strings.TrimSpace(err.Error()),
		RequestPayload:   payload,
		FallbackUsed:     false,
		AuthPass:         true,
		CapabilityID:     firstTraceString(in.CapabilityID, in.Context["capability_id"], in.Context["capability"]),
		ProviderPluginID: firstTraceString(in.Manifest["provider"], in.Context["provider_plugin_id"], in.Context["plugin_id"]),
		AgentID:          firstTraceString(in.Context["agent_id"]),
		SessionID:        firstTraceString(in.Context["session_id"]),
		MessageID:        firstTraceString(in.Context["message_id"]),
		ExecutorPath:     manifestExecutorPath(in.Manifest),
	})
}

func asStringInterface(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprint(x), " "))
	}
}

func (s *InvokeService) recordFailedTrace(
	ctx context.Context,
	startAt time.Time,
	resolved *InvokeResolution,
	tenantUUID string,
	payload map[string]interface{},
	err error,
) {
	if s == nil || s.auditService == nil || resolved == nil || err == nil {
		return
	}
	_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
		TraceID:        resolved.TraceID,
		TenantUUID:     tenantUUID,
		SkillID:        resolved.SkillID,
		Version:        resolved.Version,
		Entrypoint:     resolved.Entrypoint,
		InvokePath:     resolved.InvokePath,
		ProtocolUsed:   "skill",
		Status:         "failed",
		LatencyMS:      int(time.Since(startAt).Milliseconds()),
		ErrorCode:      errorCodeFromError(err),
		ErrorSummary:   strings.TrimSpace(err.Error()),
		RequestPayload: payload,
		FallbackUsed:   false,
		AuthPass:       true,
	})
}

func errorCodeFromError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	for _, code := range []string{
		ErrorCodePluginNotInstalled,
		ErrorCodePluginExecutorUnavailable,
		ErrorCodePluginContextMissing,
		ErrorCodePluginCapabilityMismatch,
		ErrorCodeSkillNotFound,
		ErrorCodeVersionNotFound,
		ErrorCodePermissionDenied,
	} {
		if strings.Contains(msg, strings.ToLower(code)) {
			return code
		}
	}
	return ErrorCodeExecutionFailed
}

func firstTraceString(values ...interface{}) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		if s := strings.TrimSpace(fmt.Sprint(value)); s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func manifestExecutorPath(manifest map[string]interface{}) string {
	raw, ok := manifest["executor"].(map[string]interface{})
	if !ok {
		return ""
	}
	return firstTraceString(raw["path"])
}
