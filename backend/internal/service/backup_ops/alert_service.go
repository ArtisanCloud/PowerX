package backup_ops

import (
	"context"
	"fmt"
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

type AlertService struct {
	alertRepo  *repoops.BackupAlertRepository
	policyRepo *repoops.BackupPolicyRepository
	jobRepo    *repoops.BackupJobRepository
}

type ListAlertOptions struct {
	Level    string
	Acked    *bool
	Page     int
	PageSize int
}

type BackupOverview struct {
	PoliciesEnabled   int64      `json:"policies_enabled"`
	JobsRunning       int64      `json:"jobs_running"`
	JobsFailed24h     int64      `json:"jobs_failed_24h"`
	AlertsHighUnacked int64      `json:"alerts_high_unacked"`
	LastSuccessAt     *time.Time `json:"last_success_at,omitempty"`
}

func NewAlertService(db *gorm.DB) *AlertService {
	return &AlertService{
		alertRepo:  repoops.NewBackupAlertRepository(db),
		policyRepo: repoops.NewBackupPolicyRepository(db),
		jobRepo:    repoops.NewBackupJobRepository(db),
	}
}

func (s *AlertService) ListAlerts(ctx context.Context, opt ListAlertOptions) ([]modelops.BackupAlert, int64, error) {
	page := opt.Page
	if page <= 0 {
		page = 1
	}
	size := opt.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	offset := (page - 1) * size
	return s.alertRepo.List(ctx, opt.Level, opt.Acked, size, offset)
}

func (s *AlertService) AckAlert(ctx context.Context, alertID uint64, operator string) error {
	if alertID == 0 {
		return ErrInvalidBackupRequest
	}
	row, err := s.alertRepo.GetById(ctx, alertID, nil)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrBackupAlertNotFound
	}
	return s.alertRepo.Ack(ctx, alertID, normalizeOperator(operator))
}

func (s *AlertService) BuildOverview(ctx context.Context) (*BackupOverview, error) {
	enabled := true
	_, enabledCount, err := s.policyRepo.List(ctx, &enabled, 1, 0)
	if err != nil {
		return nil, err
	}
	runningCount, err := s.jobRepo.CountByStatus(ctx, modelops.BackupJobStatusRunning)
	if err != nil {
		return nil, err
	}
	failed24h, err := s.jobRepo.CountFailedSince(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		return nil, err
	}
	highUnacked, err := s.alertRepo.CountUnackedByLevel(ctx, modelops.BackupAlertLevelHigh)
	if err != nil {
		return nil, err
	}
	latestSuccess, err := s.jobRepo.GetLatestSuccess(ctx)
	if err != nil {
		return nil, err
	}
	var lastSuccessAt *time.Time
	if latestSuccess != nil && latestSuccess.EndedAt != nil {
		lastSuccessAt = latestSuccess.EndedAt
	}

	return &BackupOverview{
		PoliciesEnabled:   enabledCount,
		JobsRunning:       runningCount,
		JobsFailed24h:     failed24h,
		AlertsHighUnacked: highUnacked,
		LastSuccessAt:     lastSuccessAt,
	}, nil
}

// HandleJobCompletionAlert 实现“连续 2 次失败升级高优先级告警”规则。
func (s *AlertService) HandleJobCompletionAlert(ctx context.Context, job *modelops.BackupJob) error {
	if job == nil || job.PolicyID == 0 {
		return nil
	}
	if job.Status != modelops.BackupJobStatusFailed {
		return nil
	}
	consecutive, err := s.jobRepo.CountConsecutiveFailures(ctx, job.PolicyID, 5)
	if err != nil {
		return err
	}
	level := alertLevelForConsecutiveFailures(consecutive)

	msg := fmt.Sprintf("备份任务执行失败（策略 %d，连续失败 %d 次）", job.PolicyID, consecutive)
	if strings.TrimSpace(job.ErrorMessage) != "" {
		msg = fmt.Sprintf("%s：%s", msg, strings.TrimSpace(job.ErrorMessage))
	}
	alert := &modelops.BackupAlert{
		PolicyID:   job.PolicyID,
		JobID:      job.ID,
		Level:      level,
		AlertType:  "backup_job_failed",
		Message:    msg,
		Suggestion: "检查备份脚本、存储权限与目标实例连通性。",
		TraceID:    strings.TrimSpace(job.TraceID),
	}
	alert.Normalize()
	_, err = s.alertRepo.Create(ctx, alert)
	return err
}

func alertLevelForConsecutiveFailures(consecutive int) modelops.BackupAlertLevel {
	if consecutive >= 2 {
		return modelops.BackupAlertLevelHigh
	}
	return modelops.BackupAlertLevelMedium
}

func (s *AlertService) CreateCleanupFailureAlert(ctx context.Context, policyID uint64, traceID string, err error) error {
	if err == nil {
		return nil
	}
	alert := &modelops.BackupAlert{
		PolicyID:   policyID,
		JobID:      0,
		Level:      modelops.BackupAlertLevelMedium,
		AlertType:  "backup_cleanup_failed",
		Message:    fmt.Sprintf("备份清理失败：%s", strings.TrimSpace(err.Error())),
		Suggestion: "检查产物存储权限与清理脚本日志，确认过期备份可删除。",
		TraceID:    strings.TrimSpace(traceID),
	}
	alert.Normalize()
	_, createErr := s.alertRepo.Create(ctx, alert)
	return createErr
}
