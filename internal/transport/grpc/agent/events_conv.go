// internal/server/agent/transport/grpc/events_conv.go
package agentgrpc

import (
	"encoding/json"
	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/grpc"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func ToStreamEvent(env dto.WSEnvelope) (*agentv1.StreamResponse, error) {
	var m map[string]any
	if len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, &m); err != nil {
			return nil, err
		}
	} else {
		m = map[string]any{}
	}

	// 把 flow/exec/step 从 data 里提取到顶层（如果存在）
	flowID := utils.GetString(m, "flow_id")
	execID := utils.GetString(m, "execution_id")
	stepID := utils.GetString(m, "step_id")

	resp := &agentv1.StreamResponse{
		Type:        strings.TrimSpace(env.Type),
		FlowId:      flowID,
		ExecutionId: execID,
		StepId:      stepID,
		Timestamp:   env.Timestamp,
	}

	switch resp.Type {
	case dto.EventStart:
		resp.Payload = &agentv1.StreamResponse_EvStart{
			EvStart: &agentv1.EventStart{Message: utils.GetString(m, "message")},
		}
	case dto.EventToken:
		resp.Payload = &agentv1.StreamResponse_EvToken{
			EvToken: &agentv1.EventToken{Delta: utils.GetString(m, "delta")},
		}
	case dto.EventPlan:
		resp.Payload = &agentv1.StreamResponse_EvPlan{
			EvPlan: &agentv1.EventPlan{Plan: grpc.MustStructFromAny(m)},
		}
	case dto.EventIntent:
		resp.Payload = &agentv1.StreamResponse_EvIntent{
			EvIntent: &agentv1.EventIntent{Tasks: grpc.MustStructFromAny(m)},
		}
	case dto.EventFinal:
		resp.Payload = &agentv1.StreamResponse_EvFinal{
			EvFinal: &agentv1.EventFinal{Data: grpc.MustStructFromAny(m)},
		}
	case dto.EventEnd:
		resp.Payload = &agentv1.StreamResponse_EvEnd{
			EvEnd: &agentv1.EventEnd{Message: utils.GetString(m, "message")},
		}
	case dto.EventError:
		resp.Payload = &agentv1.StreamResponse_EvError{
			EvError: &agentv1.EventError{
				Code:    utils.GetString(m, "code"),
				Message: utils.GetString(m, "message"),
				Detail:  grpc.MustStructFromAny(utils.GetMap(m, "detail")),
			},
		}
	case dto.EventHeartbeat:
		ts := utils.GetInt64(m, "ts")
		if ts == 0 {
			ts = env.Timestamp
		}
		resp.Payload = &agentv1.StreamResponse_EvHeartbeat{
			EvHeartbeat: &agentv1.EventHeartbeat{Ts: ts},
		}
	case "meta":
		resp.Payload = &agentv1.StreamResponse_EvMeta{
			EvMeta: &agentv1.EventMeta{
				SessionId: uint64(utils.GetInt64(m, "session_id")),
				AgentId:   uint64(utils.GetInt64(m, "agent_id")),
				Extra:     grpc.MustStructFromAny(m),
			},
		}
	default:
		// 未识别类型：当作 data 事件降级
		resp.Type = "data"
		resp.Payload = &agentv1.StreamResponse_EvData{
			EvData: &agentv1.EventData{Data: grpc.MustStructFromAny(m)},
		}
	}

	// 清理：避免 data 中的 flow/exec/step 再次出现在客户端（可选）
	delete(m, "flow_id")
	delete(m, "execution_id")
	delete(m, "step_id")

	return resp, nil
}

// ========== gRPC -> WS ==========

func ToWSEnvelope(ev *agentv1.StreamResponse) (dto.WSEnvelope, error) {
	m := map[string]any{}

	// 把顶层 flow/exec/step 合并到 data 中（与历史 WS 负载保持一致）
	if ev.GetFlowId() != "" {
		m["flow_id"] = ev.GetFlowId()
	}
	if ev.GetExecutionId() != "" {
		m["execution_id"] = ev.GetExecutionId()
	}
	if ev.GetStepId() != "" {
		m["step_id"] = ev.GetStepId()
	}

	switch p := ev.Payload.(type) {
	case *agentv1.StreamResponse_EvStart:
		if p.EvStart != nil && p.EvStart.Message != "" {
			m["message"] = p.EvStart.Message
		}
	case *agentv1.StreamResponse_EvToken:
		if p.EvToken != nil && p.EvToken.Delta != "" {
			m["delta"] = p.EvToken.Delta
		}
	case *agentv1.StreamResponse_EvPlan:
		grpc.MergeStruct(m, p.EvPlan.GetPlan())
	case *agentv1.StreamResponse_EvIntent:
		grpc.MergeStruct(m, p.EvIntent.GetTasks())
	case *agentv1.StreamResponse_EvData:
		grpc.MergeStruct(m, p.EvData.GetData())
	case *agentv1.StreamResponse_EvFinal:
		grpc.MergeStruct(m, p.EvFinal.GetData())
	case *agentv1.StreamResponse_EvEnd:
		if p.EvEnd != nil && p.EvEnd.Message != "" {
			m["message"] = p.EvEnd.Message
		}
	case *agentv1.StreamResponse_EvError:
		if p.EvError != nil {
			m["code"] = p.EvError.Code
			m["message"] = p.EvError.Message
			if p.EvError.Detail != nil {
				m["detail"] = p.EvError.Detail.AsMap()
			}
		}
	case *agentv1.StreamResponse_EvHeartbeat:
		if p.EvHeartbeat != nil && p.EvHeartbeat.Ts > 0 {
			m["ts"] = p.EvHeartbeat.Ts
		}
	case *agentv1.StreamResponse_EvMeta:
		if p.EvMeta != nil {
			if p.EvMeta.SessionId > 0 {
				m["session_id"] = p.EvMeta.SessionId
			}
			if p.EvMeta.AgentId > 0 {
				m["agent_id"] = p.EvMeta.AgentId
			}
			if p.EvMeta.Extra != nil {
				// Extra 合并进去，避免嵌套过深
				for k, v := range p.EvMeta.Extra.AsMap() {
					m[k] = v
				}
			}
		}
	default:
		// 没有 payload：保持为空对象
	}

	var raw json.RawMessage
	if len(m) > 0 {
		b, err := json.Marshal(m)
		if err != nil {
			return dto.WSEnvelope{}, err
		}
		raw = b
	}

	return dto.WSEnvelope{
		Type:      ev.GetType(),
		Data:      raw,
		Timestamp: ev.GetTimestamp(),
	}, nil
}
