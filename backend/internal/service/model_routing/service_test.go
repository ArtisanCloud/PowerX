package model_routing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestUpsertPolicyVersionValidations(t *testing.T) {
	svc := newRoutingServiceForTest(t)
	ctx := context.Background()

	_, err := svc.UpsertPolicyVersion(ctx, "default", PolicyInput{})
	require.Error(t, err)

	_, err = svc.UpsertPolicyVersion(ctx, "default", PolicyInput{TenantScope: "tenant-a"})
	require.Error(t, err)

	input := PolicyInput{
		TenantScope:   "tenant-a",
		Rules:         datatypes.JSON(utils.MustJSONBytes(sampleRules())),
		FallbackChain: datatypes.JSON(utils.MustJSONBytes([]string{"provider-backup"})),
	}
	policy, err := svc.UpsertPolicyVersion(ctx, "default", input)
	require.NoError(t, err)
	require.Equal(t, statusDraft, policy.Status)
	require.Equal(t, "pending", utils.ToStr(policy.ApprovalRecord["outcome"]))
	require.InDelta(t, 0.92, policy.SafeModeThresholds["minHitRate"], 0.0001)

	active, err := svc.LatestPolicy(ctx, "default", "tenant-a")
	require.NoError(t, err)
	require.Nil(t, active)
}

