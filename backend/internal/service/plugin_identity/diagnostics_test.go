package plugin_identity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func captureRejections(s *Service) *observer.ObservedLogs {
	core, logs := observer.New(zap.WarnLevel)
	l := zap.New(core)
	s.rejectionLogger = func(_ context.Context, msg string, fields ...zap.Field) { l.Warn(msg, fields...) }
	return logs
}

func diagnosticInput(f fixture) IssueInput {
	return IssueInput{PluginID: f.plugin, Claims: f.user, Method: "GET", Path: "/admin/operations/orders", Transport: "http", RequestID: "original-request", TraceID: "original-trace"}
}

func diagnosticContext() context.Context {
	return audit.WithCorrelationID(reqctx.WithTraceID(context.Background(), "current-trace"), "request-correlation")
}

func TestIssueRejectionSeparatesAccessAndServerSessionFailures(t *testing.T) {
	cases := []struct {
		reason string
		change func(*testing.T, fixture, *IssueInput)
	}{
		{"user_access_expired", func(t *testing.T, f fixture, in *IssueInput) {
			in.Claims.ExpiresAt = jwt.NewNumericDate(f.s.clock().Add(-time.Second))
		}},
		{"user_access_expiry_missing", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.ExpiresAt = nil }},
		{"session_id_missing", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.ID = "" }},
		{"user_issuer_mismatch", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.Issuer = "other" }},
		{"user_audience_mismatch", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.Audience = []string{"other"} }},
		{"user_scope_invalid", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.Scope = "refresh" }},
		{"request_identity_fields_invalid", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.UserUUID = "" }},
		{"session_not_found", func(t *testing.T, f fixture, in *IssueInput) { in.Claims.ID = uuid.NewString() }},
		{"session_expired", func(t *testing.T, f fixture, in *IssueInput) {
			require.NoError(t, f.db.Model(&f.session).Update("expires_at", f.s.clock().Add(-time.Second).UnixMilli()).Error)
		}},
		{"session_revoked", func(t *testing.T, f fixture, in *IssueInput) {
			require.NoError(t, f.db.Model(&f.session).Update("revoked_at", f.s.clock().UnixMilli()).Error)
		}},
		{"session_identity_mismatch", func(t *testing.T, f fixture, in *IssueInput) {
			require.NoError(t, f.db.Model(&f.session).Update("member_uuid", uuid.NewString()).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			f := newFixture(t, false)
			logs := captureRejections(f.s)
			in := diagnosticInput(f)
			tc.change(t, f, &in)
			ticket, err := f.s.Issue(diagnosticContext(), in)
			require.Empty(t, ticket)
			require.Equal(t, 401, dto.StatusCode(err))
			require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", dto.CodeOf(err))
			require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", err.Error())
			require.Nil(t, dto.DetailsOf(err), "服务端诊断不能进入公开错误明细")
			require.Len(t, logs.All(), 1)
			entry := logs.All()[0]
			require.Equal(t, "iam_request_identity_rejected", entry.Message)
			fields := entry.ContextMap()
			require.Equal(t, tc.reason, fields["reason"])
			require.Equal(t, "issue", fields["phase"])
			require.Equal(t, "original-request", fields["source_request_id"])
			require.Equal(t, "original-trace", fields["source_trace_id"])
			require.Equal(t, "current-trace", fields["trace_id"])
			require.Equal(t, "request-correlation", fields["correlation_id"])
			require.Equal(t, f.tenant, fields["tenant_uuid"])
			if in.Claims.ID != "" {
				require.Equal(t, hash(in.Claims.ID), fields["session_ref"])
			}
			if tc.reason == "session_expired" {
				require.Contains(t, fields, "user_access_expires_at")
				require.Contains(t, fields, "session_expires_at")
				checked, e := time.Parse(time.RFC3339Nano, fields["checked_at"].(string))
				require.NoError(t, e)
				accessExpiry, e := time.Parse(time.RFC3339Nano, fields["user_access_expires_at"].(string))
				require.NoError(t, e)
				require.True(t, accessExpiry.After(checked), "复现 access 仍有效、服务端会话已经到期")
			}
			if tc.reason == "session_revoked" {
				require.Contains(t, fields, "session_revoked_at")
			}
			if tc.reason == "session_identity_mismatch" {
				require.Equal(t, true, fields["session_user_matches"])
				require.Equal(t, true, fields["session_tenant_matches"])
				require.Equal(t, false, fields["session_member_matches"])
			}
		})
	}
}

