package runtime

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"sync"

	bindings "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentservice "github.com/ArtisanCloud/PowerX/internal/service/agent"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

//go:embed locales/service_session_context.json
var serviceSessionContextJSON []byte

func serviceSessionContextPrompt(language string, history []sessions.Message) (string, error) {
	var locales map[string]string
	if err := json.Unmarshal(serviceSessionContextJSON, &locales); err != nil {
		return "", sessions.ErrDependency
	}
	locale := "zh-CN"
	if strings.HasPrefix(strings.ToLower(language), "en") {
		locale = "en-US"
	}
	raw, err := json.Marshal(history)
	if err != nil {
		return "", sessions.ErrDependency
	}
	return strings.ReplaceAll(locales[locale], "{{history_json}}", string(raw)), nil
}

type ServiceSessionExecutor struct {
	repo     *repository.ServiceSessionRepository
	bindings *bindings.AgentSkillBindingRepository
	config   *agentservice.ChatConfigResolver
}

func NewServiceSessionExecutor(db *gorm.DB) *ServiceSessionExecutor {
	return &ServiceSessionExecutor{repository.NewServiceSessionRepository(db), bindings.NewAgentSkillBindingRepository(db), agentservice.NewChatConfigResolver(db)}
}

var _ sessions.Executor = (*ServiceSessionExecutor)(nil)

func (e *ServiceSessionExecutor) Execute(ctx context.Context, session sessions.Session, message sessions.Message) (string, error) {
	tenant, err := uuid.Parse(reqctx.GetTenantUUID(ctx))
	if err != nil || tenant == uuid.Nil {
		return "", sessions.ErrUnauthorized
	}
	agent, err := e.repo.Agent(ctx, tenant, session.AgentUUID)
	if err != nil {
		return "", err
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || agent.Status != "active" || agent.OwnerPluginID == nil || *agent.OwnerPluginID != claims.PluginID {
		return "", sessions.ErrForbidden
	}
	if strings.TrimSpace(agent.Env) == "" {
		return "", sessions.ErrDependency
	}
	tenantUUID := tenant.String()
	// The numeric surrogate is confined to the existing internal resolver. No
	// numeric reference enters the service-session records or external DTO.
	cfg, err := e.config.ResolveForAgentChat(ctx, agent.Env, &tenantUUID, agent.ID, nil)
	if err != nil {
		return "", err
	}
	memory := sessions.ExecutionMemoryFromContext(ctx)
	if memory == nil || cfg == nil {
		return "", sessions.ErrDependency
	}
	historyPrompt, err := serviceSessionContextPrompt(ctxStringValue(ctx, "locale"), memory.History)
	if err != nil {
		return "", err
	}
	copyConfig := *cfg
	copyConfig.SystemPrompt += "\n" + historyPrompt
	cfg = &copyConfig
	if len(memory.Pending) > 0 {
		if !pendingTaskStatusAwaitingParams(memory.Pending) {
			return "", sessions.ErrDependency
		}
		ctx = context.WithValue(ctx, "agent_pending_task", memory.Pending)
	}
	rows, err := e.bindings.ListByAgent(ctx, agent.Env, &tenantUUID, agent.ID)
	if err != nil {
		return "", err
	}
	skills := make([]string, 0, len(rows))
	for _, row := range rows {
		skills = append(skills, row.SkillID)
	}
	for key, value := range map[string]any{
		"env": agent.Env, "agent_env": agent.Env, "tenant_uuid": tenantUUID,
		"agent_id": session.AgentUUID.String(), "agent_uuid": session.AgentUUID.String(),
		"session_id": session.SessionUUID.String(), "session_uuid": session.SessionUUID.String(),
		"message_id": message.MessageUUID.String(), "agent_bound_skill_ids": skills,
	} {
		ctx = context.WithValue(ctx, key, value)
	}
	sink := &serviceSessionFinalSink{memory: memory}
	_, _, err = NewEngine().RunPlanInvoke(ctx, message.Content, cfg, "", sink)
	if err != nil {
		return "", err
	}
	return sink.result()
}

// Only the runtime's declared final envelope completes an invocation. Token
// fragments, debug events and free-form error text are never output substitutes.
type serviceSessionFinalSink struct {
	memory *sessions.ExecutionMemory
	mu     sync.Mutex
	seen   bool
	output string
	err    error
}

func (s *serviceSessionFinalSink) Emit(event string, payload any) error {
	if s.memory != nil && event == dto.EventAgentRunAwaitingParams {
		task := mapFromAny(payload)
		if len(task) == 0 {
			s.mu.Lock()
			s.err = sessions.ErrDependency
			s.mu.Unlock()
			return sessions.ErrDependency
		}
		task["status"] = dto.AgentTaskStatusAwaitingParams
		if err := s.memory.SetPending(task); err != nil {
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
			return err
		}
	}
	if event != dto.EventFinal {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen {
		s.err = sessions.ErrDependency
		return s.err
	}
	s.seen = true
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	raw, err := json.Marshal(payload)
	if err == nil {
		err = json.Unmarshal(raw, &envelope)
	}
	if err != nil || !envelope.Success || strings.TrimSpace(envelope.Data.Content) == "" {
		s.err = sessions.ErrDependency
		return s.err
	}
	s.output = SanitizeAssistantVisibleText(envelope.Data.Content)
	return nil
}
func (s *serviceSessionFinalSink) result() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.seen || s.err != nil || strings.TrimSpace(s.output) == "" {
		return "", sessions.ErrDependency
	}
	return s.output, nil
}
