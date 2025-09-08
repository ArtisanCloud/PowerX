package agentgrpc

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils/grpc"
	"strings"
	"time"

	v1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

/******** server ********/
type AgentStreamServer struct {
	v1.UnimplementedAgentStreamServiceServer
	his *agentSvc.ChatHistoryService
}

func NewAgentStreamServer(deps *shared.Deps) *AgentStreamServer {
	return &AgentStreamServer{his: agentSvc.NewChatHistoryService(deps.DB)}
}

/******** Stream（真实流，对齐 StreamSSE） ********/
func (s *AgentStreamServer) Stream(req *v1.StreamRequest, srv v1.AgentStreamService_StreamServer) error {
	ctx := srv.Context()

	// 1) 校验与解析（同 StreamSSE）
	env := strings.TrimSpace(req.GetEnv())
	if env == "" {
		if e := strings.TrimSpace(reqctx.GetEnv(ctx)); e != "" {
			env = e
		} else {
			env = "default"
		}
	}

	msg := strings.TrimSpace(utils.FirstNonEmpty(req.GetMessage()))
	if msg == "" {
		return status.Error(codes.InvalidArgument, "message is required")
	}
	agentID := req.GetAgentId()

	// tenant：优先 request.ctx.tenant_id，其次 JWT/context
	var tenantID *uint64
	if req.GetCtx() != nil && req.GetCtx().GetTenantId() > 0 {
		t := uint64(req.GetCtx().GetTenantId())
		tenantID = &t
	} else if tid := reqctx.GetTenantID(ctx); tid > 0 {
		t := tid
		tenantID = &t
	}

	// 2) 会话：session_id 优先，否则 sticky（与 StreamSSE 等同）
	var sess *dbmodel.AgentChatSession
	if sid := req.GetSessionId(); sid > 0 {
		sess, _ = s.his.FindSessionByID(ctx, env, tenantID, sid)
	}
	if sess == nil {
		uid := reqctx.GetUserID(ctx) // uint64 from JWT/context
		var err error
		sess, err = s.his.GetOrCreateSession(ctx, env, tenantID, agentID, uid, false, nil)
		if err != nil {
			return status.Errorf(codes.Internal, "创建会话失败: %v", err)
		}
	}

	// 3) 写入 user 消息
	_, _ = s.his.AppendMessage(ctx, env, tenantID, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil)

	// 4) 首帧 meta（与 SSE 首帧的 meta 语义一致）
	_ = srv.Send(&v1.StreamResponse{
		Type:      "meta",
		Timestamp: time.Now().Unix(),
		Payload: &v1.StreamResponse_EvMeta{
			EvMeta: &v1.EventMeta{
				SessionId: sess.ID,
				AgentId:   agentID,
				Extra:     grpc.MustStruct(nil),
			},
		},
	})

	// 5) 引擎执行：事件 → gRPC（薄转发），并做 token 聚合 + final 入库（对齐 SSE 的 HistorySink）
	sink := &grpcEventSink{srv: srv}
	hist := newTokenHistory(s.his, ctx, env, tenantID, sess, agentID)
	engineSink := eventChain(sink, hist)

	cfg := &dto.ChatConfig{} // 与 HTTP 保持一致
	return runtime.NewEngine().Run(ctx, msg, cfg, strings.TrimSpace(req.GetFlowId()), engineSink)
}

/******** Simulate（等价 SimulateSSE） ********/
func (s *AgentStreamServer) Simulate(req *v1.SimulateRequest, srv v1.AgentStreamService_SimulateServer) error {
	ctx := srv.Context()

	now := func() int64 { return time.Now().Unix() }

	// 探针
	if req.GetProbe() {
		_ = srv.Send(&v1.SimulateResponse{
			Type:      dto.EventStart,
			Timestamp: now(),
			Payload:   &v1.SimulateResponse_EvStart{EvStart: &v1.EventStart{Message: "probe ok"}},
		})
		_ = srv.Send(&v1.SimulateResponse{
			Type:      dto.EventEnd,
			Timestamp: now(),
			Payload:   &v1.SimulateResponse_EvEnd{EvEnd: &v1.EventEnd{Message: "ok"}},
		})
		return nil
	}

	// 默认文本（与 HTTP 的中文段落一致）
	text := strings.TrimSpace(utils.FirstNonEmpty(req.GetText(),
		`<think>这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。
这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。
这是一个 SSE 模拟流，前端可以用它测试逐字渲染与事件解析。</think> 
这是think后，完成的结论1,
这是think后，完成的结论2,
这是think后，完成的结论3,
这是think后，完成的结论4,
这是think后，完成的结论5
`))
	chunk := req.GetChunk()
	if chunk <= 0 {
		chunk = 1
	}
	delay := time.Duration(utils.MaxInt(int(req.GetDelayMs()), 0)) * time.Millisecond

	flowID := "mock_flow"
	execID := fmt.Sprintf("mock_%d", time.Now().UnixNano())
	stepID := "mock"

	// start
	_ = srv.Send(&v1.SimulateResponse{
		Type:        dto.EventStart,
		FlowId:      flowID,
		ExecutionId: execID,
		Timestamp:   now(),
		Payload:     &v1.SimulateResponse_EvStart{EvStart: &v1.EventStart{Message: "开始模拟 SSE 输出"}},
	})

	// heartbeat
	hb := time.NewTicker(25 * time.Second)
	defer hb.Stop()

	// runes chunk
	rs := []rune(text)
	for i := 0; i < len(rs); i += int(chunk) {
		select {
		case <-ctx.Done():
			_ = srv.Send(&v1.SimulateResponse{
				Type:      "error",
				Timestamp: now(),
				Payload:   &v1.SimulateResponse_EvError{EvError: &v1.EventError{Code: "canceled", Message: ctx.Err().Error()}},
			})
			_ = srv.Send(&v1.SimulateResponse{
				Type:      "end",
				Timestamp: now(),
				Payload:   &v1.SimulateResponse_EvEnd{EvEnd: &v1.EventEnd{Message: "连接已中断"}},
			})
			return nil
		case <-hb.C:
			_ = srv.Send(&v1.SimulateResponse{
				Type:      "heartbeat",
				Timestamp: now(),
				Payload:   &v1.SimulateResponse_EvHeartbeat{EvHeartbeat: &v1.EventHeartbeat{Ts: now()}},
			})
			i -= int(chunk) // 心跳不消耗文本
			continue
		default:
		}

		j := i + int(chunk)
		if j > len(rs) {
			j = len(rs)
		}
		delta := string(rs[i:j])

		_ = srv.Send(&v1.SimulateResponse{
			Type:      dto.EventToken,
			StepId:    stepID,
			Timestamp: now(),
			Payload:   &v1.SimulateResponse_EvToken{EvToken: &v1.EventToken{Delta: delta}},
		})

		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
			}
		}
	}

	// final + end
	_ = srv.Send(&v1.SimulateResponse{
		Type:      dto.EventFinal,
		Timestamp: now(),
		Payload: &v1.SimulateResponse_EvFinal{EvFinal: &v1.EventFinal{Data: grpc.MustStruct(map[string]any{
			"success":   true,
			"data":      map[string]any{"content": text},
			"metadata":  map[string]any{"mock": true},
			"timestamp": now(),
		})}},
	})
	_ = srv.Send(&v1.SimulateResponse{
		Type:      dto.EventEnd,
		Timestamp: now(),
		Payload:   &v1.SimulateResponse_EvEnd{EvEnd: &v1.EventEnd{Message: "SSE 模拟完成"}},
	})
	return nil
}

