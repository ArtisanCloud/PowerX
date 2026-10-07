package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	bindings "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentservice "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type planningChatConfigResolver interface {
	ResolveForAgentChat(context.Context, string, *string, uint64, *dto.ChatConfig) (*dto.ChatConfig, error)
}

// DBPlanningInputLoader reconstructs one authorized planning request from
// the low-frequency admission anchor and immutable session messages.
type DBPlanningInputLoader struct {
	db       *gorm.DB
	repo     *repository.ServiceSessionRepository
	config   planningChatConfigResolver
	bindings *bindings.AgentSkillBindingRepository
}

// NewDBPlanningInputLoader uses the existing service-session repositories.
func NewDBPlanningInputLoader(db *gorm.DB) *DBPlanningInputLoader {
	if db == nil {
		return &DBPlanningInputLoader{}
	}
	return &DBPlanningInputLoader{db: db, repo: repository.NewServiceSessionRepository(db),
		config: agentservice.NewChatConfigResolver(db), bindings: bindings.NewAgentSkillBindingRepository(db)}
}

func (l *DBPlanningInputLoader) Load(ctx context.Context, ref agent_run.TaskRef) (InvokePlanningInput, error) {
	if l == nil || l.db == nil || l.repo == nil || l.config == nil || l.bindings == nil ||
		!validPlanArtifactRef(ref) || ref.Revision != 0 || ref.TaskID != agent_run.PlanningTaskID || ref.Attempt != 1 {
		return InvokePlanningInput{}, sessions.ErrInvalid
	}
	tenant, _ := uuid.Parse(ref.TenantUUID)
	runID, _ := uuid.Parse(ref.RunID)
	var anchor model.ServiceInvocation
	if err := l.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenant, runID).First(&anchor).Error; err != nil {
		return InvokePlanningInput{}, sessions.ErrNotFound
	}
	if anchor.AdmissionState != "admitted" || anchor.RunEnv != ref.Env ||
		anchor.Status != "accepted" || !time.Now().Before(anchor.DeadlineAt) ||
		anchor.PluginID == "" || anchor.ServiceActor == "" || anchor.CancelRequestedAt != nil {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	owner := repository.SessionOwner{TenantUUID: tenant, PluginID: anchor.PluginID, ServiceActor: anchor.ServiceActor}
	for _, capability := range []string{sessions.SessionCapability, sessions.InvokeCapability} {
		published, raw, err := l.repo.GrantFacts(ctx, tenant, owner.PluginID, capability)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return InvokePlanningInput{}, sessions.ErrDependency
		}
		if !published || err != nil || !serviceCredentialAllows(raw, owner.ServiceActor, capability) {
			return InvokePlanningInput{}, sessions.ErrForbidden
		}
	}
	session, err := l.repo.GetSession(ctx, owner, anchor.SessionUUID)
	if err != nil || session.Status != "active" || session.AgentUUID != anchor.AgentUUID {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	message, err := l.repo.Message(ctx, session, anchor.MessageUUID)
	if err != nil || message.Role != "user" {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	agent, err := l.repo.Agent(ctx, tenant, anchor.AgentUUID)
	if err != nil || agent.Status != "active" || agent.OwnerPluginID == nil ||
		*agent.OwnerPluginID != owner.PluginID || agent.Env != ref.Env {
		return InvokePlanningInput{}, sessions.ErrForbidden
	}
	history, err := l.repo.ExecutionHistory(ctx, session, message.Sequence, sessions.MaxExecutionHistoryMessages+1)
	if err != nil || len(history) > sessions.MaxExecutionHistoryMessages {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	bytes := 0
	items := make([]sessions.Message, 0, len(history))
	for _, row := range history {
		if row.Role != "user" && row.Role != "assistant" {
			return InvokePlanningInput{}, sessions.ErrDependency
		}
		bytes += len(row.Content) + len(row.ResponseEnvelope)
		items = append(items, sessions.Message{MessageUUID: row.UUID, SessionUUID: row.SessionUUID,
			Role: row.Role, Content: row.Content, ResponseEnvelope: json.RawMessage(row.ResponseEnvelope), Sequence: row.Sequence, CreatedAt: row.CreatedAt})
	}
	if bytes > sessions.MaxExecutionHistoryBytes {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	var pending map[string]any
	if len(session.PendingTask) > 0 && string(session.PendingTask) != "null" {
		if session.PendingTaskExpiresAt == nil || !time.Now().Before(*session.PendingTaskExpiresAt) ||
			json.Unmarshal(session.PendingTask, &pending) != nil || pending["status"] != "awaiting_params" ||
			strings.TrimSpace(anyToString(pending["node_ref"])) == "" {
			return InvokePlanningInput{}, sessions.ErrContextExpired
		}
	}
	runCtx := reqctx.WithEnv(reqctx.WithTraceID(reqctx.WithTenantUUID(ctx, ref.TenantUUID), anchor.TraceUUID.String()), ref.Env)
	tenantString := tenant.String()
	cfg, err := l.config.ResolveForAgentChat(runCtx, ref.Env, &tenantString, agent.ID, nil)
	if err != nil || cfg == nil {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	historyPrompt, err := serviceSessionContextPrompt("", items)
	if err != nil {
		return InvokePlanningInput{}, err
	}
	copyConfig := *cfg
	copyConfig.SystemPrompt += "\n" + historyPrompt
	rows, err := l.bindings.ListByAgent(runCtx, ref.Env, &tenantString, agent.ID)
	if err != nil {
		return InvokePlanningInput{}, sessions.ErrDependency
	}
	skills := make([]string, 0, len(rows))
	for _, row := range rows {
		skills = append(skills, row.SkillID)
	}
	if len(pending) > 0 {
		runCtx = context.WithValue(runCtx, "agent_pending_task", pending)
	}
	for key, value := range map[string]any{
		"env": ref.Env, "agent_env": ref.Env, "tenant_uuid": ref.TenantUUID,
		"agent_id": anchor.AgentUUID.String(), "agent_uuid": anchor.AgentUUID.String(),
		"agent_numeric_id": agent.ID,
		"session_id":       anchor.SessionUUID.String(), "session_uuid": anchor.SessionUUID.String(),
		"message_id": anchor.MessageUUID.String(), "agent_bound_skill_ids": skills,
	} {
		runCtx = context.WithValue(runCtx, key, value)
	}
	return InvokePlanningInput{Context: runCtx, Message: message.Content, Config: &copyConfig}, nil
}

func serviceCredentialAllows(raw []byte, actor, capability string) bool {
	var credential struct {
		ClientID string   `json:"client_id"`
		Allowed  []string `json:"allowed_capabilities"`
	}
	if json.Unmarshal(raw, &credential) != nil || credential.ClientID == "" || actor != "client:"+credential.ClientID {
		return false
	}
	for _, allowed := range credential.Allowed {
		if allowed == capability {
			return true
		}
	}
	return false
}
