package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mediaservice "github.com/ArtisanCloud/PowerX/internal/service/media"
	"net/url"

	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	bindings "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentservice "github.com/ArtisanCloud/PowerX/internal/service/agent"
	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 仅持久化运行协议明确声明的上下文；绝不序列化 context、JWT、API Key 或任意请求头。
var adminInputContextKeys = []string{
	"locale", "agent_workspace_mode", "team_id", "team_key", "team_display_name_i18n", "parent_agent_id", "team_members", "team_orchestration",
	"agent_bound_skill_ids", "agent_pending_task", "agent_response_plan", "agent_response_context_layers",
	"planner_optimizer_enabled", "planner_optimizer_candidate_top_k", "planner_optimizer_prompt_slim_mode",
	"planner_optimizer_decision_cache_enabled", "planner_optimizer_decision_cache_ttl_sec",
	"planner_optimizer_quota_workflow", "planner_optimizer_quota_skill", "planner_optimizer_quota_tooling", "planner_optimizer_quota_llm",
}

type adminRunInput struct {
	Schema       string         `json:"schema"`
	TenantUUID   string         `json:"tenant_uuid"`
	Env          string         `json:"env"`
	RunID        string         `json:"run_id"`
	Message      string         `json:"message"`
	Config       dto.ChatConfig `json:"config"`
	ExplicitFlow string         `json:"explicit_flow,omitempty"`
	Context      map[string]any `json:"context"`
}

