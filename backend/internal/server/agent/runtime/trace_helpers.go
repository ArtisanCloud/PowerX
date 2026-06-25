package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type traceRuntime struct {
	logger    agenttrace.AgentTraceLogger
	meta      agenttrace.AgentRunMeta
	startedAt time.Time
	seq       int
	starts    map[string]time.Time
}

func (e *Engine) newTraceRuntime(ctx context.Context, msg string, reqCfg *dto.ChatConfig, explicitFlow, transport string) (*traceRuntime, error) {
	logger := e.mgr.AgentTraceLogger()
	if logger == nil {
		return nil, nil
	}
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))
	if traceID == "" {
		traceID = fmt.Sprintf("trace_%d", time.Now().UnixNano())
	}
	runID := fmt.Sprintf("run_%d", time.Now().UnixNano())
	sessionID := firstTraceString(
		traceContextValue(ctx, "session_id"),
		traceContextValue(ctx, "sessionId"),
		traceValue(reqCfg, "session_id"),
		traceValue(reqCfg, "sessionId"),
	)
	messageID := firstTraceString(
		traceContextValue(ctx, "message_id"),
		traceContextValue(ctx, "messageId"),
		traceValue(reqCfg, "message_id"),
		traceValue(reqCfg, "messageId"),
	)
	agentID := firstTraceString(traceContextValue(ctx, "agent_id"), traceContextValue(ctx, "agentId"), traceValue(reqCfg, "agent_id"), traceValue(reqCfg, "agentId"), "system_default_agent")
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" {
		tenantUUID = firstTraceString(traceContextValue(ctx, "tenant_uuid"), traceContextValue(ctx, "tenantUuid"), traceValue(reqCfg, "tenant_uuid"), traceValue(reqCfg, "tenantUuid"))
	}
	meta := agenttrace.AgentRunMeta{
		TraceID:           traceID,
		RunID:             runID,
		TenantUUID:        tenantUUID,
		UserUUID:          strings.TrimSpace(reqctx.GetUserUUID(ctx)),
		AgentID:           agentID,
		SessionID:         sessionID,
		MessageID:         messageID,
		Channel:           firstTraceString(traceValue(reqCfg, "channel"), transport),
		UserMessageDigest: digestString(msg),
		Attributes: map[string]any{
			"explicit_flow": strings.TrimSpace(explicitFlow),
			"transport":     transport,
			"env":           strings.TrimSpace(reqctx.GetEnv(ctx)),
		},
	}
	runCtx, err := logger.StartRun(ctx, meta)
	if err != nil {
		return nil, err
	}
	if runCtx != nil {
		meta = runCtx.Meta
	}
	return &traceRuntime{logger: logger, meta: meta, startedAt: time.Now().UTC(), starts: map[string]time.Time{}}, nil
}

func (tr *traceRuntime) appendRunStateEvent(ctx context.Context, event string, payload any) {
	if tr == nil || tr.logger == nil {
		return
	}
	_ = tr.logger.AppendRunStateEvent(ctx, tr.meta, event, payload)
}

func (tr *traceRuntime) withPlan(planID string) {
	if tr == nil {
		return
	}
	tr.meta.PlanID = strings.TrimSpace(planID)
}

func (tr *traceRuntime) applyExecutionMeta(mt agentschema.ExecutionMeta) agentschema.ExecutionMeta {
	if tr == nil {
		return mt
	}
	if mt.TraceID == "" {
		mt.TraceID = tr.meta.TraceID
	}
	if mt.Metadata == nil {
		mt.Metadata = map[string]any{}
	}
	mt.Metadata["trace_id"] = tr.meta.TraceID
	mt.Metadata["run_id"] = tr.meta.RunID
	mt.Metadata["tenant_uuid"] = tr.meta.TenantUUID
	mt.Metadata["user_uuid"] = tr.meta.UserUUID
	mt.Metadata["agent_id"] = tr.meta.AgentID
	mt.Metadata["session_id"] = tr.meta.SessionID
	mt.Metadata["message_id"] = tr.meta.MessageID
	mt.Metadata["plan_id"] = tr.meta.PlanID
	mt.Metadata["channel"] = tr.meta.Channel
	return mt
}

