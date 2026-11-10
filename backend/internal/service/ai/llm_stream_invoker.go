package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

const LLMStreamCapability = "com.corex.ai.llm.stream"
const LLMStreamEndpoint = "core://ai/llm/stream"

type LLMStreamOperation struct {
	ModelKey      string         `json:"model_key"`
	Inputs        []ContentItem  `json:"inputs"`
	Params        map[string]any `json:"params,omitempty"`
	StreamOptions struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

type LLMStreamService interface {
	ResolveTenantEnv(context.Context, string) (string, bool, error)
	LLMStream(context.Context, string, string, string, []ContentItem, map[string]any, func(string)) (string, error)
}

// LLMStreamInvoker implements the fixed tenant-bound Host operation. The shared
// invocation service checks publication, registration, grants and model access.
type LLMStreamInvoker struct{ Service LLMStreamService }

func (*LLMStreamInvoker) InvokeCoreCapability(context.Context, cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	return nil, cap.ErrCoreCapabilityNotHandled
}
func (i *LLMStreamInvoker) InvokeCoreCapabilityStream(ctx context.Context, in cap.CoreCapabilityInvokeInput) (io.ReadCloser, error) {
	if in.CapabilityID != LLMStreamCapability {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trace := reqctx.GetTraceID(ctx)
	if in.Method != "INVOKE" || in.Endpoint != LLMStreamEndpoint || len(in.Context) != 0 || trace == "" {
		return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
	}
	for key := range in.Payload {
		if key != "body" && key != "query" {
			return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
		}
	}
	for key := range in.Query {
		if key != "env" {
			return nil, MediaError(400, "AI_TYPED_OPERATION_REQUIRED")
		}
	}
	if tenant := reqctx.GetTenantUUID(ctx); tenant == "" || tenant != in.TenantUUID {
		return nil, MediaError(403, "AI_FORBIDDEN")
	}
	var op LLMStreamOperation
	if err := strictMediaBody(in.Body, &op); err != nil {
		return nil, err
	}
	if strings.TrimSpace(op.ModelKey) == "" || len(op.Inputs) == 0 {
		return nil, MediaError(400, "AI_OPERATION_INVALID")
	}
	if i == nil || i.Service == nil {
		return nil, MediaError(503, "AI_DEPENDENCY_UNAVAILABLE")
	}
	env := strings.TrimSpace(reqctx.GetEnv(ctx))
	if env == "default" {
		env = ""
	}
	if requested := in.Query["env"]; requested != "" {
		if env != "" && env != requested {
			return nil, MediaError(403, "AI_FORBIDDEN")
		}
		env = requested
	}
	if env == "" {
		var err error
		env, _, err = i.Service.ResolveTenantEnv(ctx, in.TenantUUID)
		if err != nil {
			return nil, err
		}
	}
	if env != "dev" && env != "staging" && env != "prod" {
		return nil, MediaError(400, "AI_OPERATION_INVALID")
	}
	streamCtx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	go func() {
		stop := context.AfterFunc(streamCtx, func() { _ = writer.CloseWithError(streamCtx.Err()) })
		defer func() { stop(); _ = writer.Close(); cancel() }()
		emit := func(kind string, data map[string]any) error {
			data["traceId"] = trace
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", kind, raw)
			return err
		}
		if emit("start", map[string]any{"model_key": op.ModelKey}) != nil {
			return
		}
		index := 0
		var writeErr error
		text, err := i.Service.LLMStream(streamCtx, env, in.TenantUUID, op.ModelKey, op.Inputs, op.Params, func(delta string) {
			if writeErr != nil {
				return
			}
			writeErr = emit("delta", map[string]any{"delta": delta, "index": index})
			index++
			if writeErr != nil {
				cancel()
			}
		})
		if writeErr != nil || streamCtx.Err() != nil {
			return
		}
		if err != nil {
			_ = emit("error", map[string]any{"code": "AI_STREAM_FAILED"})
			return
		}
		if op.StreamOptions.IncludeUsage {
			if emit("usage", map[string]any{}) != nil {
				return
			}
		}
		_ = emit("done", map[string]any{"ok": true, "text": text, "finish_reason": "stop"})
	}()
	return &llmStreamReadCloser{PipeReader: reader, cancel: cancel}, nil
}

type llmStreamReadCloser struct {
	*io.PipeReader
	cancel context.CancelFunc
}

func (r *llmStreamReadCloser) Close() error { r.cancel(); return r.PipeReader.Close() }
