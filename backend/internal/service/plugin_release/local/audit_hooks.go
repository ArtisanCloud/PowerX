package local

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// AuditMetadata captures contextual data emitted alongside audit events.
type AuditMetadata struct {
	ArtifactURI  string
	FeatureFlags []string
	ResetCache   bool
	Force        bool
}

// AuditHooks observe lifecycle transitions for local hotload sessions.
type AuditHooks interface {
	OnSessionStarted(ctx context.Context, session *models.LocalInstallSession, meta AuditMetadata)
	OnSessionStopped(ctx context.Context, session *models.LocalInstallSession, meta AuditMetadata)
}

// NoopAuditHooks provides a default implementation that performs no operations.
type NoopAuditHooks struct{}

func (NoopAuditHooks) OnSessionStarted(context.Context, *models.LocalInstallSession, AuditMetadata) {}
func (NoopAuditHooks) OnSessionStopped(context.Context, *models.LocalInstallSession, AuditMetadata) {}

// auditHooks implements AuditHooks using the shared audit service.
type auditHooks struct {
	auditor audit.Auditor
}

// NewAuditHooks constructs audit hooks from shared auditor.
func NewAuditHooks(auditor audit.Auditor) AuditHooks {
	if auditor == nil {
		return NoopAuditHooks{}
	}
	return &auditHooks{auditor: auditor}
}

func (h *auditHooks) OnSessionStarted(ctx context.Context, session *models.LocalInstallSession, meta AuditMetadata) {
	metaPayload := map[string]any{
		"session_id":   session.UUID.String(),
		"artifact_uri": strings.TrimSpace(meta.ArtifactURI),
		"feature_flags": func() []string {
			if len(meta.FeatureFlags) == 0 {
				return nil
			}
			return meta.FeatureFlags
		}(),
		"reset_cache":           meta.ResetCache,
		"status":                session.Status,
		"developer_member_uuid": session.DeveloperMemberUUID,
		"hotload_started":       time.Now().UTC().Format(time.RFC3339Nano),
		"hotload_expires":       session.ExpiredAt,
		"tenant_uuid":           strings.TrimSpace(session.TenantUUID),
		"actor":                 actorFromContext(ctx),
	}
	h.emit(ctx, "plugin_release.local_install.start", metaPayload)
}

func (h *auditHooks) OnSessionStopped(ctx context.Context, session *models.LocalInstallSession, meta AuditMetadata) {
	metaPayload := map[string]any{
		"session_id":            session.UUID.String(),
		"status":                session.Status,
		"force":                 meta.Force,
		"tenant_uuid":           strings.TrimSpace(session.TenantUUID),
		"developer_member_uuid": session.DeveloperMemberUUID,
		"hotload_finished":      time.Now().UTC().Format(time.RFC3339Nano),
		"actor":                 actorFromContext(ctx),
	}
	h.emit(ctx, "plugin_release.local_install.stop", metaPayload)
}

func (h *auditHooks) emit(ctx context.Context, action string, meta map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			// ensure audit failures never panic the hotload path
		}
	}()
	payload, err := json.Marshal(meta)
	if err != nil {
		payload = []byte(fmt.Sprintf(`{"_marshal_error":"%v"}`, err))
	}
	h.auditor.LogAPI(ctx, action, 200, 0)
	// Additional structured sink could be added via future auditor enhancements
	_ = payload
}

func actorFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.MemberUUID != "" {
		return claims.MemberUUID
	}
	if sub := reqctx.GetSubject(ctx); sub != "" {
		return sub
	}
	if v := ctx.Value("authorization"); v != nil {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
