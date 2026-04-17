package backup_ops

import (
	"context"
	"strconv"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func publishBackupJobStatus(ctx context.Context, payload map[string]any) {
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" {
		return
	}
	body := map[string]any{
		"type":      "info",
		"title":     "备份任务更新",
		"content":   "备份任务状态已更新",
		"kind":      "ops.backup.job",
		"category":  "ops.backup",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"isRead":    false,
	}
	for k, v := range payload {
		body[k] = v
	}
	wsbus.DefaultHub.Publish(tenantUUID, eventbus.TopicSystemNotification, body, strings.TrimSpace(reqctx.GetTraceID(ctx)))
}

func publishRestoreDrillStatus(ctx context.Context, payload map[string]any) {
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" {
		return
	}
	body := map[string]any{
		"type":      "info",
		"title":     "恢复验证任务更新",
		"content":   "恢复验证任务状态已更新",
		"kind":      "ops.backup.restore",
		"category":  "ops.backup",
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"isRead":    false,
	}
	for k, v := range payload {
		body[k] = v
	}
	wsbus.DefaultHub.Publish(tenantUUID, eventbus.TopicSystemNotification, body, strings.TrimSpace(reqctx.GetTraceID(ctx)))
}

func toStringUint(v uint64) string {
	return strconv.FormatUint(v, 10)
}
