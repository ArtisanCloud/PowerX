package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestManualSnapshotHTTPBindsSelectedSystemPolicy(t *testing.T) {
	// 本用例验证入队时的策略绑定与幂等，不启动会竞争策略锁的后台采集／清理。
	svc, engine, root, member := snapshotHTTPFixture(t, snapshotops.Options{DispatchCapture: func(context.Context, string) error { return nil }})
	ctx := reqctx.WithIsRoot(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true}), true)
	p := snapshotops.Policy{IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 7}
	a, e := svc.PutPolicy(ctx, "", "Daily", p)
	require.NoError(t, e)
	call := func(token, key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/custom/v2/admin/ops/backup/system-snapshots", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		out := httptest.NewRecorder()
		engine.ServeHTTP(out, req)
		return out
	}
	body := fmt.Sprintf(`{"policy_id":%q}`, a.ID)
	require.Equal(t, 403, call(member, uuid.NewString(), body).Code)
	require.Equal(t, 400, call(root, uuid.NewString(), `{}`).Code)
	require.Equal(t, 400, call(root, uuid.NewString(), `{"policy_id":"invalid"}`).Code)
	require.Equal(t, 400, call(root, uuid.NewString(), fmt.Sprintf(`{"policy_id":%q,"directory":"override"}`, a.ID)).Code)
	key := uuid.NewString()
	created := call(root, key, body)
	require.Equal(t, 202, created.Code, created.Body.String())
	var result struct {
		Data struct {
			Snapshot snapshotops.Snapshot `json:"snapshot"`
		}
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &result))
	first := result.Data.Snapshot
	require.Equal(t, a.ID, first.PolicyID)
	require.Equal(t, a.Name, first.PolicyName)
	b, e := svc.PutPolicy(ctx, "", "Upgrade", p)
	require.NoError(t, e)
	_, e = svc.SetDefaultPolicy(ctx, b.ID)
	require.NoError(t, e)
	stale := call(root, uuid.NewString(), body)
	require.Equal(t, 409, stale.Code)
	require.Contains(t, stale.Body.String(), "SNAPSHOT_POLICY_CHANGED")
	replayed := call(root, key, body)
	require.Equal(t, 202, replayed.Code, replayed.Body.String())
	require.NoError(t, json.Unmarshal(replayed.Body.Bytes(), &result))
	require.Equal(t, first.ID, result.Data.Snapshot.ID)
	conflict := call(root, key, fmt.Sprintf(`{"policy_id":%q}`, b.ID))
	require.Equal(t, 409, conflict.Code)
	require.Contains(t, conflict.Body.String(), "SNAPSHOT_IDEMPOTENCY_CONFLICT")
}

func TestSnapshotPolicyHTTPCollectionAndExclusiveActivation(t *testing.T) {
	_, engine, root, member := snapshotHTTPFixture(t)
	base := "/custom/v2/admin/ops/backup/system-snapshots/policies"
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		engine.ServeHTTP(out, req)
		return out
	}
	body := `{"name":"Daily","enabled":false,"interval_minutes":1440,"retention_days":7,"retention_count":7}`
	require.Equal(t, 401, call("POST", base, "", body).Code)
	require.Equal(t, 403, call("POST", base, member, body).Code)
	created := call("POST", base, root, body)
	require.Equal(t, 201, created.Code, created.Body.String())
	var result struct {
		Data struct {
			Policy snapshotops.NamedPolicy `json:"policy"`
		}
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &result))
	first := result.Data.Policy
	require.True(t, first.Default)
	require.False(t, first.Enabled)
	require.Equal(t, 409, call("POST", base, root, body).Code)
	second := call("POST", base, root, `{"name":"Before upgrade","enabled":true,"interval_minutes":1440,"retention_days":7,"retention_count":2}`)
	require.Equal(t, 201, second.Code, second.Body.String())
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &result))
	require.True(t, result.Data.Policy.Enabled)
	secondPolicyID := result.Data.Policy.ID
	activated := call("PATCH", base+"/"+first.ID+"/status", root, `{"enabled":true}`)
	require.Equal(t, 200, activated.Code, activated.Body.String())
	listed := call("GET", base, root, "")
	var list struct {
		Data struct {
			Policies  []snapshotops.NamedPolicy `json:"policies"`
			DefaultID string                    `json:"default_policy_id"`
		}
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &list))
	require.Len(t, list.Data.Policies, 2)
	require.Equal(t, first.ID, list.Data.DefaultID)
	enabled := 0
	for _, p := range list.Data.Policies {
		if p.Enabled {
			enabled++
			require.Equal(t, first.ID, p.ID)
		}
	}
	require.Equal(t, 1, enabled)
	require.Equal(t, 400, call("PATCH", base+"/"+first.ID+"/status", root, `{}`).Code)
	require.Equal(t, 400, call("PATCH", base+"/"+first.ID+"/status", root, `{"enabled":true,"schedule_anchor":"override"}`).Code)
	require.NotContains(t, listed.Body.String(), "schedule_anchor")
	require.NotContains(t, listed.Body.String(), "private-value")
	selected := call("PATCH", base+"/"+secondPolicyID+"/status", root, `{"enabled":false,"make_default":true}`)
	require.Equal(t, 200, selected.Code, selected.Body.String())
	require.NoError(t, json.Unmarshal(selected.Body.Bytes(), &result))
	require.True(t, result.Data.Policy.Default)
	require.False(t, result.Data.Policy.Enabled)
	require.Equal(t, secondPolicyID, result.Data.Policy.ID)
	require.Equal(t, 400, call("PATCH", base+"/"+first.ID+"/status", root, `{"enabled":true,"make_default":true}`).Code)
	listed = call("GET", base, root, "")
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &list))
	for _, policy := range list.Data.Policies {
		require.False(t, policy.Enabled, "default selection alone does not enable a schedule")
	}
}
