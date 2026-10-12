package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestExecutePlanWithHooksQueuesSameStageAtConfiguredConcurrency(t *testing.T) {
	m := NewAgentManager()
	var active atomic.Int32
	var peak atomic.Int32
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			seen := peak.Load()
			if current <= seen || peak.CompareAndSwap(seen, current) {
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
		return &SkillInvokeOutput{Status: "completed", ProtocolUsed: "skill", SkillID: in.SkillID, Result: map[string]any{"content": in.SkillID}}, nil
	})

	_, err := m.ExecutePlanWithHooks(context.Background(), flowschema.ExecutionPlan{
		PlanID: "plan-serial-stage",
		Tasks: []flowschema.PlanTask{
			{TaskID: "one", NodeKind: "skill", NodeRef: "skill.one", Stage: 1},
			{TaskID: "two", NodeKind: "skill", NodeRef: "skill.two", Stage: 1},
		},
	}, aschema.ExecutionMeta{TenantUUID: "tenant-test"}, &PlanExecutionHooks{MaxConcurrentTasks: 1})
	require.NoError(t, err)
	require.Equal(t, int32(1), peak.Load())
}

func TestExecutePlanWithHooksReportsContinuedFailure(t *testing.T) {
	m := NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		if in.SkillID == "skill.fail" {
			return nil, errors.New("upstream unavailable")
		}
		return &SkillInvokeOutput{
			TraceID:      "trace-report",
			Status:       "completed",
			ProtocolUsed: "skill",
			SkillID:      in.SkillID,
			Result:       map[string]any{"content": "completed"},
		}, nil
	})

	report, err := m.ExecutePlanWithHooks(context.Background(), flowschema.ExecutionPlan{
		PlanID: "plan-report",
		Tasks: []flowschema.PlanTask{
			{TaskID: "failed", NodeKind: "skill", NodeRef: "skill.fail", FailurePolicy: "continue", Stage: 0},
			{TaskID: "completed", NodeKind: "skill", NodeRef: "skill.ok", FailurePolicy: "fail-fast", Stage: 1},
		},
	}, aschema.ExecutionMeta{TenantUUID: "tenant-report", Metadata: map[string]any{"env": "test"}}, nil)
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.HasFailures())
	require.True(t, report.HasCompletedTasks())
	require.NotNil(t, report.FinalResult)
	require.Len(t, report.Tasks, 2)
	require.Equal(t, PlanTaskExecutionFailed, report.Tasks[0].Status)
	require.Equal(t, PlanTaskExecutionCompleted, report.Tasks[1].Status)
}

