package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type EventSink interface {
	Emit(event string, payload any) error
}

type Engine struct {
	mgr *agent.Manager
}

func NewEngine() *Engine { return &Engine{mgr: agent.GetAgentManager()} }

func (e *Engine) Run(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow string, sink EventSink) error {
	// 1) 多意图识别
	tasks, err := e.mgr.DetectTasks(ctx, msg)
	if err != nil {
		return sink.Emit(dto.EventError, map[string]any{"message": "意图识别失败", "detail": err.Error()})
	}
	_ = sink.Emit(dto.EventIntent, map[string]any{"mode": "intent_multi", "tasks": tasks})

	// 2) 生成计划（强/弱类型兼容）
	rawPlan := e.mgr.BuildPlan(tasks) // FIX: 原来写成了 BuildPl
	plan, ok := NormalizeExecPlan(rawPlan)
	_ = sink.Emit(dto.EventPlan, PlanOrRaw(plan, rawPlan))

	// 先根据 plan 拿一个候选 flowID（按 stage 最小、原序）
	var flowID string
	if ok && len(plan.Tasks) > 0 {
		flowID = plan.Tasks[0].FlowID
	}

	// 若调用方显式传了 flow，优先生效；否则再按“第一条任务”兜底
	flowID = explicitFlow // FIX: 原来用 := 重新定义，导致覆盖问题
	if flowID == "" {
		flowID = PickFirstFlowID(plan)
	}

	// 3) 路由 & 执行
	ag, fallbackFlowID, err := e.mgr.GetDefaultRoute()
	if err != nil {
		return sink.Emit(dto.EventError, map[string]any{"message": "创建 Agent 失败", "detail": err.Error()})
	}
	if flowID == "" {
		flowID = fallbackFlowID
	}

	execID := fmt.Sprintf("exec_%d", time.Now().UnixNano())
	_ = sink.Emit(dto.EventStart, map[string]any{"flow_id": flowID, "execution_id": execID})

	sr, err := ag.Stream(ctx, flowID, flowschema.Context{
		"message": msg,
		"config":  reqCfg,
	}, agentschema.ExecutionMeta{
		RequestID: execID,
		Timeout:   60,
		Metadata:  map[string]any{"transport": "engine"},
	})
	if err != nil {
		return sink.Emit(dto.EventError, map[string]any{"message": "流式聊天执行失败", "detail": err.Error()})
	}

	// 4) 转发流事件
	defer sr.Close()
	for {
		ch, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
				return nil
			}
			_ = sink.Emit(dto.EventError, map[string]any{"message": err.Error()})
			_ = sink.Emit(dto.EventEnd, map[string]any{"success": false})
			return err
		}

		if delta, ok := ch.Metadata["delta_text"].(string); ok && delta != "" {
			_ = sink.Emit(dto.EventToken, map[string]any{"delta": delta, "step_id": ch.StepID, "timestamp": ch.Timestamp})
			continue
		}

		_ = sink.Emit(dto.EventData, map[string]any{
			"success":   ch.Success,
			"data":      ch.Data,
			"step_id":   ch.StepID,
			"timestamp": ch.Timestamp,
			"metadata":  ch.Metadata,
		})

		if isFinal, _ := ch.Metadata["is_final"].(bool); isFinal {
			_ = sink.Emit(dto.EventFinal, map[string]any{
				"success":   ch.Success,
				"data":      ch.Data,
				"metadata":  ch.Metadata,
				"timestamp": ch.Timestamp,
			})
			_ = sink.Emit(dto.EventEnd, map[string]any{"success": true})
			return nil
		}
	}
}

func PickFirstFlowID(plan *flowschema.ExecutionPlan) string {
	if plan == nil || len(plan.Tasks) == 0 {
		return ""
	}
	tasks := make([]flowschema.PlanTask, len(plan.Tasks))
	copy(tasks, plan.Tasks)

	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Stage == tasks[j].Stage {
			return i < j
		}
		return tasks[i].Stage < tasks[j].Stage
	})

	if id := strings.TrimSpace(tasks[0].FlowID); id != "" {
		return id
	}
	if id := strings.TrimSpace(tasks[0].TaskID); id != "" {
		return id
	}
	return ""
}

func ExtractAssistantText(chunk *agentschema.ExecutionResult) string {
	if chunk == nil || chunk.Data == nil {
		return ""
	}
	if res, ok := chunk.Data["result"].(map[string]any); ok {
		if s, ok := res["content"].(string); ok {
			return s
		}
	}
	if s, ok := chunk.Data["content"].(string); ok {
		return s
	}
	return ""
}
