package eino

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	einoCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino/config"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func TestExecLLM_MissingLLMConfigDoesNotPanic(t *testing.T) {
	a := &AgentClient{
		config: &config.AgentConfig{
			LLMConfig: nil,
		},
	}

	node := &flowschema.Node{
		Kind:   flowschema.KindLLM,
		Params: map[string]any{},
	}
	in := flowschema.Context{"message": "hello"}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()

	_, err := execLLM(a)(context.Background(), a, "base_flow", node, in, agentschema.ExecutionMeta{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveLLMConfig_PriorityOrder(t *testing.T) {
	a := &AgentClient{
		config: &config.AgentConfig{
			LLMConfig: &einoCfg.ModelConfig{
				Provider: "hunyuan",
				Model:    "agent-model",
			},
		},
	}

	node := &flowschema.Node{
		Kind: flowschema.KindLLM,
		Params: map[string]any{
			"model": "node-model",
		},
	}
	in := flowschema.Context{
		"message": "hello",
		"config": &dto.ChatConfig{
			Provider:  "ollama",
			ModelName: "req-model",
		},
	}

	mc, err := resolveLLMConfig(node, in, a)
	if err != nil {
		t.Fatalf("resolveLLMConfig: %v", err)
	}
	// node params highest: model from node, provider from request
	if mc.Provider != "ollama" {
		t.Fatalf("provider mismatch: %s", mc.Provider)
	}
	if mc.Model != "node-model" {
		t.Fatalf("model mismatch: %s", mc.Model)
	}
}

func TestStream_ExecutorPanicIsRecovered(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	// NewAgentClient 需要目录存在
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}

	cli, err := NewAgentClient(&config.AgentConfig{
		FlowSpec: config.FlowSpecConfig{
			BusinessDir: tmpDir,
		},
		LLMConfig: nil,
	})
	if err != nil {
		t.Fatalf("NewAgentClient: %v", err)
	}
	a := cli.(*AgentClient)

	// 注入一个会 panic 的 executor
	a.execMu.Lock()
	a.kindExecs["panic"] = func(ctx context.Context, a *AgentClient, curFlowID string, node *flowschema.Node, in flowschema.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
		panic("boom")
	}
	a.execMu.Unlock()

	// 缓存一个包含 panic 节点的 flow，绕过 resolver
	a.flowMu.Lock()
	a.flowCache["panic_flow"] = &flowschema.Flow{
		FlowID: "panic_flow",
		Nodes: []*flowschema.Node{
			{Kind: flowschema.NodeKind("panic"), Params: map[string]any{}},
		},
	}
	a.flowMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sr, err := a.Stream(ctx, "panic_flow", flowschema.Context{"message": "hi"}, agentschema.ExecutionMeta{RequestID: "test"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	defer sr.Close()

	// 先读 start
	_, err = sr.Recv()
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected error before panic: %v", err)
	}

	// 随后应该收到 panic 转换成的 error（而不是进程崩溃）
	for {
		_, recvErr := sr.Recv()
		if errors.Is(recvErr, io.EOF) {
			t.Fatalf("expected panic error, got EOF")
		}
		if recvErr == nil {
			continue
		}
		if !strings.Contains(recvErr.Error(), "panic in stream") {
			t.Fatalf("unexpected error: %v", recvErr)
		}
		break
	}
}
