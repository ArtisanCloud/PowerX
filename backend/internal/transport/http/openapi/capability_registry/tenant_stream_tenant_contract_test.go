package capability_registry

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// 与 Framework StreamInput.TenantUUID 生成的线上请求对照；租户只能取可信凭据。
func TestCoreInternalStreamCredentialTenantRejectsLegacyTenantHeader(t *testing.T) {
	for _, tenantHeader := range []string{"", apiKeyCatalogTestTenant, "00000000-0000-4000-8000-000000000099"} {
		t.Run("header="+tenantHeader, func(t *testing.T) {
			f := newInvocationHTTPFixture(t, false)
			binding, err := json.Marshal([]cm.ProtocolBinding{{Channel: "core_internal", Method: "INVOKE", Endpoint: "core://runtime/realtime", AuthType: "api_key_or_sts", ActorContext: "service_actor", ResourceScope: "tenant"}})
			require.NoError(t, err)
			require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Update("protocols", datatypes.JSON(binding)).Error)
			require.NoError(t, f.db.Model(&cm.AdapterEndpoint{}).Where("capability_id=?", f.capID).Updates(map[string]any{"transport_type": "core_internal", "endpoint": "core://runtime/realtime", "labels": datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
			envelope := streamEnvelope(f)
			envelope["preferred_protocol"] = "core_internal"
			raw, err := json.Marshal(envelope)
			require.NoError(t, err)
			req := httptest.NewRequest("POST", "/api/v1/tenant/invocations/stream?env=dev", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Authorization", "ApiKey "+f.secret)
			if tenantHeader != "" {
				req.Header.Set("tenant_uuid", tenantHeader)
			}
			w := httptest.NewRecorder()
			f.engine.ServeHTTP(w, req)
			if tenantHeader != "" {
				require.Equal(t, 400, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), "capability.tenant_override")
				require.Zero(t, f.core.calls)
				require.Zero(t, f.authz.calls)
				return
			}
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "event: ready")
			require.Equal(t, 1, f.core.calls)
			require.Equal(t, apiKeyCatalogTestTenant, f.core.input.TenantUUID)
			require.Equal(t, 1, f.authz.calls)
		})
	}
}
