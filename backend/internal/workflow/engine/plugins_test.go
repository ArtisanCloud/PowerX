package engine

import (
	"context"
	"errors"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/stretchr/testify/require"
)

type fakeSelector struct {
	lastReq capservice.CapabilityInvokeRequest
	err     error
}

func (f *fakeSelector) Invoke(ctx context.Context, req capservice.CapabilityInvokeRequest) (capservice.CapabilityInvokeResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return capservice.CapabilityInvokeResponse{}, f.err
	}
	return capservice.CapabilityInvokeResponse{
		CapabilityID: req.CapabilityID,
		TraceID:      req.TraceID,
		Status:       "completed",
		ProtocolUsed: "mcp",
	}, nil
}

func TestCapabilityStepAdapterInvoke(t *testing.T) {
	selector := &fakeSelector{}
	adapter := NewCapabilityStepAdapter(selector, nil)
	require.NotNil(t, adapter)

	resp, err := adapter.InvokeCapability(context.Background(), CapabilityStepInput{
		CapabilityID:      "demo.capability",
		TenantUUID:        "tenant-001",
		Intent:            "demo.intent",
		ToolScope:         "default",
		ToolGrantIDs:      []string{"grant-alpha", "grant-alpha"},
		PreferredProtocol: "mcp",
		IdempotencyKey:    "ide-1",
		TraceID:           "trace-1",
		Payload: map[string]interface{}{
			"foo": "bar",
		},
		Context: map[string]interface{}{
			"feature_flags": []string{"beta"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "demo.capability", resp.CapabilityID)
	require.Equal(t, "trace-1", resp.TraceID)
	require.Equal(t, 1, len(selector.lastReq.ToolGrantIDs))
	require.Equal(t, "grant-alpha", selector.lastReq.ToolGrantIDs[0])
}

func TestCapabilityStepAdapterMissingFields(t *testing.T) {
	adapter := NewCapabilityStepAdapter(&fakeSelector{}, nil)
	_, err := adapter.InvokeCapability(context.Background(), CapabilityStepInput{})
	require.Error(t, err)
}

func TestCapabilityStepAdapterSelectorError(t *testing.T) {
	selector := &fakeSelector{err: errors.New("invoke failed")}
	adapter := NewCapabilityStepAdapter(selector, nil)
	_, err := adapter.InvokeCapability(context.Background(), CapabilityStepInput{
		CapabilityID: "demo.capability",
		TenantUUID:   "tenant-001",
	})
	require.Error(t, err)
	require.Equal(t, "invoke failed", err.Error())
}