/******** 事件转发（薄适配） ********/
// 只做“事件→gRPC发送”；不关心历史、摘要等（交给 tokenHistory）
type grpcEventSink struct {
	srv v1.AgentStreamService_StreamServer
}

func (g *grpcEventSink) Emit(event string, payload any) error {
	ts := time.Now().Unix()

	switch event {
	case dto.EventStart:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventStart,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvStart{EvStart: &v1.EventStart{Message: utils.GetString(payload, "message")}},
		})
	case dto.EventToken:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventToken,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvToken{EvToken: &v1.EventToken{Delta: utils.GetString(payload, "delta")}},
		})
	case dto.EventPlan:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventPlan,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvPlan{EvPlan: &v1.EventPlan{Plan: grpc.MustStructFromAny(payload)}},
		})
	case dto.EventIntent:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventIntent,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvIntent{EvIntent: &v1.EventIntent{Tasks: grpc.MustStructFromAny(payload)}},
		})
	case dto.EventFinal:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventFinal,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvFinal{EvFinal: &v1.EventFinal{Data: grpc.MustStructFromAny(payload)}},
		})
	case dto.EventEnd:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventEnd,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvEnd{EvEnd: &v1.EventEnd{Message: utils.GetString(payload, "message")}},
		})
	case dto.EventHeartbeat:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventHeartbeat,
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvHeartbeat{EvHeartbeat: &v1.EventHeartbeat{Ts: ts}},
		})
	case dto.EventError:
		return g.srv.Send(&v1.StreamResponse{
			Type:      dto.EventError,
			Timestamp: ts,
			Payload: &v1.StreamResponse_EvError{EvError: &v1.EventError{
				Code:    utils.GetString(payload, "code"),
				Message: utils.GetString(payload, "message"),
				Detail:  grpc.MustStructFromAny(utils.GetAny(payload, "detail")),
			}},
		})
	default:
		// 兜底当作 data 事件
		return g.srv.Send(&v1.StreamResponse{
			Type:      "data",
			Timestamp: ts,
			Payload:   &v1.StreamResponse_EvData{EvData: &v1.EventData{Data: grpc.MustStructFromAny(payload)}},
		})
	}
}

// 只做 token 聚合 + final 入库（与 SSE 的 HistorySink 语义对齐）
type tokenHistory struct {
	his      *agentSvc.ChatHistoryService
	ctx      context.Context
	env      string
	tenantID *uint64
	sess     *dbmodel.AgentChatSession
	agentID  uint64
	buf      strings.Builder
}

func newTokenHistory(h *agentSvc.ChatHistoryService, ctx context.Context, env string, tid *uint64, sess *dbmodel.AgentChatSession, agentID uint64) *tokenHistory {
	return &tokenHistory{his: h, ctx: ctx, env: env, tenantID: tid, sess: sess, agentID: agentID}
}
func (h *tokenHistory) Emit(event string, payload any) error {
	switch event {
	case dto.EventToken:
		if s := utils.GetString(payload, "delta"); s != "" {
			h.buf.WriteString(s)
		}
	case dto.EventFinal:
		txt := strings.TrimSpace(h.buf.String())
		if txt != "" && h.sess != nil {
			_, _ = h.his.AppendMessage(h.ctx, h.env, h.tenantID, h.sess.ID, h.agentID, "assistant", txt, "text", 0, 0, false, nil)
			_, _ = h.his.SummarizeIfNeeded(h.ctx, h.env, h.tenantID, h.sess)
		}
	}
	return nil
}

// 把 Engine 事件链到多个 sink（顺序：先发到 gRPC，再聚合历史）
type chainedSink []interface{ Emit(string, any) error }

func eventChain(sinks ...interface{ Emit(string, any) error }) chainedSink { return sinks }
func (c chainedSink) Emit(ev string, p any) error {
	for _, s := range c {
		_ = s.Emit(ev, p)
	}
	return nil
}
