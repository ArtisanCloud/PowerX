// services/agent/manager_min.go
package agent

import (
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/handler"
	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/run_log"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"

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

	// —— 意图字段（方法挪到 manager_intent.go，需要这些字段作为状态） —— //
	intentStrategies []contract.IntentStrategy
	intentLow        float64
	intentHigh       float64
	plannerRules     []schemas.WireRule

	// Handler注册器
	handlerReg *handler.HandlerRegistry

	// 事件记录
	runLog run_log.RunLogger
}

// —— 单例 —— //
var (
	agentManager *Manager
	once         sync.Once
)

func NewAgentManager() *Manager {
	return &Manager{
		agentClients: make(map[string]contract.AgentClient),
		meta:         make(map[string]*aschema.AgentMeta),
		runtime:      make(map[string]*aschema.Runtime),
		routesByFlow: make(map[string]routeRecord),
		flowsByAgent: make(map[string]map[string]struct{}),
	}
}
func GetAgentManager() *Manager {
	once.Do(func() { agentManager = NewAgentManager() })
	return agentManager
}

func (m *Manager) SetRunLogger(l run_log.RunLogger) { m.runLog = l }
func (m *Manager) log() run_log.RunLogger {
	if m.runLog == nil {
		return &noopRunLogger{}
	}
	return m.runLog
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
