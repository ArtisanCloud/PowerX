package authorizationbaseline

import (
	"testing"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
)

func TestOverlappingEpochsRetainInternalDecisionBoundaries(t *testing.T) {
	at := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	half := at.Add(30 * time.Minute)
	one := at.Add(time.Hour)
	two := at.Add(2 * time.Hour)
	plan := Plan([]Source{{ID: "A", From: at, Until: &one, Scope: im.ScopeTenant, Effect: "allow"}, {ID: "B", From: half, Until: &two, Scope: im.ScopeTenant, Effect: "allow"}}, at, false)
	AssignEpochs(plan, nil, false)
	require.Len(t, plan, 4)
	for i := 0; i < 3; i++ {
		require.Equal(t, Allowed, plan[i].State)
		require.Equal(t, plan[0].EpochUUID, plan[i].EpochUUID)
		require.Equal(t, at, *plan[i].GrantedAt)
		require.Equal(t, two, *plan[i].ValidUntil)
	}
	require.NotEqual(t, plan[0].UUID, plan[1].UUID)
	require.Equal(t, half, *plan[0].EffectiveUntil)
	require.Equal(t, Denied, plan[3].State)
}
func TestScheduleGapAndAdjacentBoundaries(t *testing.T) {
	at := Micro(time.Now())
	one := at.Add(time.Hour)
	two := at.Add(2 * time.Hour)
	three := at.Add(3 * time.Hour)
	src := []Source{{ID: "A", From: at, Until: &one, Scope: im.ScopeTenant, Effect: "allow"}, {ID: "B", From: two, Until: &three, Scope: im.ScopeTenant, Effect: "allow"}}
	plan := Plan(src, at, false)
	AssignEpochs(plan, nil, true)
	require.Equal(t, "initialized", plan[0].BaselineKind)
	require.Nil(t, plan[0].GrantedAt)
	require.Equal(t, Denied, plan[1].State)
	require.Equal(t, "native", plan[2].BaselineKind)
	require.Equal(t, two, *plan[2].GrantedAt)
	require.NotEqual(t, plan[0].EpochUUID, plan[2].EpochUUID)
	src[1].From = one
	plan = Plan(src, at, false)
	AssignEpochs(plan, nil, false)
	require.Equal(t, plan[0].EpochUUID, plan[1].EpochUUID)
	require.Equal(t, three, *plan[0].ValidUntil)
}
func TestDenyScopeConditionAndRootAreNotOrdinaryAllows(t *testing.T) {
	at := Micro(time.Now())
	allow := Source{ID: "allow", From: at, Scope: im.ScopeTenant, Effect: "allow"}
	for _, tc := range []struct {
		name    string
		sources []Source
		root    bool
		state   string
	}{
		{"tenant_deny", []Source{allow, {ID: "deny", From: at, Scope: im.ScopeTenant, Effect: "deny"}}, false, Denied},
		{"restricted_deny", []Source{allow, {ID: "deny", From: at, Scope: im.ScopeSelf, Effect: "deny"}}, false, UnsupportedScope},
		{"only_self", []Source{{ID: "self", From: at, Scope: im.ScopeSelf, Effect: "allow"}}, false, UnsupportedScope},
		{"conditional", []Source{allow, {ID: "conditional", From: at, Scope: im.ScopeTenant, Effect: "allow", Conditional: true}}, false, UnsupportedPolicy},
		{"root_bypass", nil, true, RootBypass},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.state, Plan(tc.sources, at, tc.root)[0].State) })
	}
}

func TestUnknownHistoryCannotBeReplacedByANativeGrantTime(t *testing.T) {
	at := Micro(time.Now())
	one, two, three, four := at.Add(time.Hour), at.Add(2*time.Hour), at.Add(3*time.Hour), at.Add(4*time.Hour)
	plan := Plan([]Source{
		{ID: "allow", From: at, Until: &three, Scope: im.ScopeTenant, Effect: "allow"},
		{ID: "unknown", From: one, Until: &two, Scope: im.ScopeTenant, Effect: "deny", Conditional: true},
		{ID: "regrant", From: four, Scope: im.ScopeTenant, Effect: "allow"},
	}, at, false)
	AssignEpochs(plan, nil, true)
	require.Len(t, plan, 5)
	require.Equal(t, Allowed, plan[0].State)
	require.Nil(t, plan[0].ValidUntil)
	require.Equal(t, one, *plan[0].EffectiveUntil)
	require.Equal(t, UnsupportedPolicy, plan[1].State)
	require.Equal(t, Unavailable, plan[2].State)
	require.Nil(t, plan[2].GrantedAt)
	require.Equal(t, Denied, plan[3].State)
	require.Equal(t, Allowed, plan[4].State)
	require.Equal(t, four, *plan[4].GrantedAt)
	// 写入移除未知策略也不能把这个写入时刻冒充真实连续授权起点。
	current := Plan([]Source{{ID: "allow", From: at, Scope: im.ScopeTenant, Effect: "allow"}}, two, false)
	AssignEpochs(current, &plan[1], false)
	require.Equal(t, Unavailable, current[0].State)
}