func TestResolveAndWSRejectionKeepOriginalRequestCorrelation(t *testing.T) {
	for _, phase := range []string{"resolve", "ws_revalidate"} {
		t.Run(phase, func(t *testing.T) {
			f := newFixture(t, true)
			in := diagnosticInput(f)
			in.Transport = "ws_grant"
			ticket, err := f.s.Issue(diagnosticContext(), in)
			require.NoError(t, err)
			initial, err := f.s.Resolve(f.svc, query(ticket))
			require.NoError(t, err)
			logs := captureRejections(f.s)
			require.NoError(t, f.db.Model(&f.session).Update("revoked_at", f.s.clock().UnixMilli()).Error)
			var result Result
			if phase == "resolve" {
				result, err = f.s.Resolve(f.svc, query(ticket))
			} else {
				result, err = f.s.RevalidateWS(f.svc, wsQuery(initial))
			}
			require.Empty(t, result.Permissions)
			require.Equal(t, 401, dto.StatusCode(err))
			require.Equal(t, "IAM_CONTEXT_USER_SESSION_INVALID", dto.CodeOf(err))
			require.Len(t, logs.All(), 1, "一次失败只输出一条拒绝事件")
			fields := logs.All()[0].ContextMap()
			require.Equal(t, phase, fields["phase"])
			require.Equal(t, "session_revoked", fields["reason"])
			require.Equal(t, "original-trace", fields["source_trace_id"])
			require.Equal(t, hash(f.session.JTI), fields["session_ref"])
			require.Equal(t, true, fields["is_root"])
		})
	}
}

type diagnosticFailingStore struct{ cache.ICache }

func (s diagnosticFailingStore) Set(context.Context, string, interface{}, time.Duration) error {
	return errors.New("dependency error with sensitive-credential-sentinel")
}

func TestRejectionLogsExcludeCredentialsAndDependencyMessages(t *testing.T) {
	f := newFixture(t, false)
	logs := captureRejections(f.s)
	in := diagnosticInput(f)
	in.Path += "?access_token=access-secret&context_ticket=ticket-secret#fragment-secret"
	f.s.store = diagnosticFailingStore{f.store}
	_, err := f.s.Issue(diagnosticContext(), in)
	require.Equal(t, 503, dto.StatusCode(err))
	require.Equal(t, "IAM_CONTEXT_UNAVAILABLE", dto.CodeOf(err))
	require.Len(t, logs.All(), 1)
	fields := logs.All()[0].ContextMap()
	require.Equal(t, "ticket_store_failed", fields["reason"])
	require.Equal(t, "/admin/operations/orders", fields["path"])
	raw, err := json.Marshal(fields)
	require.NoError(t, err)
	for _, secret := range []string{f.session.JTI, "access-secret", "ticket-secret", "fragment-secret", "sensitive-credential-sentinel", f.key.KeyHash} {
		require.NotContains(t, string(raw), secret)
	}
	_, err = f.s.Resolve(f.svc, query("raw-invalid-ticket-secret"))
	require.Equal(t, "IAM_CONTEXT_PROOF_INVALID", dto.CodeOf(err))
	_, err = f.s.RevalidateWS(f.svc, WSQuery{Schema: WSQuerySchema, Handle: "raw-invalid-handle-secret"})
	require.Equal(t, "IAM_CONTEXT_WS_HANDLE_INVALID", dto.CodeOf(err))
	for _, entry := range logs.All() {
		raw, err := json.Marshal(entry.ContextMap())
		require.NoError(t, err)
		require.NotContains(t, string(raw), "raw-invalid-ticket-secret")
		require.NotContains(t, string(raw), "raw-invalid-handle-secret")
	}
	logs.TakeAll()
	in.Claims.ID = ""
	in.RequestID = "Bearer secret"
	in.TraceID = strings.Repeat("x", 129)
	_, err = f.s.Issue(diagnosticContext(), in)
	require.Error(t, err)
	fields = logs.All()[0].ContextMap()
	require.Empty(t, fields["source_request_id"])
	require.Empty(t, fields["source_trace_id"])
}

func TestSessionDependencyAndSubjectStateHaveDistinctDiagnostics(t *testing.T) {
	for _, kind := range []string{"session_lookup_failed", "user_state_changed", "member_disabled"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, false)
			logs := captureRejections(f.s)
			status, code := 403, "IAM_CONTEXT_USER_FORBIDDEN"
			switch kind {
			case "session_lookup_failed":
				db, err := f.db.DB()
				require.NoError(t, err)
				require.NoError(t, db.Close())
				status, code = 503, "IAM_CONTEXT_UNAVAILABLE"
			case "user_state_changed":
				require.NoError(t, f.db.Model(&im.User{}).Where("uuid=?", f.user.UserUUID).Update("status", 2).Error)
			case "member_disabled":
				require.NoError(t, f.db.Model(&im.Member{}).Where("uuid=?", f.user.MemberUUID).Update("status", 2).Error)
			}
			_, err := f.s.Issue(diagnosticContext(), diagnosticInput(f))
			require.Equal(t, status, dto.StatusCode(err))
			require.Equal(t, code, dto.CodeOf(err))
			require.Len(t, logs.All(), 1)
			require.Equal(t, kind, logs.All()[0].ContextMap()["reason"])
		})
	}
}

func TestSuccessfulIdentityDoesNotEmitRejection(t *testing.T) {
	f := newFixture(t, false)
	logs := captureRejections(f.s)
	ticket, err := f.s.Issue(diagnosticContext(), diagnosticInput(f))
	require.NoError(t, err)
	_, err = f.s.Resolve(f.svc, query(ticket))
	require.NoError(t, err)
	require.Empty(t, logs.All())
}
