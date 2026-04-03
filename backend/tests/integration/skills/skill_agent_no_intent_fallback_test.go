package skillsintegration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	agentpkg "github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

type noIntentStubAgent struct{}

func (s *noIntentStubAgent) GetInfo() *agentschema.AgentInfo { return &agentschema.AgentInfo{Name: "no-intent-stub"} }
func (s *noIntentStubAgent) ListFlows(context.Context, agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
	return nil, nil
}
func (s *noIntentStubAgent) GetFlowInfo(context.Context, string, agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
	return nil, nil
}
func (s *noIntentStubAgent) ValidateParams(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) error {
	return nil
}
func (s *noIntentStubAgent) Invoke(_ context.Context, flowID string, _ flowschema.Context, _ agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	return &agentschema.ExecutionResult{
		Success: true,
		Data: flowschema.Result{
			"content": "fallback-llm-reply",
			"flow_id": flowID,
		},
		Metadata: flowschema.Result{
			"is_final": true,
		},
	}, nil
}
func (s *noIntentStubAgent) InvokeAsync(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) (string, error) {
	return "", nil
}
func (s *noIntentStubAgent) Stream(context.Context, string, flowschema.Context, agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
	return nil, nil
}
func (s *noIntentStubAgent) GetExecutionStatus(context.Context, string, agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	return nil, nil
}
func (s *noIntentStubAgent) CancelExecution(context.Context, string, agentschema.ExecutionMeta) error { return nil }
func (s *noIntentStubAgent) GetExecutionResult(context.Context, string, agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	return nil, nil
}
func (s *noIntentStubAgent) GetMetrics(context.Context, agentschema.ExecutionMeta) (flowschema.Result, error) {
	return flowschema.Result{}, nil
}
func (s *noIntentStubAgent) Health(context.Context) error   { return nil }
func (s *noIntentStubAgent) Shutdown(context.Context) error { return nil }

var _ contract.AgentClient = (*noIntentStubAgent)(nil)

type captureSink struct {
	mu     sync.Mutex
	events []string
}

func (s *captureSink) Emit(event string, payload any) error {
	_ = payload
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *captureSink) count(event string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.events {
		if e == event {
			n++
		}
	}
	return n
}

func TestSkillAgentNoIntentFallbackToNormalReply(t *testing.T) {
	m := agentpkg.GetAgentManager()
	m.SetIntentStrategies(nil, 0.6, 0.95)

	agentID := fmt.Sprintf("agent.no-intent.%d", time.Now().UnixNano())
	flowID := "flow.chat.fallback"
	err := m.Register(agentID, &noIntentStubAgent{}, &agentschema.AgentMeta{FlowID: flowID})
	require.NoError(t, err)
	require.NoError(t, m.SetDefaultAgent(agentID, flowID))

	sink := &captureSink{}
	out, plan, err := runtime.NewEngine().RunPlanInvoke(context.Background(), "%%% no intent sentence %%%", nil, "", sink)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Nil(t, plan)

	require.GreaterOrEqual(t, sink.count(dto.EventIntent), 1)
	require.GreaterOrEqual(t, sink.count(dto.EventFinal), 1)
	require.Equal(t, 0, sink.count(dto.EventNodeStart))
	require.Equal(t, 0, sink.count(dto.EventNodeEnd))
}

