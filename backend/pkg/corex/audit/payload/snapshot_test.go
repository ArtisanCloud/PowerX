package payload

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotRedactsTypedAndNestedCopiesWithoutMutation(t *testing.T) {
	typed := struct {
		Ticket string `json:"ws_revalidation_handle"`
		Tenant string `json:"tenant_uuid"`
	}{"fixture-private-ticket", "fixture-tenant"}
	original := map[string]any{"headers": map[string]any{"Authorization": "Bearer fixture-private-bearer", "X-Api-Key": "fixture-private-key", "Cookie": "fixture-private-cookie"}, "result": typed, "encoded": `{"client_secret":"fixture-private-secret"}`, "message": "request https://user:fixture-private-password@example.invalid/path?access_token=fixture-private-query failed"}
	raw := Snapshot(original)
	require.True(t, json.Valid(raw))
	require.LessOrEqual(t, len(raw), MaxPayloadBytes)
	for _, secret := range []string{"fixture-private-ticket", "fixture-private-bearer", "fixture-private-key", "fixture-private-cookie", "fixture-private-secret", "fixture-private-password", "fixture-private-query"} {
		require.NotContains(t, string(raw), secret)
	}
	require.Contains(t, string(raw), "fixture-tenant")
	require.Equal(t, "fixture-private-ticket", typed.Ticket)
	require.Equal(t, "Bearer fixture-private-bearer", original["headers"].(map[string]any)["Authorization"])
}

func TestSnapshotIsBoundedForHugeCollectionsStringsAndCycles(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	for _, value := range []any{map[string]any{"large": strings.Repeat("fixture-private-body", 100000)}, map[string]any{"matrix": make([]any, 1000000)}, cycle, map[string]any{"prompt": "fixture-private-prompt", "messages": []any{"fixture-private-message"}}, map[string]any{"_audit": map[string]any{"mode": "already_safe"}, "password": "fixture-private-spoof"}} {
		raw := Snapshot(value)
		require.True(t, json.Valid(raw))
		require.LessOrEqual(t, len(raw), MaxPayloadBytes)
		require.NotContains(t, string(raw), "fixture-private")
	}
}

func TestRawJSONIsValidatedAndHardLimitCannotBeDisabled(t *testing.T) {
	for _, raw := range [][]byte{[]byte(`{"safe":true} invalid`), []byte(`{"password":"fixture-private"} {}`), []byte(strings.Repeat("x", 1000000))} {
		out, changed := JSON(raw, 1<<30)
		require.True(t, changed)
		require.True(t, json.Valid(out))
		require.LessOrEqual(t, len(out), MaxPayloadBytes)
		require.NotContains(t, string(out), "fixture-private")
	}
	out, changed := JSON([]byte(`{"note":"ok","password":"fixture-private"}`), 512)
	require.True(t, changed)
	require.LessOrEqual(t, len(out), 512)
	require.Contains(t, string(out), "ok")
	require.NotContains(t, string(out), "fixture-private")
}

func TestSnapshotAcceptsNamedBytesAndOmitsFreeFormBody(t *testing.T) {
	type privateByte uint8
	for _, v := range []any{[]privateByte{'{', '}'}, [2]privateByte{'{', '}'}, map[string]any{"body": "fixture-private-freeform"}} {
		raw := Snapshot(v)
		require.True(t, json.Valid(raw))
		require.NotContains(t, string(raw), "fixture-private-freeform")
	}
}

func TestErrorTextRemovesCredentialsAndEnforcesPostRedactionLimit(t *testing.T) {
	for _, raw := range []string{`failed password="fixture-private-pass" token=x api_key='fixture-private-key'`, "Authorization: Bearer fixture-private-bearer", `upstream {"client_secret":"fixture-private-\"quote"}`, "database postgres://user:fixture-private@db.invalid/x failed", "request failed\nfixture-private-body", strings.Repeat("api_key=x ", 180)} {
		out := SafeText(raw)
		require.LessOrEqual(t, len(out), MaxErrorBytes)
		require.NotContains(t, out, "fixture-private")
	}
}
