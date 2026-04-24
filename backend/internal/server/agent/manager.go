// services/agent/manager_min.go
package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/handler"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/run_log"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound   = errors.New("agent not found")
	ErrIDConflict = errors.New("agent id conflict")
)

// —— 路由记录：flow → (agent, intent spec) —— //
type routeRecord struct {
	AgentID string
	Spec    *schemas.IntentSpec
}

type SkillInvokeInput struct {
	TenantUUID   string
	Env          string
	AgentID      uint64
	SkillID      string
	Version      string
	CapabilityID string
	Entrypoint   string
	TraceID      string
	Payload      map[string]any
	Context      map[string]any
	ToolGrantIDs []string
}

type SkillInvokeOutput struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	SkillID      string
	Version      string
	Result       map[string]any
}

type ToolingInvokeInput struct {
	TenantUUID        string
	Env               string
	CapabilityID      string
	PreferredProtocol string
	TraceID           string
	Payload           map[string]any
	Context           map[string]any
}

type ToolingInvokeOutput struct {
	TraceID      string
	Status       string
	ProtocolUsed string
	FallbackUsed bool
	Result       map[string]any
}

type AgentHandoffInput struct {
	TenantUUID     string
	ParentAgentID  uint64
	ChildAgentID   uint64
	TeamID         uint64
	TaskID         string
	PlanID         string
	NodeID         string
	SessionID      uint64
	FailurePolicy  string
	ContextRefID   string
	HandoffTraceID string
	FlowID         string
	Message        string
	Payload        map[string]any
	Context        map[string]any
}

type AgentHandoffOutput struct {
	TaskID         string
	HandoffTraceID string
	Status         string
	Result         map[string]any
}

type SkillInvoker func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error)
type ToolingInvoker func(ctx context.Context, in ToolingInvokeInput) (*ToolingInvokeOutput, error)
type AgentHandoffInvoker func(ctx context.Context, in AgentHandoffInput) (*AgentHandoffOutput, error)
type ContextRefAuthorizer func(ctx context.Context, tenantUUID string, childAgentID uint64, contextRefID string) error

type DebugTraceConfig struct {
	Enabled      bool
	Dir          string
	MaxBodyBytes int
}

type ContextOptimizerConfig struct {
	Enabled                   bool
	MaxPromptTokens           int
	ReservedCompletionTokens  int
	RecentMessages            int
	RetrievalTopK             int
	CacheMode                 string
	SummaryRefreshIntervalSec int
}

type PlannerKindQuota struct {
	Workflow int
	Skill    int
	Tooling  int
	LLM      int
}

type PlannerOptimizerConfig struct {
	Enabled              bool
	CandidateTopK        int
	PerKindQuota         PlannerKindQuota
	PromptSlimMode       string // compact|verbose
	DecisionCacheEnabled bool
	DecisionCacheTTLSec  int
}

// —— Manager（非意图部分：Agent/路由/状态） —— //
type Manager struct {
	mu            sync.RWMutex
	agentClients  map[string]contract.AgentClient
	meta          map[string]*aschema.AgentMeta
	runtime       map[string]*aschema.Runtime
	defaultAgID   string
	defaultFlowID string

	// 路由：单一事实源 + 二级索引
	routesByFlow map[string]routeRecord         // flowID -> (agentID, spec)
	flowsByAgent map[string]map[string]struct{} // agentID -> set(flowID)
	// 统一候选池：skill/tooling/workflow（workflow 仍可由 routesByFlow 衍生）
	unifiedCandidates map[string]ToolCallCandidate // key=name
	skillInvoker      SkillInvoker
	toolingInvoker    ToolingInvoker
	handoffInvoker    AgentHandoffInvoker
	contextRefAuthz   ContextRefAuthorizer

	// —— 意图字段（方法挪到 manager_intent.go，需要这些字段作为状态） —— //
	intentStrategies []contract.IntentStrategy
	intentLow        float64
	intentHigh       float64
	plannerRules     []schemas.WireRule

	// Handler注册器
	handlerReg *handler.HandlerRegistry

	// 事件记录
	runLog run_log.RunLogger
	// 调试追踪：单请求落单文件，便于还原输入/输出。
	debugTraceCfg DebugTraceConfig
	// 上下文优化运行参数。
	contextOptimizerCfg ContextOptimizerConfig
	// Planner 提速参数（候选预筛 + prompt 瘦身 + 决策缓存）。
	plannerOptimizerCfg PlannerOptimizerConfig
	// planner 阶段 usage 快照（按 trace_id 暂存，供 stream 聚合）。
	plannerUsageByTrace map[string]map[string]any
}

// —— 单例 —— //
var (
	agentManager *Manager
	once         sync.Once
)

