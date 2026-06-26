package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

type fakeSkillStateStore struct {
	got SkillStateUpsert
	err error
}

func (s *fakeSkillStateStore) UpsertSkillState(ctx context.Context, in SkillStateUpsert) error {
	s.got = in
	return s.err
}

func TestPersistAwaitingSkillStateRequiresRuntimeScope(t *testing.T) {
	store := &fakeSkillStateStore{}
	ctx := ContextWithSkillStateStore(context.Background(), store)

	err := persistAwaitingSkillState(ctx, map[string]any{
		"node_kind":      dto.NodeKindSkill,
		"skill_id":       "powerxplugin.template.basic",
		"action":         "create",
		"missing_fields": []string{"template.title"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "env is required")
}

func TestPersistAwaitingSkillStateWritesStructuredState(t *testing.T) {
	store := &fakeSkillStateStore{}
	ctx := ContextWithSkillStateStore(context.Background(), store)
	ctx = context.WithValue(ctx, "env", "dev")
	ctx = context.WithValue(ctx, "tenant_uuid", "tenant-a")
	ctx = context.WithValue(ctx, "session_id", "81")
	ctx = context.WithValue(ctx, "agent_id", "18")
	ctx = context.WithValue(ctx, "message_id", "99")

	err := persistAwaitingSkillState(ctx, map[string]any{
		"node_kind":      dto.NodeKindSkill,
		"skill_id":       "powerxplugin.template.basic",
		"action":         "create",
		"missing_fields": []string{"template.description", "template.content"},
		"collected_params": map[string]any{
			"template": map[string]any{"title": "测试模板"},
		},
		"trace_id":      "trace-1",
		"run_id":        "run-1",
		"plan_id":       "plan-1",
		"capability_id": "com.powerx.plugins.base.local.template.create",
	})

	require.NoError(t, err)
	require.Equal(t, "dev", store.got.Env)
	require.NotNil(t, store.got.TenantUUID)
	require.Equal(t, "tenant-a", *store.got.TenantUUID)
	require.Equal(t, uint64(81), store.got.SessionID)
	require.Equal(t, uint64(18), store.got.AgentID)
	require.Equal(t, uint64(99), store.got.LastMessageID)
	require.Equal(t, "powerxplugin.template.basic", store.got.SkillID)
	require.Equal(t, "powerxplugin.template.basic.create", store.got.StateKey)
	require.Equal(t, "collecting", store.got.Status)
	require.Equal(t, "create", store.got.Action)
	require.Equal(t, "trace-1", store.got.Meta["trace_id"])
	require.Equal(t, []string{"template.description", "template.content"}, store.got.State["missing"])
	require.NotNil(t, store.got.State["collected"])
}

func TestPersistAwaitingSkillStatePropagatesStoreError(t *testing.T) {
	store := &fakeSkillStateStore{err: errors.New("db unavailable")}
	ctx := ContextWithSkillStateStore(context.Background(), store)
	ctx = context.WithValue(ctx, "env", "dev")
	ctx = context.WithValue(ctx, "tenant_uuid", "tenant-a")
	ctx = context.WithValue(ctx, "session_id", "81")
	ctx = context.WithValue(ctx, "agent_id", "18")
	ctx = context.WithValue(ctx, "message_id", "99")

	err := persistAwaitingSkillState(ctx, map[string]any{
		"node_kind": dto.NodeKindSkill,
		"skill_id":  "powerxplugin.template.basic",
		"action":    "create",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "db unavailable")
}
