package agentgrpc

// internal/server/agent/transport/grpc/stream_handler.go

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	v1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/runtime"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

/* ===================== Server ===================== */

type AgentStreamServer struct {
	UnimplementedAgentStreamServiceServer

	his *agentSvc.ChatHistoryService
}

func NewAgentStreamServer(deps *shared.Deps) *AgentStreamServer {
	return &AgentStreamServer{
		his: agentSvc.NewChatHistoryService(deps.DB),
	}
}

/* ============ gRPC 入参到内部模型的轻量映射 ============ */

// 如果你的 proto 里定义了 config/route/exec，请在此补齐映射
func toChatConfig(_ *v1.ChatConfig) *dto.ChatConfig {
	if _ == nil {
		return &dto.ChatConfig{}
	}
	// TODO: 按需要把字段映射到 dto.ChatConfig
	return &dto.ChatConfig{}
}

/* ===================== gRPC Stream =====================

proto（示意）:
service AgentStreamService {
  rpc StreamChat(StreamChatRequest) returns (stream StreamEvent);
}

message StreamChatRequest {
  string env        = 1;
  uint64 tenant_id  = 2;  // 可选：若你从 metadata 拿，也可不传
  uint64 agent_id   = 3;
  uint64 session_id = 4;  // 可选：不传则 sticky 创建
  string message    = 5;
  ChatConfig config = 6;  // 可选
}

message StreamEvent {
  string type              = 1;   // "start"|"intent"|...
  google.protobuf.Struct data = 2;   // payload
  int64  ts                = 3;   // utc seconds
}
======================================================== */

func (s *AgentStreamServer) StreamChat(req *v1.StreamChatRequest, srv v1.AgentStreamService_StreamChatServer) error {
	ctx := srv.Context()

	// 1) 校验入参
	env := strings.TrimSpace(req.GetEnv())
	if env == "" {
		env = "default"
	}
	msg := strings.TrimSpace(req.GetMessage())
	if msg == "" {
		return status.Error(codes.InvalidArgument, "message is required")
	}

	agentID := req.GetAgentId()
	var tenantID *uint64
	if req.GetTenantId() > 0 {
		t := uint64(req.GetTenantId())
		tenantID = &t
	}

	// 2) 会话：优先用传入的；否则 sticky 获取/创建
	var sess *dbmodel.AgentChatSession
	var err error
	if sid := req.GetSessionId(); sid > 0 {
		sess, err = s.his.GetSessionByID(ctx, env, tenantID, sid)
		if err != nil {
			return status.Errorf(codes.NotFound, "session not found: %v", err)
		}
	} else {
		// 这里的 userID 你可以从 metadata 中解析；演示用空串
		sess, err = s.his.GetOrCreateSession(ctx, env, tenantID, agentID, "", false, nil)
		if err != nil {
			return status.Errorf(codes.Internal, "create session failed: %v", err)
		}
	}

	// 3) 写入 user 消息
	_, _ = s.his.AppendMessage(ctx, env, tenantID, sess.ID, agentID, "user", msg, "text", 0, 0, false, nil)

	// 4) 事件 sink：gRPC + 历史装饰
	baseSink := &grpcSink{srv: srv}
	histSink := &grpcHistorySink{
		next:     baseSink,
		svc:      s.his,
		ctx:      ctx,
		env:      env,
		tenantID: tenantID,
		session:  sess,
		agentID:  agentID,
	}

	// 5) 调用引擎
	cfg := toChatConfig(req.GetConfig())
	return runtime.NewEngine().Run(ctx, msg, cfg, "", histSink)
}

/* ===================== sinks ===================== */

// 事件下沉：把 runtime 的统一事件，写成 gRPC 的 StreamEvent
type grpcSink struct {
	srv v1.AgentStreamService_StreamChatServer
}

func (g *grpcSink) Emit(event string, payload any) error {
	st, err := toStruct(payload)
	if err != nil {
		// 尽量容错，把错误当 error 事件推到客户端
		_ = g.srv.Send(&v1.StreamEvent{
			Type: "error",
			Data: mustStruct(map[string]any{"message": fmt.Sprintf("marshal payload failed: %v", err)}),
			Ts:   time.Now().UTC().Unix(),
		})
		return nil
	}
	return g.srv.Send(&v1.StreamEvent{
		Type: event,
		Data: st,
		Ts:   time.Now().UTC().Unix(),
	})
}

// gRPC 专用的“带历史写入”的装饰 sink
type grpcHistorySink struct {
	next     runtime.EventSink
	svc      *agentSvc.ChatHistoryService
	ctx      context.Context
	env      string
	tenantID *uint64
	session  *dbmodel.AgentChatSession
	agentID  uint64

	buf strings.Builder
}

func (h *grpcHistorySink) Emit(event string, payload any) error {
	// 先下发给客户端
	if err := h.next.Emit(event, payload); err != nil {
		return err
	}

	// token 聚合
	if event == dto.EventToken {
		if m, ok := payload.(map[string]any); ok {
			if s, ok := m["delta"].(string); ok && s != "" {
				h.buf.WriteString(s)
			}
		}
		return nil
	}

	// final 入库，并触发摘要
	if event == dto.EventFinal && h.session != nil {
		text := extractText(payload)
		if strings.TrimSpace(text) == "" {
			text = h.buf.String()
		}
		if strings.TrimSpace(text) != "" {
			_, _ = h.svc.AppendMessage(h.ctx, h.env, h.tenantID, h.session.ID, h.agentID, "assistant", text, "text", 0, 0, false, nil)
			_, _ = h.svc.SummarizeIfNeeded(h.ctx, h.env, h.tenantID, h.session)
		}
	}
	return nil
}

/* ===================== helpers ===================== */

func toStruct(v any) (*structpb.Struct, error) {
	if v == nil {
		return &structpb.Struct{}, nil
	}
	switch m := v.(type) {
	case map[string]any:
		return structpb.NewStruct(m)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var mm map[string]any
		if err := json.Unmarshal(b, &mm); err != nil {
			// 如果是简单值（string/number），包一层 value
			return structpb.NewStruct(map[string]any{"value": v})
		}
		return structpb.NewStruct(mm)
	}
}

func mustStruct(m map[string]any) *structpb.Struct {
	s, _ := structpb.NewStruct(m)
	return s
}

func extractText(payload any) string {
	switch m := payload.(type) {
	case map[string]any:
		if d, ok := m["data"].(map[string]any); ok {
			if s, ok := d["content"].(string); ok {
				return s
			}
		}
		if s, ok := m["content"].(string); ok {
			return s
		}
	}
	return ""
}