func NewAgentManager() *Manager {
	return &Manager{
		agentClients:        make(map[string]contract.AgentClient),
		meta:                make(map[string]*aschema.AgentMeta),
		runtime:             make(map[string]*aschema.Runtime),
		routesByFlow:        make(map[string]routeRecord),
		flowsByAgent:        make(map[string]map[string]struct{}),
		unifiedCandidates:   make(map[string]ToolCallCandidate),
		plannerUsageByTrace: make(map[string]map[string]any),
		plannerOptimizerCfg: PlannerOptimizerConfig{
			Enabled:       true,
			CandidateTopK: 24,
			PerKindQuota: PlannerKindQuota{
				Workflow: 4,
				Skill:    10,
				Tooling:  8,
				LLM:      2,
			},
			PromptSlimMode:       "compact",
			DecisionCacheEnabled: true,
			DecisionCacheTTLSec:  60,
		},
	}
}
func GetAgentManager() *Manager {
	once.Do(func() { agentManager = NewAgentManager() })
	return agentManager
}

func (m *Manager) SetRunLogger(l run_log.RunLogger) { m.runLog = l }

func (m *Manager) SetDebugTraceConfig(cfg DebugTraceConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.Dir == "" {
		cfg.Dir = "logs/agent_debug"
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 512 * 1024
	}
	m.debugTraceCfg = cfg
}

func (m *Manager) debugTraceConfig() DebugTraceConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.debugTraceCfg
}

func (m *Manager) GetDebugTraceConfig() DebugTraceConfig {
	return m.debugTraceConfig()
}

func (m *Manager) SetContextOptimizerConfig(cfg ContextOptimizerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.MaxPromptTokens <= 0 {
		cfg.MaxPromptTokens = 12000
	}
	if cfg.ReservedCompletionTokens <= 0 {
		cfg.ReservedCompletionTokens = 1200
	}
	if cfg.RecentMessages <= 0 {
		cfg.RecentMessages = 8
	}
	if cfg.RetrievalTopK <= 0 {
		cfg.RetrievalTopK = 6
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.CacheMode))
	switch mode {
	case "auto", "force_off", "force_on":
		cfg.CacheMode = mode
	default:
		cfg.CacheMode = "auto"
	}
	if cfg.SummaryRefreshIntervalSec <= 0 {
		cfg.SummaryRefreshIntervalSec = 900
	}
	m.contextOptimizerCfg = cfg
}

func (m *Manager) GetContextOptimizerConfig() ContextOptimizerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.contextOptimizerCfg
}

func (m *Manager) SetPlannerOptimizerConfig(cfg PlannerOptimizerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cfg.CandidateTopK <= 0 {
		cfg.CandidateTopK = 24
	}
	if cfg.PerKindQuota.Workflow <= 0 {
		cfg.PerKindQuota.Workflow = 4
	}
	if cfg.PerKindQuota.Skill <= 0 {
		cfg.PerKindQuota.Skill = 10
	}
	if cfg.PerKindQuota.Tooling <= 0 {
		cfg.PerKindQuota.Tooling = 8
	}
	if cfg.PerKindQuota.LLM <= 0 {
		cfg.PerKindQuota.LLM = 2
	}
	switch strings.ToLower(strings.TrimSpace(cfg.PromptSlimMode)) {
	case "compact", "verbose":
		cfg.PromptSlimMode = strings.ToLower(strings.TrimSpace(cfg.PromptSlimMode))
	default:
		cfg.PromptSlimMode = "compact"
	}
	if cfg.DecisionCacheTTLSec <= 0 {
		cfg.DecisionCacheTTLSec = 60
	}
	m.plannerOptimizerCfg = cfg
}

func (m *Manager) GetPlannerOptimizerConfig() PlannerOptimizerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plannerOptimizerCfg
}

func (m *Manager) log() run_log.RunLogger {
	if m.runLog == nil {
		return &noopRunLogger{}
	}
	return m.runLog
}

func (m *Manager) SetSkillInvoker(inv SkillInvoker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.skillInvoker = inv
}

func (m *Manager) SetToolingInvoker(inv ToolingInvoker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolingInvoker = inv
}

func (m *Manager) SetAgentHandoffInvoker(inv AgentHandoffInvoker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handoffInvoker = inv
}

func (m *Manager) SetContextRefAuthorizer(fn ContextRefAuthorizer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contextRefAuthz = fn
}

func (m *Manager) setPlannerUsage(traceID string, usage map[string]any) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" || len(usage) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plannerUsageByTrace == nil {
		m.plannerUsageByTrace = make(map[string]map[string]any)
	}
	m.plannerUsageByTrace[traceID] = usage
}

func (m *Manager) PopPlannerUsage(traceID string) map[string]any {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.plannerUsageByTrace == nil {
		return nil
	}
	v := m.plannerUsageByTrace[traceID]
	delete(m.plannerUsageByTrace, traceID)
	return v
}

