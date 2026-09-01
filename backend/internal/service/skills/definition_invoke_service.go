package skills

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// DefinitionInvokeService is the v2 runtime entry point. It reads exactly one
// tenant-owned, published revision and dispatches by its declared executor.
// It intentionally has no Registry/local-file/remote-repository fallback.
type DefinitionInvokeService struct {
	repo         *skillrepo.SkillDefinitionRepository
	executor     SkillExecutor
	auditService *AuditTraceService
}

func NewDefinitionInvokeService(
	repo *skillrepo.SkillDefinitionRepository,
	executor SkillExecutor,
	auditService *AuditTraceService,
) *DefinitionInvokeService {
	if repo == nil {
		panic("skill definition invoke service requires repository")
	}
	if executor == nil {
		panic("skill definition invoke service requires executor")
	}
	return &DefinitionInvokeService{repo: repo, executor: executor, auditService: auditService}
}

func (s *DefinitionInvokeService) Execute(
	ctx context.Context,
	req InvokeRequest,
	payload map[string]any,
	contextMap map[string]any,
) (*InvokeExecutionResult, error) {
	if s == nil || s.repo == nil || s.executor == nil {
		return nil, errors.New("skill.definition_runtime_unavailable")
	}
	startAt := time.Now()
	req.TenantUUID = strings.TrimSpace(strings.ToLower(req.TenantUUID))
	req.SkillID = strings.TrimSpace(strings.ToLower(req.SkillID))
	req.Entrypoint = strings.TrimSpace(req.Entrypoint)
	if req.TenantUUID == "" || req.SkillID == "" {
		return nil, errors.New("skill.definition_runtime_context_invalid")
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "runbook.default"
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if contextMap == nil {
		contextMap = map[string]any{}
	}

	draft, err := s.repo.GetDraftBySkillID(ctx, req.TenantUUID, req.SkillID)
	if err != nil {
		return nil, err
	}
	if draft.Status != skillmodel.SkillDefinitionDraftStatusPublished {
		return nil, errors.New("skill.definition_not_published")
	}
	revision, err := s.repo.GetCurrentRevision(ctx, req.TenantUUID, draft.UUID.String())
	if err != nil {
		return nil, err
	}
	if revision.Status != skillmodel.SkillDefinitionRevisionStatusPublished {
		return nil, errors.New("skill.definition_revision_not_published")
	}
	if requestedVersion := strings.TrimSpace(req.Version); requestedVersion != "" && requestedVersion != revision.UUID.String() {
		return nil, errors.New("skill.definition_revision_not_current")
	}
	if strings.TrimSpace(revision.PublishedArtifactURI) == "" || strings.TrimSpace(revision.PublishedChecksum) == "" {
		return nil, errors.New("skill.definition_published_artifact_missing")
	}
	var definition map[string]any
	if err := json.Unmarshal(revision.DefinitionJSON, &definition); err != nil {
		return nil, errors.New("skill.definition_runtime_definition_invalid")
	}
	if err := validatePowerXDefinition(definition); err != nil {
		return nil, err
	}
	if err := validateEntrypointAllowed(definition, req.Entrypoint); err != nil {
		return nil, err
	}
	in := ExecuteInput{
		TenantUUID: req.TenantUUID,
		TraceID:    req.TraceID,
		SkillID:    draft.SkillID,
		Version:    revision.UUID.String(),
		Entrypoint: req.Entrypoint,
		Payload:    payload,
		Context:    contextMap,
		Manifest:   definition,
		Source:     draft.SourceKind,
	}
	result, err := s.executor.Execute(ctx, in)
	if err != nil {
		s.record(ctx, startAt, req, in, nil, err)
		return nil, err
	}
	if result == nil {
		return nil, errors.New("skill.definition_executor_empty_result")
	}
	out := &InvokeExecutionResult{
		TraceID: req.TraceID, Status: "completed", ProtocolUsed: "skill", FallbackUsed: false,
		SkillID: draft.SkillID, Version: revision.UUID.String(), Result: result,
	}
	s.record(ctx, startAt, req, in, result, nil)
	return out, nil
}

func (s *DefinitionInvokeService) record(ctx context.Context, startedAt time.Time, req InvokeRequest, in ExecuteInput, result map[string]any, execErr error) {
	if s == nil || s.auditService == nil {
		return
	}
	status := "completed"
	code := ""
	summary := ""
	if execErr != nil {
		status = "failed"
		code = errorCodeFromError(execErr)
		summary = execErr.Error()
	}
	_ = s.auditService.RecordExecutionTrace(ctx, ExecutionTraceInput{
		TraceID: req.TraceID, TenantUUID: req.TenantUUID, SkillID: in.SkillID, Version: in.Version,
		Entrypoint: req.Entrypoint, InvokePath: "tenant.skills.definition_invoke", ProtocolUsed: "skill",
		Status: status, LatencyMS: int(time.Since(startedAt).Milliseconds()), ErrorCode: code, ErrorSummary: summary,
		RequestPayload: in.Payload, ResponsePayload: result, FallbackUsed: false, AuthPass: true,
		AgentID: firstTraceString(in.Context["agent_uuid"]), SessionID: firstTraceString(in.Context["session_uuid"]),
		MessageID: firstTraceString(in.Context["message_uuid"]), ExecutorPath: manifestExecutorPath(in.Manifest),
	})
}
