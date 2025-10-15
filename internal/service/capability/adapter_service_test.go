package capability

import (
	"context"
	"errors"
	"testing"
	"time"

	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
)

type fakeAdapter struct {
	invoke      func(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
	stream      func(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error
	healthCheck func(ctx context.Context, capabilityKey string) (*TransportHealthReport, error)
	close       func(ctx context.Context) error
}

func (f *fakeAdapter) Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
	if f.invoke != nil {
		return f.invoke(ctx, req)
	}
	return nil, nil
}

func (f *fakeAdapter) Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error {
	if f.stream != nil {
		return f.stream(ctx, req, sink)
	}
	return nil
}

func (f *fakeAdapter) HealthCheck(ctx context.Context, capabilityKey string) (*TransportHealthReport, error) {
	if f.healthCheck != nil {
		return f.healthCheck(ctx, capabilityKey)
	}
	return &TransportHealthReport{Status: "healthy", CheckedAt: time.Now()}, nil
}

func (f *fakeAdapter) Close(ctx context.Context) error {
	if f.close != nil {
		return f.close(ctx)
	}
	return nil
}

func TestAdapterServiceInvokeWithFallback(t *testing.T) {
	svc := NewAdapterService(nil, nil, nil)

	failing := &fakeAdapter{
		invoke: func(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
			return nil, errors.New("boom")
		},
	}
	succeeding := &fakeAdapter{
		invoke: func(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
			return &TransportResponse{Output: map[string]any{"ok": true}}, nil
		},
	}

	svc.RegisterAdapter(capb.TransportKind_TRANSPORT_KIND_HTTP, failing)
	svc.RegisterAdapter(capb.TransportKind_TRANSPORT_KIND_GRPC, succeeding)

	req := &TransportRequest{
		CapabilityKey: "demo.capability",
		Version:       "1.0.0",
		Transport:     capb.TransportKind_TRANSPORT_KIND_HTTP,
	}

	resp, err := svc.InvokeWithFallback(context.Background(), req, []capb.TransportKind{
		capb.TransportKind_TRANSPORT_KIND_HTTP,
		capb.TransportKind_TRANSPORT_KIND_GRPC,
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp == nil || resp.Output["ok"] != true {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAdapterServiceRetryBackoff(t *testing.T) {
	attempts := 0
	svc := NewAdapterService(nil, nil, nil)
	svc.RegisterAdapter(capb.TransportKind_TRANSPORT_KIND_HTTP, &fakeAdapter{
		invoke: func(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
			attempts++
			if attempts < 2 {
				return nil, errors.New("transient")
			}
			return &TransportResponse{Status: ResponseStatusSuccess}, nil
		},
	})

	req := &TransportRequest{
		CapabilityKey: "demo.capability",
		Transport:     capb.TransportKind_TRANSPORT_KIND_HTTP,
		RetryContext: &RetryContext{
			MaxAttempts:    2,
			InitialBackoff: 5 * time.Millisecond,
			Multiplier:     1.0,
			Idempotent:     true,
			ShouldRetry:    func(err error) bool { return true },
		},
	}

	resp, err := svc.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if resp == nil || resp.Status != ResponseStatusSuccess {
		t.Fatalf("unexpected response status: %+v", resp)
	}
}
