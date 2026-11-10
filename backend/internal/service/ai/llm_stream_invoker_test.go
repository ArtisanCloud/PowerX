package ai

import (
	"context"
	"errors"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"
)

type llmStreamFixtureService struct {
	calls   int
	fail    bool
	wait    bool
	stopped chan struct{}
}

func (*llmStreamFixtureService) ResolveTenantEnv(context.Context, string) (string, bool, error) {
	return "dev", true, nil
}
func (s *llmStreamFixtureService) LLMStream(ctx context.Context, env, tenant, model string, inputs []ContentItem, params map[string]any, delta func(string)) (string, error) {
	s.calls++
	if s.wait {
		<-ctx.Done()
		close(s.stopped)
		return "", ctx.Err()
	}
	if s.fail {
		return "", errors.New("fixture-private-upstream-detail")
	}
	delta("fixture-delta")
	return "fixture-delta", nil
}
func llmStreamFixtureInput() (context.Context, cap.CoreCapabilityInvokeInput) {
	ctx := reqctx.WithTraceID(reqctx.WithTenantUUID(context.Background(), "6b5d0240-9920-46da-b707-88200e0f51ea"), "b3d0388d-b6eb-4500-a98d-22f4eb60c277")
	return ctx, cap.CoreCapabilityInvokeInput{CapabilityID: LLMStreamCapability, TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea", Method: "INVOKE", Endpoint: LLMStreamEndpoint, Body: map[string]any{"model_key": "ollama/fixture", "inputs": []any{map[string]any{"role": "user", "type": "text", "content": "fixture-input"}}}, Payload: map[string]any{}}
}
func TestLLMStreamTypedBindingUsesCanonicalCoreEvents(t *testing.T) {
	ctx, in := llmStreamFixtureInput()
	svc := &llmStreamFixtureService{}
	r, err := (&LLMStreamInvoker{Service: svc}).InvokeCoreCapabilityStream(ctx, in)
	require.NoError(t, err)
	defer r.Close()
	raw, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Equal(t, 1, svc.calls)
	for _, event := range []string{"event: start", "event: delta", "event: done"} {
		require.Contains(t, string(raw), event)
	}
	require.Contains(t, string(raw), `"ok":true`)
	require.Contains(t, string(raw), `"traceId":"b3d0388d-b6eb-4500-a98d-22f4eb60c277"`)
}
func TestLLMStreamTypedBindingRejectsOverridesBeforeProvider(t *testing.T) {
	for _, field := range []string{"tenant", "endpoint", "method", "context", "headers", "body", "query", "trace", "environment"} {
		t.Run(field, func(t *testing.T) {
			ctx, in := llmStreamFixtureInput()
			switch field {
			case "tenant":
				in.TenantUUID = "foreign"
			case "endpoint":
				in.Endpoint = "http://untrusted.invalid"
			case "method":
				in.Method = "POST"
			case "context":
				in.Context = map[string]any{"is_root": true}
			case "headers":
				in.Payload["headers"] = map[string]any{}
			case "body":
				in.Body["tenant_uuid"] = "foreign"
			case "query":
				in.Query = map[string]string{"endpoint": "foreign"}
			case "environment":
				ctx = reqctx.WithEnv(ctx, "dev")
				in.Query = map[string]string{"env": "prod"}
			case "trace":
				ctx = reqctx.WithTraceID(ctx, "")
			}
			svc := &llmStreamFixtureService{}
			r, err := (&LLMStreamInvoker{Service: svc}).InvokeCoreCapabilityStream(ctx, in)
			require.Error(t, err)
			require.Nil(t, r)
			require.Zero(t, svc.calls)
		})
	}
}
func TestLLMStreamTypedBindingSanitizesProviderFailure(t *testing.T) {
	ctx, in := llmStreamFixtureInput()
	r, err := (&LLMStreamInvoker{Service: &llmStreamFixtureService{fail: true}}).InvokeCoreCapabilityStream(ctx, in)
	require.NoError(t, err)
	defer r.Close()
	raw, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(raw), "AI_STREAM_FAILED")
	require.NotContains(t, string(raw), "fixture-private")
	require.NotContains(t, string(raw), "event: done")
}
func TestLLMStreamTypedBindingCancelsProviderOnConsumerClose(t *testing.T) {
	ctx, in := llmStreamFixtureInput()
	svc := &llmStreamFixtureService{wait: true, stopped: make(chan struct{})}
	r, err := (&LLMStreamInvoker{Service: svc}).InvokeCoreCapabilityStream(ctx, in)
	require.NoError(t, err)
	// Consume start so the provider begins; then close the consumer.
	buf := make([]byte, 1024)
	n, err := r.Read(buf)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(buf[:n]), "event: start"))
	require.NoError(t, r.Close())
	select {
	case <-svc.stopped:
	case <-time.After(time.Second):
		t.Fatal("LLM_STREAM_PROVIDER_NOT_CANCELED")
	}
}