func (tr *traceRuntime) startNode(ctx context.Context, kind, ref string, attrs map[string]any) string {
	if tr == nil || tr.logger == nil {
		return ""
	}
	tr.seq++
	nodeID := firstTraceString(fmt.Sprint(attrs["node_id"]), fmt.Sprintf("%03d_%s", tr.seq, kind))
	now := time.Now().UTC()
	tr.starts[nodeID] = now
	node := agenttrace.AgentTraceNode{
		AgentRunMeta: tr.meta,
		NodeID:       nodeID,
		NodeSeq:      tr.seq,
		NodeKind:     kind,
		NodeRef:      strings.TrimSpace(ref),
		InputSummary: cloneMap(attrs),
		Attributes:   cloneMap(attrs),
		StartedAt:    now,
	}
	node.ContextRef = fmt.Sprint(attrs["context_ref_id"])
	node.SkillID = fmt.Sprint(attrs["skill_id"])
	node.PluginID = fmt.Sprint(attrs["plugin_id"])
	node.CapabilityID = fmt.Sprint(attrs["capability_id"])
	node.ExecutorPath = fmt.Sprint(attrs["executor_path"])
	_ = tr.logger.StartNode(ctx, node)
	return nodeID
}

func (tr *traceRuntime) endNode(ctx context.Context, nodeID, kind, ref string, out map[string]any) {
	if tr == nil || tr.logger == nil || nodeID == "" {
		return
	}
	now := time.Now().UTC()
	_ = tr.logger.EndNode(ctx, agenttrace.AgentTraceNodeResult{
		AgentRunMeta:  tr.meta,
		NodeID:        nodeID,
		NodeSeq:       tr.seq,
		NodeKind:      kind,
		NodeRef:       strings.TrimSpace(ref),
		OutputDigest:  digestAny(out),
		OutputSummary: cloneMap(out),
		StartedAt:     tr.starts[nodeID],
		EndedAt:       now,
	})
}

func (tr *traceRuntime) failNode(ctx context.Context, nodeID, kind, ref string, err error) {
	if tr == nil || tr.logger == nil || nodeID == "" {
		return
	}
	now := time.Now().UTC()
	summary := ""
	if err != nil {
		summary = err.Error()
	}
	_ = tr.logger.FailNode(ctx, agenttrace.AgentTraceNodeFailure{
		AgentTraceNodeResult: agenttrace.AgentTraceNodeResult{
			AgentRunMeta: tr.meta,
			NodeID:       nodeID,
			NodeSeq:      tr.seq,
			NodeKind:     kind,
			NodeRef:      strings.TrimSpace(ref),
			StartedAt:    tr.starts[nodeID],
			EndedAt:      now,
		},
		ErrorCode:    "AGENT_RUNTIME_NODE_FAILED",
		ErrorSummary: summary,
	})
}

func (tr *traceRuntime) complete(ctx context.Context, status string, finalText string, runErr error) {
	if tr == nil || tr.logger == nil {
		return
	}
	if status == "" {
		status = agenttrace.RunStatusCompleted
	}
	result := agenttrace.AgentRunResult{
		AgentRunMeta:        tr.meta,
		Status:              status,
		FinalResponseDigest: digestString(finalText),
		StartedAt:           tr.startedAt,
		EndedAt:             time.Now().UTC(),
	}
	if runErr != nil {
		result.ErrorCode = "AGENT_RUNTIME_FAILED"
		result.ErrorSummary = runErr.Error()
	}
	_ = tr.logger.CompleteRun(ctx, result)
}

func traceMetaMap(tr *traceRuntime) map[string]any {
	if tr == nil {
		return nil
	}
	return map[string]any{
		"trace_id":   tr.meta.TraceID,
		"run_id":     tr.meta.RunID,
		"session_id": tr.meta.SessionID,
		"message_id": tr.meta.MessageID,
		"plan_id":    tr.meta.PlanID,
	}
}

func mergeTraceMetadata(dst map[string]any, tr *traceRuntime) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range traceMetaMap(tr) {
		dst[k] = v
	}
	return dst
}

func traceValue(cfg *dto.ChatConfig, key string) string {
	if cfg == nil {
		return ""
	}
	raw, _ := json.Marshal(cfg)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(m[key]))
}

func traceContextValue(ctx context.Context, key string) string {
	if ctx == nil || strings.TrimSpace(key) == "" {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(ctx.Value(key)))
}

func firstTraceString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" && strings.TrimSpace(v) != "<nil>" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func digestString(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(v))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestAny(v any) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return digestString(fmt.Sprint(v))
	}
	return digestString(string(data))
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out[k] = v
	}
	return out
}