func saveAdminRunInput(ctx context.Context, objects agent_run.ReportObjectStore, ref agent_run.TaskRef, message string, cfg *dto.ChatConfig, flow string) (string, string, error) {
	if cfg == nil || objects == nil || !validPlanArtifactRef(ref) || strings.TrimSpace(message) == "" {
		return "", "", agent_run.ErrInvalid
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", "", fmt.Errorf("model endpoint must not contain inline credentials")
	}
	frozen := *cfg
	frozen.APIKey = ""
	input := adminRunInput{Schema: "powerx.agent.admin-input/v1", TenantUUID: ref.TenantUUID, Env: ref.Env, RunID: ref.RunID, Message: message, Config: frozen, ExplicitFlow: flow, Context: map[string]any{}}
	for _, key := range adminInputContextKeys {
		if value := ctx.Value(key); value != nil {
			input.Context[key] = value
		}
	}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > maxExecutionPlanArtifactBytes {
		return "", "", agent_run.ErrInvalid
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	key := fmt.Sprintf("agent-runs/%s/%s/%s/input-%s.json", ref.TenantUUID, ref.Env, ref.RunID, checksum)
	if err := objects.Put(ctx, key, raw); err != nil {
		return "", "", err
	}
	read, err := objects.Get(ctx, key)
	if err != nil {
		return "", "", err
	}
	if sha256.Sum256(read) != sum {
		return "", "", agent_run.ErrConflict
	}
	return key, checksum, nil
}

func loadAdminRunInput(ctx context.Context, objects agent_run.ReportObjectStore, row *model.AdminRunAdmission) (adminRunInput, error) {
	var input adminRunInput
	expected := fmt.Sprintf("agent-runs/%s/%s/%s/input-%s.json", row.TenantUUID, row.Env, row.UUID, row.InputChecksum)
	if objects == nil || !artifactChecksumPattern.MatchString(row.InputChecksum) || row.InputRef != expected {
		return input, agent_run.ErrInvalid
	}
	raw, err := objects.Get(ctx, expected)
	if err != nil {
		return input, err
	}
	sum := sha256.Sum256(raw)
	if len(raw) > maxExecutionPlanArtifactBytes || hex.EncodeToString(sum[:]) != row.InputChecksum {
		return input, agent_run.ErrConflict
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, err
	}
	if input.Schema != "powerx.agent.admin-input/v1" || input.TenantUUID != row.TenantUUID.String() || input.Env != row.Env || input.RunID != row.UUID.String() || input.Config.APIKey != "" {
		return input, agent_run.ErrInvalid
	}
	allowed := map[string]bool{}
	for _, key := range adminInputContextKeys {
		allowed[key] = true
	}
	for key := range input.Context {
		if !allowed[key] {
			return input, agent_run.ErrInvalid
		}
	}
	return input, nil
}

type adminResourceAuthorizer interface {
	ResolveEffectivePermissions(context.Context, string, string, string, string, uint64, bool, uuid.UUID) (agentauthz.EffectivePermissionsResult, error)
}
type adminSnapshotRestorer interface {
	Restore(context.Context, string, string, uuid.UUID, uuid.UUID) (*ResourceSnapshot, error)
}

type adminInputLoader struct {
	media     *mediaservice.MediaService
	teams     *agentservice.TeamService
	repo      *repo.AdminRunRepository
	objects   agent_run.ReportObjectStore
	snapshots adminSnapshotRestorer
	agents    *agentservice.AgentService
	config    planningChatConfigResolver
	authz     adminResourceAuthorizer
	bindings  *bindings.AgentSkillBindingRepository
}

func newAdminInputLoader(db *gorm.DB, objects agent_run.ReportObjectStore) *adminInputLoader {
	return &adminInputLoader{repo: repo.NewAdminRunRepository(db), teams: agentservice.NewTeamService(db), objects: objects, snapshots: NewRuntimeSnapshotService(db), agents: agentservice.NewAgentService(db), config: agentservice.NewChatConfigResolver(db), authz: agentauthz.NewService(db), bindings: bindings.NewAgentSkillBindingRepository(db)}
}
func (l *adminInputLoader) Load(ctx context.Context, ref agent_run.TaskRef) (InvokePlanningInput, error) {
	if !validPlanArtifactRef(ref) {
		return InvokePlanningInput{}, agent_run.ErrInvalid
	}
	row, err := l.repo.Get(ctx, uuid.MustParse(ref.TenantUUID), ref.Env, uuid.MustParse(ref.RunID))
	if err != nil {
		return InvokePlanningInput{}, err
	}
	if row.AdmissionState != "admitted" || row.FinishedAt != nil || !time.Now().Before(row.DeadlineAt) {
		return InvokePlanningInput{}, agent_run.ErrConflict
	}
	actor, member, err := l.repo.Actor(ctx, row.TenantUUID, row.SubjectUUID, row.MemberUUID)
	if err != nil {
		return InvokePlanningInput{}, fmt.Errorf("admin run actor authorization revoked")
	}
	session, err := l.repo.Session(ctx, row.TenantUUID, row.Env, row.SessionUUID)
	if err != nil || session.ID != row.SessionID || session.AgentID != row.AgentID || session.Status != "active" || (session.UserID != 0 && session.UserID != actor.ID) || (session.ExpiredAt != nil && !time.Now().Before(*session.ExpiredAt)) {
		return InvokePlanningInput{}, fmt.Errorf("admin run session unavailable")
	}
	ctx = reqctx.WithEnv(reqctx.WithTraceID(reqctx.WithTenantUUID(ctx, row.TenantUUID.String()), row.TraceUUID.String()), row.Env)
	ctx = reqctx.WithUserUUID(reqctx.WithUserID(reqctx.WithIsRoot(ctx, actor.IsRoot), actor.ID), actor.UUID.String())
	memberUUID := ""
	var memberID uint64
	if member != nil {
		memberUUID, memberID = member.UUID.String(), member.ID
		ctx = reqctx.WithMemberUUID(reqctx.WithMemberID(ctx, memberID), memberUUID)
	}
	tenant := row.TenantUUID.String()
	ag, err := l.agents.Get(ctx, row.Env, &tenant, row.AgentID)
	if err != nil || ag.UUID != row.AgentUUID || ag.Env != row.Env || ag.Status != "active" {
		return InvokePlanningInput{}, fmt.Errorf("admin run agent is unavailable")
	}
	permissions, err := l.authz.ResolveEffectivePermissions(ctx, row.Env, tenant, actor.UUID.String(), memberUUID, memberID, actor.IsRoot, row.AgentUUID)
	if err != nil {
		return InvokePlanningInput{}, err
	}
	if !permissions.AgentAccessAllowed {
		return InvokePlanningInput{}, fmt.Errorf("admin run agent access revoked")
	}
	snapshot, err := l.snapshots.Restore(ctx, row.Env, tenant, row.UUID, row.SnapshotUUID)
	if err != nil {
		return InvokePlanningInput{}, err
	}
	allowed := map[uuid.UUID]agentauthz.EffectivePermissionItem{}
	uuidByID := map[string]uuid.UUID{}
	for _, item := range permissions.Items {
		if item.EffectiveAllowed {
			allowed[item.CapabilityUUID] = item
			uuidByID[item.CapabilityID] = item.CapabilityUUID
		}
	}
	for _, resource := range snapshot.Resources {
		if resource.Kind == ResourceKindCapability {
			item, ok := allowed[resource.CapabilityUUID]
			if !ok {
				return InvokePlanningInput{}, fmt.Errorf("admin run capability authorization revoked")
			}
			contract, err := ParseCapabilityRuntimeContract(item.Policy)
			if err != nil {
				return InvokePlanningInput{}, err
			}
			for _, id := range contract.AlternativeCapabilityIDs {
				alternative, ok := uuidByID[id]
				if !ok {
					return InvokePlanningInput{}, fmt.Errorf("admin run alternative authorization revoked")
				}
				contract.AlternativeCapabilityUUIDs = append(contract.AlternativeCapabilityUUIDs, alternative)
			}
			liveContract, _ := json.Marshal(contract)
			frozenContract, _ := json.Marshal(resource.RuntimeContract)
			if string(liveContract) != string(frozenContract) {
				return InvokePlanningInput{}, errAdminCapabilityContractChanged
			}
		}
		if resource.Kind == ResourceKindMediaAsset {
			if l.media == nil {
				return InvokePlanningInput{}, fmt.Errorf("admin media authorization unavailable")
			}
			if _, err := l.media.GetAsset(ctx, tenant, resource.ResourceUUID.String(), false); err != nil {
				return InvokePlanningInput{}, fmt.Errorf("admin attachment authorization revoked")
			}
		}
	}
	input, err := loadAdminRunInput(ctx, l.objects, row)
	if err != nil {
		return InvokePlanningInput{}, err
	}
	message, err := l.repo.Message(ctx, row.TenantUUID, row.Env, row.MessageUUID)
	if err != nil || message.SessionID != row.SessionID || message.AgentID != row.AgentID || message.Role != "user" || strings.TrimSpace(message.Content) != strings.TrimSpace(input.Message) {
		return InvokePlanningInput{}, fmt.Errorf("admin run input message changed")
	}
	live, err := l.config.ResolveForAgentChat(ctx, row.Env, &tenant, row.AgentID, nil)
	if err != nil || live == nil {
		return InvokePlanningInput{}, fmt.Errorf("admin run model configuration unavailable")
	}
	if live.Provider != input.Config.Provider || live.ModelName != input.Config.ModelName || live.Endpoint != input.Config.Endpoint {
		return InvokePlanningInput{}, fmt.Errorf("admin run model configuration changed")
	}
	input.Config.APIKey = live.APIKey
	input.Config.MaxConcurrentRequests = live.MaxConcurrentRequests
	bound, err := l.bindings.ListByAgent(ctx, row.Env, &tenant, row.AgentID)
	if err != nil {
		return InvokePlanningInput{}, err
	}
	current := []string{}
	for _, b := range bound {
		current = append(current, b.SkillID)
	}
	frozen := []string{}
	raw, _ := json.Marshal(input.Context["agent_bound_skill_ids"])
	if json.Unmarshal(raw, &frozen) != nil {
		return InvokePlanningInput{}, agent_run.ErrInvalid
	}
	sort.Strings(current)
	sort.Strings(frozen)
	if !slices.Equal(current, frozen) {
		return InvokePlanningInput{}, fmt.Errorf("admin run skill authorization changed")
	}
	if err := l.validateTeam(ctx, row, input.Context); err != nil {
		return InvokePlanningInput{}, err
	}
	for key, value := range input.Context {
		ctx = context.WithValue(ctx, key, value)
	}
	for key, value := range map[string]any{"run_id": row.UUID.String(), "runId": row.UUID.String(), "runtime_run_uuid": row.UUID.String(), "runtime_snapshot_uuid": row.SnapshotUUID.String(), "session_id": strconv.FormatUint(row.SessionID, 10), "session_uuid": row.SessionUUID.String(), "message_id": strconv.FormatUint(row.MessageID, 10), "message_uuid": row.MessageUUID.String(), "agent_id": strconv.FormatUint(row.AgentID, 10), "agent_numeric_id": row.AgentID, "agent_uuid": row.AgentUUID.String(), "env": row.Env, "agent_env": row.Env, "tenant_uuid": tenant} {
		ctx = context.WithValue(ctx, key, value)
	}
	ctx = context.WithValue(ctx, "agent_node_model_policy", BuildDefaultNodeModelPolicy(&input.Config))
	ctx = ContextWithResourceSnapshot(ctx, snapshot)
	return InvokePlanningInput{Context: ctx, Message: input.Message, Config: &input.Config, ExplicitFlow: input.ExplicitFlow}, nil
}

func (l *adminInputLoader) validateTeam(ctx context.Context, row *model.AdminRunAdmission, input map[string]any) error {
	text, _ := input["team_id"].(string)
	if text == "" {
		return nil
	}
	id, err := strconv.ParseUint(text, 10, 64)
	if err != nil || id == 0 {
		return agent_run.ErrInvalid
	}
	team, err := l.teams.ValidateTeamTenant(ctx, id, row.TenantUUID.String())
	if err != nil {
		return err
	}
	if team.ParentAgentID != row.AgentID || !strings.EqualFold(team.Status, "active") {
		return fmt.Errorf("admin run team unavailable")
	}
	var currentSpec any
	if err := json.Unmarshal(team.OrchestrationSpec, &currentSpec); err != nil {
		return err
	}
	left, _ := json.Marshal(currentSpec)
	right, _ := json.Marshal(input["team_orchestration"])
	if string(left) != string(right) {
		return fmt.Errorf("admin run team plan changed")
	}
	members, err := l.teams.ListMembers(ctx, id, row.TenantUUID.String())
	if err != nil {
		return err
	}
	current := []map[string]any{}
	tenant := row.TenantUUID.String()
	for _, member := range members {
		child, err := l.agents.Get(ctx, row.Env, &tenant, member.ChildAgentID)
		if err != nil {
			return err
		}
		bound, err := l.bindings.ListByAgent(ctx, row.Env, &tenant, member.ChildAgentID)
		if err != nil {
			return err
		}
		skills := []string{}
		for _, b := range bound {
			skills = append(skills, b.SkillID)
		}
		current = append(current, map[string]any{"child_agent_id": member.ChildAgentID, "child_agent_key": strings.TrimSpace(child.Key), "role": strings.TrimSpace(member.Role), "priority": member.Priority, "skill_ids": skills})
	}
	left, _ = json.Marshal(current)
	right, _ = json.Marshal(input["team_members"])
	if string(left) != string(right) {
		return fmt.Errorf("admin run team membership or skill authorization changed")
	}
	return nil
}
