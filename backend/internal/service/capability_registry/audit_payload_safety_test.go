package capability_registry

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	auditpayload "github.com/ArtisanCloud/PowerX/pkg/corex/audit/payload"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
)

func TestAuditInvocationBoundsAndRedactsEveryRecordWithoutChangingBusinessPayload(t *testing.T) {
	db := newAuditMemoryDB(t)
	a := NewAuditService(AuditServiceOptions{TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db)})
	request := map[string]any{"operation": "fixture", "password": "fixture-private", "long": strings.Repeat("body", 100000)}
	for i := 0; i < 3; i++ {
		a.RecordInvocation(context.Background(), InvocationAuditInput{TraceID: "same-trace", TenantUUID: "fixture-tenant", PluginID: "fixture-plugin", CapabilityID: "fixture-capability", ProtocolUsed: "core_internal", Status: "failed", RequestPayload: request, ResponsePayload: map[string]any{"access_token": "fixture-private-response"}, ErrorSummary: "upstream api_key=fixture-private-error failed"})
	}
	var rows []m.InvocationTrace
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.LessOrEqual(t, len(row.RequestPayload), auditpayload.MaxPayloadBytes)
		require.LessOrEqual(t, len(row.ResponsePayload), auditpayload.MaxPayloadBytes)
		require.NotContains(t, string(row.RequestPayload), "fixture-private")
		require.NotContains(t, string(row.ResponsePayload), "fixture-private")
		require.NotContains(t, row.ErrorSummary, "fixture-private")
		require.Equal(t, "failed", row.Status)
		require.NotNil(t, row.EventPublicationID)
	}
	var events []m.CapabilityEventPublication
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 3)
	for _, event := range events {
		require.NotContains(t, string(event.Payload), "fixture-private")
		require.Equal(t, "unconfirmed", event.Status)
	}
	require.Equal(t, "fixture-private", request["password"])
	require.Len(t, request["long"].(string), 400000)
}

func TestSuccessfulTypedBaselineReadsUseCompactSummaryAndPreserveAllRows(t *testing.T) {
	db := newAuditMemoryDB(t)
	a := NewAuditService(AuditServiceOptions{TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db)})
	members := make([]string, 100)
	for i := range members {
		members[i] = "fixture-member-" + strings.Repeat("x", 20)
	}
	permissions := make([]string, 10)
	for i := range permissions {
		permissions[i] = "fixture.permission.read"
	}
	decisions := make([]map[string]any, 1000)
	for i := range decisions {
		decisions[i] = map[string]any{"member_uuid": members[i%100], "permission_code": permissions[i%10], "status": "decided", "allowed": i%2 == 0, "authorization_revision": "revision", "scope_fingerprint": strings.Repeat("f", 64), "visible_since": "2026-11-01T00:00:00Z"}
	}
	response := map[string]any{"result": map[string]any{"schema": "powerx.iam.authorization-baseline-result/v1", "tenant_uuid": "fixture-tenant", "plugin_id": "fixture-caller", "evaluated_at": "2026-11-04T00:00:00Z", "items": decisions}}
	request := map[string]any{"body": map[string]any{"operation": "read_member_baselines", "request": map[string]any{"schema": "powerx.iam.authorization-baseline-query/v1", "member_uuids": members, "permission_codes": permissions}}}
	for i := 0; i < 3; i++ {
		a.RecordInvocation(context.Background(), InvocationAuditInput{TraceID: "baseline-shared", TenantUUID: "fixture-tenant", CapabilityID: "com.corex.iam.authorization_baseline.service_read", ProtocolUsed: "core_internal", Status: "completed", RequestPayload: request, ResponsePayload: response})
	}
	var rows []m.InvocationTrace
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 3)
	for _, row := range rows {
		require.Less(t, len(row.RequestPayload)+len(row.ResponsePayload), 2048)
		var data map[string]any
		require.NoError(t, json.Unmarshal(row.ResponsePayload, &data))
		require.EqualValues(t, 1000, data["item_count"])
		require.EqualValues(t, 500, data["allowed_count"])
		require.EqualValues(t, 500, data["denied_count"])
		require.NotEmpty(t, data["decision_digest"])
		require.NotContains(t, string(row.ResponsePayload), "scope_fingerprint")
	}
	require.Len(t, decisions, 1000)
	require.Equal(t, strings.Repeat("f", 64), decisions[0]["scope_fingerprint"])
	input := InvocationAuditInput{CapabilityID: "com.corex.iam.authorization_baseline.service_read", ProtocolUsed: "core_internal", Status: "failed", RequestPayload: request, ResponsePayload: response}
	_, _, ok := baselineAuditSummary(input)
	require.False(t, ok, "failed calls never use a successful-read summary")
}
