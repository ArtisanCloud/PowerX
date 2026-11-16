package storage

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDatabaseMaintenanceRoutesOnlyAcceptHumanRoot(t *testing.T) {
	sts := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	sts.Issuer = "powerx-sts"
	for _, route := range []struct{ method, path string }{{"GET", "/database-maintenance"}, {"POST", "/database-maintenance/previews"}, {"POST", "/database-maintenance/runs"}, {"GET", "/database-maintenance/runs/11111111-1111-4111-8111-111111111111"}, {"POST", "/database-maintenance/runs/11111111-1111-4111-8111-111111111111/cancellations"}, {"POST", "/database-maintenance/runs/11111111-1111-4111-8111-111111111111/reviews"}} {
		for _, claims := range []reqctx.CoreXClaims{{UserID: 1, IsRoot: false}, sts, {UserID: 1, IsRoot: true, Platforms: []string{"api_key"}}} {
			w := request(t, claims, route.method, "/api/v1/admin/ops/storage"+route.path, `{}`)
			require.Equal(t, 403, w.Code)
		}
	}
}
func TestDatabaseMaintenanceCannotAcceptSQLDatabaseOrSessionOverrides(t *testing.T) {
	root := reqctx.CoreXClaims{UserID: 1, IsRoot: true}
	for _, body := range []string{`{"mode":"reuse","tables":["capability-invocation-traces"],"sql":"TRUNCATE iam_user"}`, `{"mode":"compact","tables":["capability-invocation-traces"],"database":"production"}`, `{"mode":"reuse","tables":[],"pid":1}`} {
		w := request(t, root, "POST", "/api/v1/admin/ops/storage/database-maintenance/previews", body)
		require.Equal(t, 400, w.Code)
	}
	w := request(t, root, "GET", "/api/v1/admin/ops/storage/database-maintenance?database=other", "")
	require.Equal(t, 400, w.Code)
	w = request(t, root, "GET", "/api/v1/admin/ops/storage/database-maintenance", "")
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"ready":false`)
	require.Contains(t, w.Body.String(), `"runs":[]`)
	require.NotContains(t, w.Body.String(), `"database_bytes":0`)
}
