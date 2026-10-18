package backup

import (
	"context"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// BackupEvents 仅推送资源失效通知；详情仍由已鉴权的 typed API 查询。
func (h *handler) BackupEvents(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_EVENTS_QUERY_FORBIDDEN", "状态推送不接受 query 参数", nil))
		return
	}
	resources := map[string][]string{
		"snapshots":      h.snapshots.ChangeDirectories(),
		"recovery":       h.coordinator.ChangeDirectories(),
		"archives":       h.archives.ChangeDirectories(),
		"restore_copies": h.copySvc.ChangeDirectories(),
	}
	ctx := c.Request.Context()
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.ExpiresAt != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, claims.ExpiresAt.Time)
		defer cancel()
	}
	sub, e := opswatch.Subscribe(ctx, resources, func(path string) bool { return filepath.Base(path) != "heartbeat.json" })
	if e != nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "BACKUP_EVENTS_UNAVAILABLE", "备份状态推送暂不可用，请手动刷新", nil))
		return
	}
	defer sub.Close()
	_ = eventstream.Serve(ctx, c.Writer, sub, func(names []string) (eventstream.Event, error) {
		if names == nil {
			names = []string{"snapshots", "recovery", "archives", "restore_copies"}
		}
		return eventstream.Event{Name: "invalidate", Data: gin.H{"resources": names}}, nil
	})
}
