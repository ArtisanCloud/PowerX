package backup_ops

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notifications "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func publishBackupJobStatus(ctx context.Context, db *gorm.DB, job *modelops.BackupJob) {
	if job.Status != modelops.BackupJobStatusSuccess && job.Status != modelops.BackupJobStatusFailed {
		return
	}
	id := toStringUint(job.ID)
	input := notifications.CreateInput{
		Title: "数据库备份完成", Content: fmt.Sprintf("备份任务 #%s 已完成，可查看备份文件及执行记录。", id),
		Type: "success", Category: "system", RelatedID: id, RelatedType: "backup_job",
		Navigation: &notifications.Navigation{Label: "查看备份记录", Path: "/ops/backup?job_id=" + id},
	}
	if job.Status == modelops.BackupJobStatusFailed {
		input.Title, input.Content, input.Type, input.IsImportant = "数据库备份失败", fmt.Sprintf("备份任务 #%s 失败，请查看执行记录。", id), "error", true
	}
	publishTaskNotification(ctx, db, input, job.TriggerType == modelops.BackupTriggerTypeScheduled)
}

func publishRestoreDrillStatus(ctx context.Context, db *gorm.DB, drill *modelops.RestoreDrillRecord) {
	if drill.Status != modelops.RestoreDrillStatusSuccess && drill.Status != modelops.RestoreDrillStatusFailed {
		return
	}
	id := toStringUint(drill.ID)
	input := notifications.CreateInput{
		Title: "恢复验证完成", Content: fmt.Sprintf("恢复验证 #%s 已完成，来源备份 #%d。", id, drill.SourceJobID),
		Type: "success", Category: "system", RelatedID: id, RelatedType: "restore_drill",
		Navigation: &notifications.Navigation{Label: "查看恢复验证", Path: "/ops/backup?drill_id=" + id},
	}
	if drill.Status == modelops.RestoreDrillStatusFailed {
		input.Title, input.Content, input.Type, input.IsImportant = "恢复验证失败", fmt.Sprintf("恢复验证 #%s 失败，请查看验证报告。", id), "error", true
	}
	publishTaskNotification(ctx, db, input, drill.Operator == "system.scheduler")
}

// 结果先落入正式通知，再发送同一个 ID。通知失败不能使已完成的任务重跑，
// 也不能降级为无归属的临时广播；任务进度仍由现有备份事件推送。
func publishTaskNotification(ctx context.Context, db *gorm.DB, input notifications.CreateInput, scheduled bool) {
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenant == "" || db == nil {
		return
	}
	members, err := taskNotificationRecipients(ctx, db, tenant, scheduled)
	if err != nil {
		logOp(ctx, "error", "backup.notification.recipients_failed", zap.Error(err))
		return
	}
	svc := notifications.NewService(db)
	for _, member := range members {
		input.TenantUUID, input.MemberUUID = tenant, member
		item, err := svc.Create(ctx, input)
		if err != nil {
			logOp(ctx, "error", "backup.notification.create_failed", zap.String("related_id", input.RelatedID), zap.Error(err))
			continue
		}
		wsbus.DefaultHub.PublishWithContext(ctx, tenant, eventbus.TopicSystemNotification, notifications.CreatedEvent(item), strings.TrimSpace(reqctx.GetTraceID(ctx)))
	}
}

func taskNotificationRecipients(ctx context.Context, db *gorm.DB, tenant string, scheduled bool) ([]string, error) {
	if claims := reqctx.GetClaims(ctx); claims != nil {
		// 绑定已经通过入口鉴权的发起人；长任务完成时无需 JWT 仍未过期。
		// Create 会再检查实时 IAM 状态，WS 写帧另行校验接收端会话。
		if claims.TenantUUID != tenant || claims.UserID != reqctx.GetUserID(ctx) || claims.MemberID != reqctx.GetMemberID(ctx) {
			return nil, notifications.ErrRecipientInvalid
		}
		if claims.MemberID == 0 {
			return nil, nil
		}
		var member im.Member
		err := db.WithContext(ctx).Where("tenant_uuid = ? AND id = ? AND user_id = ? AND status = ?", tenant, claims.MemberID, claims.UserID, 1).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, notifications.ErrRecipientInvalid
		}
		if err != nil {
			return nil, err
		}
		return []string{member.UUID.String()}, nil
	}
	if member := strings.TrimSpace(reqctx.GetMemberUUID(ctx)); member != "" {
		// Create 再次校验租户、成员和账号；无效的显式身份不能退回广播。
		return []string{member}, nil
	}
	if !scheduled {
		return nil, nil
	}
	// 计划任务只通知执行租户内有效的 Root 成员，不广播实例备份信息。
	users := db.Model(&im.User{}).Select("id").Where("is_root = ? AND status = ?", true, 1)
	var members []im.Member
	err := db.WithContext(ctx).Where("tenant_uuid = ? AND status = ? AND user_id IN (?)", tenant, 1, users).Find(&members).Error
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UUID.String())
	}
	return ids, err
}

func toStringUint(v uint64) string {
	return strconv.FormatUint(v, 10)
}
