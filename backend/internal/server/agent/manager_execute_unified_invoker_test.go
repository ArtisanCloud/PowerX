package agent

import (
	"context"
	"testing"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestExecuteNonWorkflowTask_SkillInvoker(t *testing.T) {
	m := NewAgentManager()
	var got []SkillInvokeInput
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		got = append(got, in)
		if in.Entrypoint == "prepare" {
			return &SkillInvokeOutput{
				TraceID:        "trace-skill-1",
				Status:         "completed",
				ProtocolUsed:   "skill.prepare",
				SkillID:        in.SkillID,
				Version:        "1.0.0",
				ReadyToExecute: true,
				CapabilityID:   "capability.echo",
				CapabilityPayload: map[string]any{
					"text": "hello",
				},
			}, nil
		}
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
	require.Len(t, got, 2)
	require.Equal(t, "prepare", got[0].Entrypoint)
	require.Equal(t, "tenant-skill", got[1].TenantUUID)
	require.Equal(t, "dev", got[1].Env)
	require.Equal(t, "skill.thirdparty.hello-echo", got[1].SkillID)
	require.Equal(t, "1.0.0", got[1].Version)
	require.Equal(t, "hello", got[1].Payload["text"])
	require.Equal(t, "skill", out.Metadata["node_kind"])
	require.Equal(t, "skill.thirdparty.hello-echo", out.Metadata["node_ref"])
	require.Equal(t, "skill", out.Data["protocol_used"])
}

func TestExecuteNonWorkflowTask_SkillPrepareAwaitingParams(t *testing.T) {
	m := NewAgentManager()
	calls := 0
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		calls++
		require.Equal(t, "prepare", in.Entrypoint)
		return &SkillInvokeOutput{
			TraceID:        "trace-skill-awaiting",
			Status:         "awaiting_params",
			ProtocolUsed:   "skill.prepare",
			SkillID:        in.SkillID,
			Version:        "1.0.0",
			ReadyToExecute: false,
			MissingFields:  []string{"template.content"},
			StatePatch: map[string]any{
				"template": map[string]any{"title": "测试模板"},
			},
			Message: "请补充内容。",
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-awaiting",
		NodeKind: "skill",
		NodeRef:  "powerxplugin.template.basic",
		Params: map[string]any{
			"action": "create",
			"template": map[string]any{
				"title": "测试模板",
			},
		},
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-awaiting",
		Metadata:   map[string]any{"env": "dev"},
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, 1, calls)
	require.False(t, out.Success)
	require.Equal(t, "awaiting_params", out.Data["status"])
	require.Equal(t, []string{"template.content"}, out.Data["missing_fields"])
}

func TestExecuteNonWorkflowTask_SkillPrepareReadyRequiresCapabilityRequest(t *testing.T) {
	m := NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		require.Equal(t, "prepare", in.Entrypoint)
		return &SkillInvokeOutput{
			TraceID:        "trace-skill-ready-invalid",
			Status:         "completed",
			ProtocolUsed:   "skill.prepare",
			SkillID:        in.SkillID,
			ReadyToExecute: true,
		}, nil
	})

	task := flowschema.PlanTask{
		TaskID:   "task-skill-ready-invalid",
		NodeKind: "skill",
		NodeRef:  "powerxplugin.template.basic",
		Params:   map[string]any{"action": "create"},
	}
	out, err := m.executeNonWorkflowTask(context.Background(), task, flowschema.Context{}, aschema.ExecutionMeta{
		TenantUUID: "tenant-skill",
		TraceID:    "trace-skill-ready-invalid",
		Metadata:   map[string]any{"env": "dev"},
	})
	require.Error(t, err)
	require.Nil(t, out)
	require.Contains(t, err.Error(), "capability_request.capability_id")
}

func TestExecuteNonWorkflowTask_SkillInvoker_ContentFromText(t *testing.T) {
	m := NewAgentManager()
	m.SetSkillInvoker(func(ctx context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		if in.Entrypoint == "prepare" {
			return readySkillPrepareOutput(in, "trace-skill-content-text"), nil
		}
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
		if in.Entrypoint == "prepare" {
			return readySkillPrepareOutput(in, "trace-skill-content-rendered"), nil
		}
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
		if in.Entrypoint == "prepare" {
			return readySkillPrepareOutput(in, "trace-skill-ctx-1"), nil
		}
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

func readySkillPrepareOutput(in SkillInvokeInput, traceID string) *SkillInvokeOutput {
	return &SkillInvokeOutput{
		TraceID:        traceID,
		Status:         "completed",
		ProtocolUsed:   "skill.prepare",
		FallbackUsed:   false,
		SkillID:        in.SkillID,
		Version:        "1.0.0",
		ReadyToExecute: true,
		CapabilityID:   "capability.test",
		CapabilityPayload: map[string]any{
			"prepared": true,
		},
	}
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
