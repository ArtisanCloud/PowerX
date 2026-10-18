package evidence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestArchivePreservesExactSourceBytesAndCannotBecomeNewExecution(t *testing.T) {
	f := numericFixture(t, "12")
	reader := &authorizedFixtureSource{tenant: f.run.identity.TenantUUID, object: uuid.NewString(), user: reqctx.GetUserUUID(f.ctx), allow: true, field: f.policy.Fields["a"], json: []byte("{\n  \"customers\": 30,\n  \"note\": \"\\u4eba\"\n}\n")}
	svc := NewToolService(ToolServiceOptions{Objects: map[string]AuthorizedObjectSource{"example.customer_snapshot": reader}})
	run, err := svc.NewRun(f.ctx, f.run.Identity(), map[string]any{"message": "fixture"}, f.policy)
	require.NoError(t, err)
	_, err = run.ResolveDatum(f.ctx, SourceLocation{Kind: "authorized_object", ObjectType: "example.customer_snapshot", ObjectUUID: reader.object, Pointer: "/customers", Quote: "30", Literal: "30", Range: ByteRange{EndByte: 2}}, reader.field)
	require.NoError(t, err)
	labels := ResponseLabels{Schema: "powerx.response-labels/v1", RevisionUUID: run.Identity().RevisionUUID, Locales: map[string]map[string]string{"en-US": {"a.label": "Quantity"}}}
	a, err := run.ExportArchive(f.ctx, nil, labels)
	require.NoError(t, err)
	require.NoError(t, a.VerifyOrigin(f.ctx))
	raw, err := json.MarshalIndent(a, "", "  ")
	require.NoError(t, err)
	var restored RunEvidenceArchive
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.NoError(t, ValidateArchive(restored))
	found := false
	for _, source := range restored.Sources {
		if source.Kind == "authorized_object" {
			found = true
			require.Equal(t, []byte(reader.json), source.JSON)
			require.Equal(t, toolDigest(reader.json), source.SHA256)
		}
	}
	require.True(t, found)
	require.ErrorContains(t, restored.VerifyOrigin(f.ctx), "evidence.tool_receipt_untrusted")
	a.RunID = "forged"
	require.ErrorContains(t, a.VerifyOrigin(f.ctx), "evidence.tool_receipt_untrusted")
}

func TestUnicodePercentBoundariesCannotBeTruncatedIntoQuantities(t *testing.T) {
	for _, space := range []string{" ", "\t", "\u00a0", "\u3000", "\u200b"} {
		t.Run(string([]rune(space)), func(t *testing.T) {
			text := "rate:27.8" + space + "%"
			f := newToolFixture(t, map[string]any{"message": text})
			start := strings.Index(text, "27.8")
			field := f.policy.Fields["rate"]
			field.Unit, field.DatumKind = "1", "quantity"
			policy := f.policy
			policy.Fields = map[string]SourceField{"rate": field}
			run, err := f.service.NewRun(f.ctx, f.run.Identity(), map[string]any{"message": text}, policy)
			require.NoError(t, err)
			_, err = run.ResolveDatum(f.ctx, SourceLocation{Kind: "run_input", ObjectType: "powerx.run_input", ObjectUUID: run.InputObjectUUID(), Pointer: "/message", Quote: "27.8", Literal: "27.8", Range: ByteRange{StartByte: start, EndByte: start + len("27.8")}}, field)
			require.Error(t, err)
			require.False(t, containsQuotedNumericToken(text, "27.8", "27.8"))
			tokens, err := TokenizeGenericSources(map[string]any{"message": text}, []string{"/message"})
			require.NoError(t, err)
			if space == "\u200b" {
				require.Empty(t, tokens)
			} else {
				require.Len(t, tokens, 1)
				require.Equal(t, "%", tokens[0].Unit)
				datum := f.resolve(t, "rate", "/message", "27.8"+space+"%", "27.8", start)
				require.Equal(t, "reported", datum.DatumKind)
			}
		})
	}
	// 检查只使用现有 ledger，历史验证绝不生成新的运行信任。
	f := numericFixture(t, "12")
	_, err := f.run.ExportArchive(context.Background(), nil, ResponseLabels{})
	require.Error(t, err)
}
