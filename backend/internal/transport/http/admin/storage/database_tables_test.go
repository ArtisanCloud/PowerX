package storage

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSystemTableRoutesRetainHumanRootBoundaryAndFixedCatalog(t *testing.T) {
	sts := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	sts.Issuer = "powerx-sts"
	for _, path := range []string{"/api/v1/admin/ops/storage/database-tables", "/api/v1/admin/ops/storage/database-tables/audit_event"} {
		for _, claims := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}}, sts} {
			require.Equal(t, 403, request(t, claims, "GET", path, "").Code)
		}
		require.Equal(t, 200, request(t, reqctx.CoreXClaims{UserID: 1, IsRoot: true}, "GET", path, "").Code)
	}
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	require.Equal(t, 400, request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables?schema=other", "").Code)
	require.Equal(t, 404, request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/iam_user", "").Code)
	require.Equal(t, 404, request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/arbitrary_sql", "").Code)
}

func TestTableSafetyRoutesRejectDelegatedAndArbitraryDatabaseOperations(t *testing.T) {
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	sts := root
	sts.Issuer = "powerx-sts"
	for _, op := range []struct{ method, path string }{
		{"GET", "monitoring"}, {"PUT", "monitoring"}, {"POST", "monitoring/adoptions"}, {"GET", "monitoring/requests/00000000-0000-4000-8000-000000000001"}, {"GET", "audit_event/history"}, {"GET", "capability_registry_health_probe_results/retention"}, {"PUT", "capability_registry_health_probe_results/retention"}, {"POST", "capability_registry_health_probe_results/retention/previews"}, {"POST", "capability_registry_health_probe_results/retention/runs"},
	} {
		for _, c := range []reqctx.CoreXClaims{{UserID: 1}, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}, {UserID: 1, IsRoot: true, Platforms: []string{"service_actor"}}, sts} {
			require.Equal(t, 403, request(t, c, op.method, "/api/v1/admin/ops/storage/database-tables/"+op.path, `{}`).Code)
		}
	}
	for _, id := range []string{"agent_chat_messages", "iam_authorization_tracking", "audit_event", "arbitrary_sql"} {
		require.Equal(t, 404, request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/"+id+"/retention", "").Code)
	}
	require.Equal(t, 400, request(t, root, "GET", "/api/v1/admin/ops/storage/database-tables/monitoring?schema=other", "").Code)
	require.Equal(t, 400, request(t, root, "PUT", "/api/v1/admin/ops/storage/database-tables/monitoring", `{"table":"audit_event","confirm":true}`).Code)
	require.Equal(t, 400, request(t, root, "POST", "/api/v1/admin/ops/storage/database-tables/capability_registry_health_probe_results/retention/runs", `{"expected_revision":0,"confirm":false}`).Code)
}
