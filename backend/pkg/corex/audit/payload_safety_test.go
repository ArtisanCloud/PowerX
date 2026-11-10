package audit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	auditpayload "github.com/ArtisanCloud/PowerX/pkg/corex/audit/payload"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/stretchr/testify/require"
)

func TestAuditEmitterQueuesBoundedRedactedCopyAndPreservesCallerEvent(t *testing.T) {
	s := &serviceImpl{ch: make(chan queuedAuditEvent, 1)}
	original := &m.AuditEvent{Operation: "UPDATE", ResourceID: "fixture-resource", CorrelationID: "fixture-trace", Meta: []byte(`{"api_key":"fixture-private-meta","trace_id":"fixture-trace"}`), ChangesBefore: []byte(`{"password":"fixture-private-old"}`), ChangesAfter: []byte(`{"password":"fixture-private-new"}`), ChangesDiff: []byte(`{"body":"` + strings.Repeat("x", 100000) + `"}`)}
	require.NoError(t, s.Emit(context.Background(), original))
	queued := <-s.ch
	require.NotSame(t, original, queued.evt)
	require.True(t, queued.evt.ChangesRedacted)
	for _, field := range [][]byte{queued.evt.Meta, queued.evt.ChangesBefore, queued.evt.ChangesAfter, queued.evt.ChangesDiff} {
		require.True(t, json.Valid(field))
		require.LessOrEqual(t, len(field), auditpayload.MaxPayloadBytes)
		require.NotContains(t, string(field), "fixture-private")
	}
	require.Contains(t, string(queued.evt.Meta), "fixture-trace")
	require.Contains(t, string(original.Meta), "fixture-private-meta")
	require.Contains(t, string(original.ChangesBefore), "fixture-private-old")
	require.False(t, original.ChangesRedacted)
	require.Equal(t, "fixture-resource", queued.evt.ResourceID)
	require.Equal(t, "fixture-trace", queued.evt.CorrelationID)
}
