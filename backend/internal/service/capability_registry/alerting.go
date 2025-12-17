package capability_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// CapabilityAlerting 定义告警接口，便于替换和测试。
type CapabilityAlerting interface {
	NotifyAssetIssue(ctx context.Context, input AssetAlertInput)
}

const (
	AssetAlertReasonExposureMissing = "exposure_missing"
	AssetAlertReasonSchemaMissing   = "schema_missing"
	AssetAlertReasonSchemaInvalid   = "schema_invalid"
)

// NotificationSender 与企业 IM 发送器兼容。
type NotificationSender interface {
	Send(ctx context.Context, msg imnotify.Message) error
}

// AlertingOptions 注入 AlertingService 依赖。
type AlertingOptions struct {
	Audit    auditpkg.Service
	Notifier NotificationSender
	Logger   *pxlog.Logger
	Clock    func() time.Time
}

// AlertingService 负责资产告警持久化与通知。
type AlertingService struct {
	audit    auditpkg.Service
	notifier NotificationSender
	logger   *pxlog.Logger
	now      func() time.Time
}

// AssetAlertInput 描述一次资产告警的上下文。
type AssetAlertInput struct {
	PluginID      string
	PluginName    string
	PluginVersion string
	CapabilityID  string
	AssetPath     string
	ArtifactPath  string
	Reason        string
	Detail        string
}

// AssetAlertError 用于在同步流程中包装资产异常。
type AssetAlertError struct {
	PluginID      string
	PluginName    string
	PluginVersion string
	CapabilityID  string
	AssetPath     string
	ArtifactPath  string
	Reason        string
	Detail        string
	Err           error
}

var _ CapabilityAlerting = (*AlertingService)(nil)

// NewAlertingService 构造告警服务（可注入空依赖，内部自动降级）。
func NewAlertingService(opts AlertingOptions) *AlertingService {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	return &AlertingService{
		audit:    opts.Audit,
		notifier: opts.Notifier,
		logger:   logger,
		now:      clock,
	}
}

// NotifyAssetIssue 发送资产缺失/异常告警，并且写入审计事件。
func (s *AlertingService) NotifyAssetIssue(ctx context.Context, input AssetAlertInput) {
	if s == nil {
		return
	}
	meta := map[string]any{
		"plugin_id":      input.PluginID,
		"plugin_name":    input.PluginName,
		"plugin_version": input.PluginVersion,
		"capability_id":  input.CapabilityID,
		"asset_path":     input.AssetPath,
		"artifact_path":  input.ArtifactPath,
		"reason":         input.Reason,
		"detail":         input.Detail,
	}

	if s.audit != nil {
		payload, _ := json.Marshal(meta)
		_ = s.audit.Emit(ctx, &auditmodel.AuditEvent{
			TenantUUID:   "",
			Source:       "capability_registry.sync_worker",
			Operation:    "capability_registry.asset_validation",
			ResourceType: "capability",
			ResourceID:   strings.TrimSpace(input.CapabilityID),
			ResourceName: input.PluginName,
			Outcome:      "FAILED",
			Severity:     "ERROR",
			Meta:         datatypes.JSON(payload),
			OccurredAt:   s.now(),
		})
	}

	if s.notifier != nil {
		content := fmt.Sprintf("插件 %s (%s) 的能力 %s 在处理资产 %s 时失败：%s", input.PluginName, input.PluginVersion, input.CapabilityID, input.AssetPath, input.Detail)
		msg := imnotify.Message{
			Title:    fmt.Sprintf("[Capability Sync] 资产告警 - %s", input.CapabilityID),
			Content:  content,
			Severity: "critical",
			Metadata: meta,
		}
		if err := s.notifier.Send(ctx, msg); err != nil && s.logger != nil {
			s.logger.WarnF(ctx, "[capability_registry] send asset alert failed: %v", err)
		}
	}
}

// Error 实现 error 接口。
func (e *AssetAlertError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("asset %s validation failed: %v", e.AssetPath, e.Err)
}

// Unwrap 方便 errors.As 判断。
func (e *AssetAlertError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ToInput 转换为告警输入。
func (e *AssetAlertError) ToInput() AssetAlertInput {
	if e == nil {
		return AssetAlertInput{}
	}
	return AssetAlertInput{
		PluginID:      e.PluginID,
		PluginName:    e.PluginName,
		PluginVersion: e.PluginVersion,
		CapabilityID:  e.CapabilityID,
		AssetPath:     e.AssetPath,
		ArtifactPath:  e.ArtifactPath,
		Reason:        e.Reason,
		Detail:        e.Detail,
	}
}
