package audit

import (
	"context"
	"encoding/json"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
)

func AttachSupportSessionMeta(ctx context.Context, evt *dbm.AuditEvent) {
	if ctx == nil || evt == nil {
		return
	}
	supportID := reqctx.GetSupportSessionID(ctx)
	if supportID == 0 {
		return
	}
	meta := map[string]any{}
	if len(evt.Meta) > 0 {
		_ = json.Unmarshal(evt.Meta, &meta)
	}
	meta["support_session_id"] = supportID
	if target := reqctx.GetSupportSessionTargetTenantUUID(ctx); target != "" {
		meta["support_session_target_tenant_uuid"] = target
	}
	if mode := reqctx.GetSupportSessionMode(ctx); mode != "" {
		meta["support_session_mode"] = mode
	}
	raw, _ := json.Marshal(meta)
	evt.Meta = datatypes.JSON(raw)
}
