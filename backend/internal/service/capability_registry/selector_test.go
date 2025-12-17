package capability_registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSelectorInvokeResolvesFromIntentMapping(t *testing.T) {
	ctx := context.Background()
	store := &fakeSnapshotStore{
		snapshot: SelectorPolicySnapshot{
			TenantID: "tenant-001",
			IntentMappings: map[string]map[string]string{
				"demo.intent": {
					"global": "cap.demo",
				},
			},
			PreferMatrix: map[string]ProtocolPreference{
				"cap.demo": {Prefer: "grpc"},
			},
		},
	}
	invoker := &fakeInvoker{result: InvocationResult{
		TraceID:      "trace-123",
		Status:       "completed",
		ProtocolUsed: "grpc",
		Result:       map[string]interface{}{"ok": true},
	}}

	selector := NewSelector(SelectorOptions{Store: store, Invoker: invoker})
	resp, err := selector.Invoke(ctx, CapabilityInvokeRequest{
		TenantUUID:   "tenant-001",
		Intent:       "demo.intent",
		ToolScope:    "global",
		ToolGrantIDs: []string{"grant-a"},
		Payload:      map[string]interface{}{"foo": "bar"},
	})
	require.NoError(t, err)
	require.Equal(t, "cap.demo", resp.CapabilityID)
	require.Equal(t, "trace-123", resp.TraceID)
	require.Equal(t, "completed", resp.Status)
	require.Equal(t, map[string]interface{}{"ok": true}, resp.Result)

	require.Equal(t, "tenant-001", store.lastTenant)
	require.Equal(t, []string{"grant-a"}, store.lastGrants)
	require.Equal(t, "cap.demo", invoker.lastInput.CapabilityID)
	require.Equal(t, "grpc", invoker.lastInput.PreferredProtocol)
	require.Equal(t, "tenant-001", invoker.lastInput.TenantUUID)
	require.Equal(t, "bar", invoker.lastInput.Payload["foo"])
	require.Equal(t, "global", invoker.lastInput.Context["tool_scope"])
}

func TestSelectorInvokeWithCapabilityIDWithoutStore(t *testing.T) {
	ctx := context.Background()
	invoker := &fakeInvoker{result: InvocationResult{TraceID: "trace"}}
	selector := NewSelector(SelectorOptions{Invoker: invoker})

	resp, err := selector.Invoke(ctx, CapabilityInvokeRequest{
		CapabilityID:      "cap.direct",
		TenantUUID:        "tenant-002",
		PreferredProtocol: "rest",
		Context:           map[string]interface{}{"tool_scope": "fallback"},
	})
	require.NoError(t, err)
	require.Equal(t, "cap.direct", resp.CapabilityID)
	require.Equal(t, "rest", invoker.lastInput.PreferredProtocol)
	require.Equal(t, "fallback", invoker.lastInput.Context["tool_scope"])
}

func TestSelectorRejectsCapabilityOutsideSnapshot(t *testing.T) {
	ctx := context.Background()
	store := &fakeSnapshotStore{
		snapshot: SelectorPolicySnapshot{
			IntentMappings: map[string]map[string]string{
				"demo.intent": {
					"default": "cap.allowed",
				},
			},
			PreferMatrix: map[string]ProtocolPreference{
				"cap.allowed": {Prefer: "mcp"},
			},
		},
	}
	selector := NewSelector(SelectorOptions{Store: store, Invoker: &fakeInvoker{}})

	_, err := selector.Invoke(ctx, CapabilityInvokeRequest{
		CapabilityID: "cap.denied",
		TenantUUID:   "tenant-003",
	})
	require.ErrorIs(t, err, ErrSelectorCapabilityForbidden)
}

func TestSelectorErrorsWhenMappingMissing(t *testing.T) {
	ctx := context.Background()
	store := &fakeSnapshotStore{snapshot: SelectorPolicySnapshot{TenantID: "tenant-004"}}
	selector := NewSelector(SelectorOptions{Store: store, Invoker: &fakeInvoker{}})

	_, err := selector.Invoke(ctx, CapabilityInvokeRequest{
		TenantUUID: "tenant-004",
		Intent:     "unknown.intent",
	})
	require.ErrorIs(t, err, ErrSelectorCapabilityRequired)
}

func TestSelectorPropagatesStoreError(t *testing.T) {
	ctx := context.Background()
	store := &fakeSnapshotStore{err: errors.New("cache miss")}
	selector := NewSelector(SelectorOptions{Store: store, Invoker: &fakeInvoker{}})

	_, err := selector.Invoke(ctx, CapabilityInvokeRequest{TenantUUID: "tenant-005"})
	require.EqualError(t, err, "cache miss")
}

type fakeSnapshotStore struct {
	snapshot   SelectorPolicySnapshot
	err        error
	lastTenant string
	lastGrants []string
}

func (f *fakeSnapshotStore) GetSnapshot(ctx context.Context, tenantUUID string, toolGrants []string) (SelectorPolicySnapshot, error) {
	f.lastTenant = tenantUUID
	f.lastGrants = append([]string(nil), toolGrants...)
	if f.err != nil {
		return SelectorPolicySnapshot{}, f.err
	}
	return f.snapshot, nil
}

type fakeInvoker struct {
	lastInput InvocationInput
	result    InvocationResult
	err       error
}

func (f *fakeInvoker) Invoke(ctx context.Context, in InvocationInput) (InvocationResult, error) {
	f.lastInput = in
	if f.err != nil {
		return InvocationResult{}, f.err
	}
	return f.result, nil
}

func TestCandidateIntents(t *testing.T) {
	require.Equal(t, []string{"demo", "*"}, candidateIntents("demo"))
	require.Equal(t, []string{"*"}, candidateIntents(""))
}

func TestCandidateScopes(t *testing.T) {
	require.Equal(t, []string{"scope", "default"}, candidateScopes("scope"))
	require.Equal(t, []string{"default"}, candidateScopes(""))
}

func TestSnapshotProviderFunc(t *testing.T) {
	called := false
	fn := SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (SelectorPolicySnapshot, error) {
		called = true
		return SelectorPolicySnapshot{TenantID: tenant, GeneratedAt: time.Unix(0, 0)}, nil
	})
	snap, err := fn.GetSnapshot(context.Background(), "tenant-007", nil)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "tenant-007", snap.TenantID)
}