// —— Agent 注册 / 默认路由 —— //
func (m *Manager) Register(clientID string, ag contract.AgentClient, meta *aschema.AgentMeta) error {
	if clientID == "" || ag == nil || meta == nil {
		return errors.New("bad register args")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.agentClients[clientID]; ok {
		return ErrIDConflict
	}
	now := time.Now().UTC()
	in := *meta
	in.ID, in.CreatedAt, in.UpdatedAt, in.LastBeatAt = clientID, now, now, now
	if in.Status == "" {
		in.Status = aschema.StatusInit
	}
	if in.Extras == nil {
		in.Extras = map[string]string{}
	}

	m.agentClients[clientID] = ag
	m.meta[clientID] = &in
	m.runtime[clientID] = &aschema.Runtime{CurrentFlow: in.FlowID, UpdatedAt: now, Version: "v1"}
	return nil
}

func (m *Manager) SetDefaultAgent(agentID, flowID string) error {
	m.mu.RLock()
	_, ok := m.agentClients[agentID]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}
	m.defaultAgID = agentID
	m.defaultFlowID = flowID
	return nil
}

// —— Flow 路由注册 —— //
func (m *Manager) RegisterFlowRoute(agentID, flowID string, spec *schemas.IntentSpec) error {
	if agentID == "" || flowID == "" || spec == nil {
		return fmt.Errorf("invalid route registration: agentID/flowID/spec is empty")
	}

	m.mu.RLock()
	_, ok := m.agentClients[agentID]
	m.mu.RUnlock()
	if !ok {
		return ErrNotFound
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	spec.FlowID = flowID
	m.routesByFlow[flowID] = routeRecord{AgentID: agentID, Spec: spec}
	if m.flowsByAgent[agentID] == nil {
		m.flowsByAgent[agentID] = map[string]struct{}{}
	}
	m.flowsByAgent[agentID][flowID] = struct{}{}
	return nil
}

func (m *Manager) UnregisterFlowRoute(agentID, flowID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 反查 agentID（允许传空）
	if agentID == "" {
		if rec, ok := m.routesByFlow[flowID]; ok {
			agentID = rec.AgentID
		}
	}
	delete(m.routesByFlow, flowID)
	if agentID != "" && m.flowsByAgent[agentID] != nil {
		delete(m.flowsByAgent[agentID], flowID)
		if len(m.flowsByAgent[agentID]) == 0 {
			delete(m.flowsByAgent, agentID)
		}
	}
}

func (m *Manager) GetIntentSpecByFlow(flowID string) (*schemas.IntentSpec, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if rec, ok := m.routesByFlow[flowID]; ok {
		return rec.Spec, rec.AgentID
	}
	return nil, ""
}

func (m *Manager) ListFlowRoutesByAgent(agentID string) []*schemas.IntentSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	set := m.flowsByAgent[agentID]
	if set == nil {
		return nil
	}
	out := make([]*schemas.IntentSpec, 0, len(set))
	for fid := range set {
		out = append(out, m.routesByFlow[fid].Spec)
	}
	return out
}

func (m *Manager) ClearFlowRoutesByAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if set := m.flowsByAgent[agentID]; set != nil {
		for fid := range set {
			delete(m.routesByFlow, fid)
		}
		delete(m.flowsByAgent, agentID)
	}
}

// —— 运行时状态（保留最小能力）—— //
func (m *Manager) UpdateStatus(id string, st aschema.AgentStatus, lastErr string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, ok := m.meta[id]
	if !ok {
		return ErrNotFound
	}
	meta.Status = st
	meta.UpdatedAt = time.Now()

	if lastErr != "" {
		if rt, ok := m.runtime[id]; ok {
			rt.LastError = lastErr
			rt.UpdatedAt = meta.UpdatedAt
		}
	}
	return nil
}

func (m *Manager) Heartbeat(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.meta[id]
	if !ok {
		return ErrNotFound
	}
	meta.LastBeatAt = time.Now()
	return nil
}

func (m *Manager) Get(id string) (*aschema.AgentInfo, *aschema.AgentMeta, *aschema.Runtime, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ag, ok := m.agentClients[id]
	if !ok {
		return nil, nil, nil, ErrNotFound
	}
	info := ag.GetInfo()
	return info, cloneMeta(m.meta[id]), cloneRT(m.runtime[id]), nil
}

// —— 工具 —— //
func cloneMeta(src *aschema.AgentMeta) *aschema.AgentMeta {
	if src == nil {
		return nil
	}
	m := *src
	if src.Extras != nil {
		cp := make(map[string]string, len(src.Extras))
		for k, v := range src.Extras {
			cp[k] = v
		}
		m.Extras = cp
	}
	return &m
}
func cloneRT(src *aschema.Runtime) *aschema.Runtime {
	if src == nil {
		return nil
	}
	r := *src
	return &r
}