func TestUpdatePolicyStatusFlow(t *testing.T) {
	svc := newRoutingServiceForTest(t)
	ctx := context.Background()

	policy, err := svc.UpsertPolicyVersion(ctx, "default", PolicyInput{
		TenantScope:   "tenant-b",
		Rules:         datatypes.JSON(utils.MustJSONBytes(sampleRules())),
		FallbackChain: datatypes.JSON(utils.MustJSONBytes([]string{"provider-backup"})),
	})
	require.NoError(t, err)

	staged, err := svc.UpdatePolicyStatus(ctx, "default", "tenant-b", policy.Version, StatusUpdateInput{
		TargetStatus: statusStaged,
		Approval: &ApprovalUpdate{
			WorkflowID:        "ops-change",
			Approvers:         []string{"alice"},
			RequiredApprovers: 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, statusStaged, staged.Status)

	_, err = svc.UpdatePolicyStatus(ctx, "default", "tenant-b", staged.Version, StatusUpdateInput{
		TargetStatus: statusDraft,
	})
	require.Error(t, err)

	active, err := svc.UpdatePolicyStatus(ctx, "default", "tenant-b", staged.Version, StatusUpdateInput{
		TargetStatus: statusActive,
		Approval: &ApprovalUpdate{
			Approvers:         []string{"alice", "bob"},
			Outcome:           "approved",
			RequiredApprovers: 2,
		},
	})
	require.NoError(t, err)
	require.Equal(t, statusActive, active.Status)

	cached, err := svc.LatestPolicy(ctx, "default", "tenant-b")
	require.NoError(t, err)
	require.NotNil(t, cached)
	require.Equal(t, statusActive, cached.Status)

	rolled, err := svc.RollbackPolicy(ctx, "default", "tenant-b", active.Version)
	require.NoError(t, err)
	require.Equal(t, statusRolledBack, rolled.Status)

	afterRollback, err := svc.LatestPolicy(ctx, "default", "tenant-b")
	require.NoError(t, err)
	require.Nil(t, afterRollback)
}

func TestToggleSafeMode(t *testing.T) {
	svc := newRoutingServiceForTest(t)
	ctx := context.Background()

	state, err := svc.ToggleSafeMode(ctx, "default", "tenant-safe", true, time.Minute, "ops", "incident")
	require.NoError(t, err)
	require.True(t, state.Enabled)
	require.Equal(t, "tenant-safe", state.TenantScope)
	require.NotNil(t, state.ExpiresAt)

	current, err := svc.SafeModeState(ctx, "default", "tenant-safe")
	require.NoError(t, err)
	require.True(t, current.Enabled)

	disabled, err := svc.ToggleSafeMode(ctx, "default", "tenant-safe", false, 0, "ops", "")
	require.NoError(t, err)
	require.False(t, disabled.Enabled)

	final, err := svc.SafeModeState(ctx, "default", "tenant-safe")
	require.NoError(t, err)
	require.False(t, final.Enabled)
}

func TestDecideRoute(t *testing.T) {
	svc := newRoutingServiceForTest(t)
	ctx := context.Background()

	policy, err := svc.UpsertPolicyVersion(ctx, "default", PolicyInput{
		TenantScope: "tenant-route",
		Rules: datatypes.JSON(utils.MustJSONBytes([]map[string]any{
			{
				"taskPattern": "chat/*",
				"candidates": []map[string]any{
					{"providerId": "provider-a", "weight": 0.4},
					{"providerId": "provider-b", "weight": 0.9},
				},
				"sla": map[string]any{
					"latencyMs":   800,
					"costCeiling": 0.002,
				},
			},
		})),
		FallbackChain: datatypes.JSON(utils.MustJSONBytes([]string{"provider-fallback"})),
	})
	require.NoError(t, err)
	_, err = svc.UpdatePolicyStatus(ctx, "default", "tenant-route", policy.Version, StatusUpdateInput{
		TargetStatus: statusActive,
	})
	require.NoError(t, err)

	result, err := svc.DecideRoute(ctx, "default", "tenant-route", map[string]any{
		"taskType": "chat/general",
	})
	require.NoError(t, err)
	require.Equal(t, "provider-b", result.PrimaryProviderID)
	require.False(t, result.FallbackUsed)
	require.Equal(t, "chat/*", result.MatchedRulePattern)

	_, err = svc.ToggleSafeMode(ctx, "default", "tenant-route", true, time.Minute, "ops", "incident")
	require.NoError(t, err)
	safeResult, err := svc.DecideRoute(ctx, "default", "tenant-route", map[string]any{
		"taskType": "chat/general",
	})
	require.NoError(t, err)
	require.Equal(t, "provider-fallback", safeResult.PrimaryProviderID)
	require.True(t, safeResult.FallbackUsed)
	require.True(t, safeResult.SafeMode)
}

func newRoutingServiceForTest(t *testing.T) *Service {
	t.Helper()
	fakeRepo := newInMemoryRoutingRepo()
	return NewService(Options{
		Options: shared.Options{
			Cache: cache.NewMemoryCache(),
		},
		Policies: fakeRepo,
	})
}

func sampleRules() []map[string]any {
	return []map[string]any{
		{
			"taskPattern": "chat",
			"candidates": []map[string]any{
				{
					"providerId": "provider-main",
					"weight":     1.0,
				},
			},
		},
	}
}

type inMemoryRoutingRepo struct {
	seq     map[string]uint32
	records map[string]map[uint32]*model.RoutingPolicy
}

func newInMemoryRoutingRepo() *inMemoryRoutingRepo {
	return &inMemoryRoutingRepo{
		seq:     map[string]uint32{},
		records: map[string]map[uint32]*model.RoutingPolicy{},
	}
}

func repoKey(env, scope string) string {
	return fmt.Sprintf("%s|%s", env, scope)
}

func clonePolicy(policy *model.RoutingPolicy) *model.RoutingPolicy {
	cp := *policy
	cp.SafeModeThresholds = utils.CloneJSONMap(policy.SafeModeThresholds)
	cp.ApprovalRecord = utils.CloneJSONMap(policy.ApprovalRecord)
	return &cp
}

func (r *inMemoryRoutingRepo) NextVersion(_ context.Context, env, tenantScope string) (uint32, error) {
	key := repoKey(env, tenantScope)
	r.seq[key]++
	return r.seq[key], nil
}

func (r *inMemoryRoutingRepo) CreateVersion(_ context.Context, policy *model.RoutingPolicy) (*model.RoutingPolicy, error) {
	key := repoKey(policy.Env, policy.TenantScope)
	if policy.Version == 0 {
		r.seq[key]++
		policy.Version = r.seq[key]
	} else if policy.Version > r.seq[key] {
		r.seq[key] = policy.Version
	}
	if policy.UUID == uuid.Nil {
		policy.UUID = uuid.New()
	}
	if _, ok := r.records[key]; !ok {
		r.records[key] = map[uint32]*model.RoutingPolicy{}
	}
	stored := clonePolicy(policy)
	stored.CreatedAt = time.Now().UTC()
	stored.UpdatedAt = stored.CreatedAt
	r.records[key][stored.Version] = stored
	return clonePolicy(stored), nil
}

func (r *inMemoryRoutingRepo) Latest(_ context.Context, env, tenantScope, status string) (*model.RoutingPolicy, error) {
	key := repoKey(env, tenantScope)
	versions := r.records[key]
	var latest *model.RoutingPolicy
	for _, record := range versions {
		if status != "" && !strings.EqualFold(record.Status, status) {
			continue
		}
		if latest == nil || record.Version > latest.Version {
			latest = record
		}
	}
	if latest == nil {
		return nil, nil
	}
	return clonePolicy(latest), nil
}

func (r *inMemoryRoutingRepo) UpdateStatus(_ context.Context, env, tenantScope string, version uint32, status string, payload map[string]any) error {
	key := repoKey(env, tenantScope)
	record, ok := r.records[key][version]
	if !ok {
		return fmt.Errorf("policy not found")
	}
	record.Status = status
	if raw, ok := payload["approval_record"]; ok {
		if m, ok := raw.(datatypes.JSONMap); ok {
			record.ApprovalRecord = utils.CloneJSONMap(m)
		}
	}
	record.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *inMemoryRoutingRepo) FindVersion(_ context.Context, env, tenantScope string, version uint32) (*model.RoutingPolicy, error) {
	key := repoKey(env, tenantScope)
	record, ok := r.records[key][version]
	if !ok {
		return nil, nil
	}
	return clonePolicy(record), nil
}