func TestExecuteNonWorkflowTask_SkillInvoker(t *testing.T) {
	m := NewAgentManager()
	var got []SkillInvokeInput
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		got = append(got, in)
		return &SkillInvokeOutput{
			TraceID:      "trace-skill-1",
			Status:       "completed",
			ProtocolUsed: "skill",
			FallbackUsed: false,
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result:       map[string]any{"echo": "hello"},
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-1",
		NodeKind: "skill",
		NodeRef:  "skill.thirdparty.hello-echo",
		Params: map[string]any{
			"text":    "hello",
			"version": "1.0.0",
		},
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{
		"text": "hello",
	}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-1",
		Metadata: map[string]any{
			"env": "dev",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Len(t, got, 1)
	require.Equal(t, "tenant-skill", got[0].TenantUUID)
	require.Equal(t, "dev", got[0].Env)
	require.Equal(t, "skill.thirdparty.hello-echo", got[0].SkillID)
	require.Equal(t, "1.0.0", got[0].Version)
	require.Equal(t, "hello", got[0].Payload["text"])
	require.Equal(t, "skill", out.Metadata["node_kind"])
	require.Equal(t, "skill.thirdparty.hello-echo", out.Metadata["node_ref"])
	require.Equal(t, "skill", out.Data["protocol_used"])
}

func TestPayloadFromTaskParamsIncludesResolvedParamRefs(t *testing.T) {
	task := flowschema.PlanTask{
		Params: map[string]any{
			"payload": map[string]any{"content": "source material"},
		},
		ParamRefs: map[string]string{
			"upstream_source_analysis": "{{task.source_analysis.output.result}}",
		},
	}
	upstream := map[string]any{"summary": "parsed"}
	payload := payloadFromTaskParams(task, flowschema.Context{
		"payload":                  map[string]any{"content": "source material"},
		"upstream_source_analysis": upstream,
	})
	require.Equal(t, "source material", payload["content"])
	require.Equal(t, upstream, payload["upstream_source_analysis"])
}

func TestExecuteNonWorkflowTask_SkillInvoker_ContentFromText(t *testing.T) {
	m := NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		return &SkillInvokeOutput{
			TraceID:      "trace-skill-content-text",
			Status:       "completed",
			ProtocolUsed: "skill",
			FallbackUsed: false,
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result:       map[string]any{"text": "INC-1001"},
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-content-text",
		NodeKind: "skill",
		NodeRef:  "skill.thirdparty.hello-echo",
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-content-text",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "INC-1001", out.Data["content"])
}

func TestExecuteNonWorkflowTask_SkillInvoker_ContentFromRenderedText(t *testing.T) {
	m := NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		return &SkillInvokeOutput{
			TraceID:      "trace-skill-content-rendered",
			Status:       "completed",
			ProtocolUsed: "skill",
			FallbackUsed: false,
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result:       map[string]any{"rendered_text": "事故 INC-1001 影响 华东支付，修复建议 先回滚 v2.3.7。"},
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-content-rendered",
		NodeKind: "skill",
		NodeRef:  "skill.thirdparty.prompt-template",
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-content-rendered",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "事故 INC-1001 影响 华东支付，修复建议 先回滚 v2.3.7。", out.Data["content"])
}

func TestExecuteNonWorkflowTask_SkillInvoker_ContextFromTaskParams(t *testing.T) {
	m := NewAgentManager()
	var got SkillInvokeInput
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		got = in
		return &SkillInvokeOutput{
			TraceID:      "trace-skill-ctx-1",
			Status:       "completed",
			ProtocolUsed: "skill",
			FallbackUsed: false,
			SkillID:      in.SkillID,
			Version:      "1.0.0",
			Result:       map[string]any{"ok": true},
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-ctx-1",
		NodeKind: "skill",
		NodeRef:  "incident-triage",
		Params: map[string]any{
			"context": "影响华东区支付接口，最近刚发布 v2.3.7，错误码 502 激增",
		},
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-ctx-1",
		Metadata: map[string]any{
			"env": "dev",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "影响华东区支付接口，最近刚发布 v2.3.7，错误码 502 激增", got.Context["context"])
}

func TestExecuteNonWorkflowTask_SkillInvokerPropagatesRuntimeLocale(t *testing.T) {
	m := NewAgentManager()
	var got SkillInvokeInput
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		got = in
		return &SkillInvokeOutput{
			TraceID:      "trace-skill-locale-1",
			Status:       "completed",
			ProtocolUsed: "skill",
			SkillID:      in.SkillID,
			Result:       map[string]any{"ok": true},
		}, nil
	})

	ctx := context.WithValue(context.Background(), "locale", "zh-CN")
	_, err := m.executeNonWorkflowTask(ctx, flowschema.PlanTask{
		TaskID:   "task-skill-locale-1",
		NodeKind: "skill",
		NodeRef:  "marketing.metric_extract",
	}, flowschema.Context{}, aschema.ExecutionMeta{TenantUUID: "tenant-skill", TraceID: "trace-skill-locale-1"})
	require.NoError(t, err)
	require.Equal(t, "zh-CN", got.Context["locale"])
}

func TestExecuteNonWorkflowTask_ToolingInvoker(t *testing.T) {
	m := NewAgentManager()
	var got ToolingInvokeInput
	m.SetToolingInvoker(func(ctx context.Context, in ToolingInvokeInput) (*ToolingInvokeOutput, error) {
		got = in
		return &ToolingInvokeOutput{
			TraceID:      "trace-tooling-1",
			Status:       "completed",
			ProtocolUsed: "http",
			FallbackUsed: false,
			Result:       map[string]any{"ok": true},
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-tooling-1",
		NodeKind: "tooling",
		NodeRef:  "capability.echo",
		Params: map[string]any{
			"payload": map[string]any{"text": "hello tooling"},
		},
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{
		"payload": map[string]any{"text": "hello tooling"},
	}, aschema.ExecutionMeta{
		TenantUUID: "tenant-tooling",
		TraceID:    "trace-tooling-1",
		Metadata: map[string]any{
			"env": "prod",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "tenant-tooling", got.TenantUUID)
	require.Equal(t, "prod", got.Env)
	require.Equal(t, "capability.echo", got.CapabilityID)
	require.Equal(t, "hello tooling", got.Payload["text"])
	require.Equal(t, "tooling", out.Metadata["node_kind"])
	require.Equal(t, "http", out.Data["protocol_used"])
}
