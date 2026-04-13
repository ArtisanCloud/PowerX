package monitorlogs

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	eventscheduler "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/scheduler"
	settingsvc "github.com/ArtisanCloud/PowerX/internal/service/system"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	retentionServiceMu sync.RWMutex
	retentionService   *RetentionService
	cronFieldRegexp    = regexp.MustCompile(`\S+`)
)

const (
	retentionSettingKey   = "ops.monitor.logs.retention.policy"
	retentionSettingGroup = "monitor.logs"
	retentionSettingDesc  = "monitor logs retention policy"
)

type RetentionService struct {
	db          *gorm.DB
	cfg         logcfg.RetentionConfig
	cron        *eventscheduler.Service
	fileRet     *FileRetentionProvider
	dbRet       *DBRetentionProvider
	maxHistory  int
	sysSetting  *settingsvc.SettingService
	staticPaths []string
	updateCh    chan struct{}

	mu      sync.RWMutex
	runs    []RetentionRun
	nextRun *time.Time
}

func StartRetentionScheduler(ctx context.Context, cfg *config.Config, db *gorm.DB) {
	if cfg == nil {
		cfg = config.GetGlobalConfig()
	}
	if cfg == nil {
		return
	}
	svc := NewRetentionService(cfg, db)
	retentionServiceMu.Lock()
	retentionService = svc
	retentionServiceMu.Unlock()
	if svc == nil || !svc.Config().Enabled {
		return
	}
	go svc.loop(ctx)
}

func GetRetentionService() *RetentionService {
	retentionServiceMu.RLock()
	defer retentionServiceMu.RUnlock()
	return retentionService
}

func NewRetentionService(cfg *config.Config, db *gorm.DB) *RetentionService {
	if cfg == nil {
		return nil
	}
	rcfg := normalizeRetentionConfig(cfg.LogConfig.Retention)
	paths := append([]string{}, rcfg.FilePaths...)
	if cfg.LogConfig.File.Enable {
		paths = append(paths, cfg.LogConfig.File.InfoFilePath, cfg.LogConfig.File.ErrorFilePath)
	}
	if cfg.LogConfig.AgentDebug.Enable {
		paths = append(paths, cfg.LogConfig.AgentDebug.Dir)
	}
	if strings.TrimSpace(cfg.Audit.File.Dir) != "" {
		paths = append(paths, cfg.Audit.File.Dir)
	}
	svc := &RetentionService{
		db:          db,
		cfg:         rcfg,
		cron:        eventscheduler.NewService(),
		fileRet:     NewFileRetentionProvider(append([]string{}, paths...)),
		dbRet:       NewDBRetentionProvider(db, rcfg.BatchSize, rcfg.MaxDeleteRowsPerRun, append([]logcfg.RetentionDBTable{}, rcfg.DBTables...)),
		maxHistory:  50,
		sysSetting:  settingsvc.NewSettingService(db),
		staticPaths: append([]string{}, paths...),
		updateCh:    make(chan struct{}, 1),
		runs:        make([]RetentionRun, 0, 50),
	}
	svc.applyPersistedPolicy(context.Background())
	return svc
}

func (s *RetentionService) Config() logcfg.RetentionConfig {
	if s == nil {
		return logcfg.RetentionConfig{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *RetentionService) Policy() RetentionPolicy {
	cfg := s.Config()
	return toRetentionPolicy(cfg)
}

func (s *RetentionService) UpdatePolicy(ctx context.Context, policy RetentionPolicy, operator string) (RetentionPolicy, error) {
	if s == nil {
		return RetentionPolicy{}, fmt.Errorf("retention service unavailable")
	}
	cfg := normalizeRetentionConfig(fromRetentionPolicy(policy))
	if err := validateRetentionConfig(cfg); err != nil {
		return RetentionPolicy{}, err
	}
	if s.sysSetting != nil {
		if err := s.sysSetting.UpsertSystemJSON(ctx, retentionSettingKey, toRetentionPolicy(cfg), retentionSettingGroup, retentionSettingDesc, true); err != nil {
			return RetentionPolicy{}, err
		}
	}
	s.mu.Lock()
	s.cfg = cfg
	s.fileRet = NewFileRetentionProvider(append(append([]string{}, s.staticPaths...), cfg.FilePaths...))
	s.dbRet = NewDBRetentionProvider(s.db, cfg.BatchSize, cfg.MaxDeleteRowsPerRun, cfg.DBTables)
	s.mu.Unlock()
	s.notifyScheduleReload()
	logger.Info(ctx, "log.retention.policy.updated",
		zap.String("operator", strings.TrimSpace(operator)),
		zap.Bool("enabled", cfg.Enabled),
		zap.String("cron", cfg.Cron),
		zap.String("timezone", cfg.Timezone),
		zap.Int("default_retention_days", cfg.DefaultRetentionDays),
	)
	return toRetentionPolicy(cfg), nil
}

func (s *RetentionService) ListRuns(limit int) RetentionRunList {
	if s == nil {
		return RetentionRunList{}
	}
	if limit <= 0 {
		limit = 20
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := limit
	if n > len(s.runs) {
		n = len(s.runs)
	}
	items := make([]RetentionRun, n)
	copy(items, s.runs[:n])
	return RetentionRunList{
		Items:    items,
		NextRun:  s.nextRun,
		Enabled:  s.cfg.Enabled,
		Cron:     s.cfg.Cron,
		Timezone: s.cfg.Timezone,
	}
}

func (s *RetentionService) TriggerNow(ctx context.Context, operator string) RetentionRun {
	if s == nil {
		return RetentionRun{
			RunID:        fmt.Sprintf("ret-%d", time.Now().UnixNano()),
			TriggeredBy:  operator,
			StartedAt:    time.Now(),
			EndedAt:      time.Now(),
			Status:       "failed",
			ErrorSummary: "retention service unavailable",
		}
	}
	return s.execute(ctx, strings.TrimSpace(operator))
}

func (s *RetentionService) loop(ctx context.Context) {
	var lastRun *time.Time
	for {
		cfg := s.Config()
		if !cfg.Enabled {
			s.setNextRun(nil)
			select {
			case <-ctx.Done():
				return
			case <-s.updateCh:
				continue
			case <-time.After(30 * time.Second):
				continue
			}
		}
		next, err := s.computeNext(lastRun)
		if err != nil {
			logger.Warn(ctx, "log.retention.schedule.compute_failed", zap.String("error", err.Error()))
			select {
			case <-ctx.Done():
				return
			case <-s.updateCh:
				continue
			case <-time.After(time.Minute):
				continue
			}
		}
		s.setNextRun(next)
		wait := time.Until(*next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.updateCh:
			timer.Stop()
			continue
		case <-timer.C:
		}
		run := s.execute(ctx, "system.scheduler")
		last := run.EndedAt.UTC()
		lastRun = &last
	}
}

func (s *RetentionService) computeNext(lastRun *time.Time) (*time.Time, error) {
	cfg := s.Config()
	res, err := s.cron.ComputeNextRun(eventscheduler.ComputeNextRunInput{
		CronExpr:  cfg.Cron,
		Timezone:  cfg.Timezone,
		Now:       time.Now(),
		LastRunAt: lastRun,
	})
	if err != nil {
		return nil, err
	}
	if res == nil || res.NextRunAt == nil {
		return nil, fmt.Errorf("next run unavailable")
	}
	return res.NextRunAt, nil
}

func (s *RetentionService) execute(ctx context.Context, operator string) RetentionRun {
	if strings.TrimSpace(operator) == "" {
		operator = "system"
	}
	s.mu.RLock()
	cfg := s.cfg
	fileRet := s.fileRet
	dbRet := s.dbRet
	s.mu.RUnlock()
	start := time.Now()
	cutoff := start.Add(-time.Duration(cfg.DefaultRetentionDays) * 24 * time.Hour)
	var deletedFiles int64
	var deletedRows int64
	sources := make([]string, 0, 8)
	errs := make([]string, 0, 8)

	if fileRet != nil {
		files, fileErrs := fileRet.Cleanup(ctx, cutoff)
		deletedFiles += files
		if files > 0 {
			sources = append(sources, "file")
		}
		if len(fileErrs) > 0 {
			errs = append(errs, fileErrs...)
		}
	}
	if dbRet != nil {
		rows, dbErrs := dbRet.Cleanup(ctx, cfg.DefaultRetentionDays)
		deletedRows += rows
		if rows > 0 {
			sources = append(sources, "db")
		}
		if len(dbErrs) > 0 {
			errs = append(errs, dbErrs...)
		}
	}

	run := RetentionRun{
		RunID:        fmt.Sprintf("ret-%d", start.UnixNano()),
		TriggeredBy:  operator,
		StartedAt:    start,
		EndedAt:      time.Now(),
		DeletedFiles: deletedFiles,
		DeletedRows:  deletedRows,
		Sources:      uniqueStrings(sources),
	}
	run.DurationMS = run.EndedAt.Sub(run.StartedAt).Milliseconds()
	if len(errs) > 0 {
		run.Status = "failed"
		run.ErrorSummary = strings.Join(uniqueStrings(errs), " | ")
	} else {
		run.Status = "success"
	}

	logger.Info(ctx, "log.retention.execute",
		zap.String("run_id", run.RunID),
		zap.String("operator", run.TriggeredBy),
		zap.String("status", run.Status),
		zap.Int64("deleted_files", run.DeletedFiles),
		zap.Int64("deleted_rows", run.DeletedRows),
		zap.Strings("sources", run.Sources),
		zap.String("error", run.ErrorSummary),
		zap.Int64("duration_ms", run.DurationMS),
	)
	s.appendRun(run)
	return run
}

func (s *RetentionService) appendRun(run RetentionRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append([]RetentionRun{run}, s.runs...)
	if len(s.runs) > s.maxHistory {
		s.runs = s.runs[:s.maxHistory]
	}
}

func (s *RetentionService) setNextRun(t *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t == nil {
		s.nextRun = nil
		return
	}
	cp := *t
	s.nextRun = &cp
}

func (s *RetentionService) notifyScheduleReload() {
	if s == nil {
		return
	}
	select {
	case s.updateCh <- struct{}{}:
	default:
	}
}

func (s *RetentionService) applyPersistedPolicy(ctx context.Context) {
	if s == nil || s.sysSetting == nil || s.db == nil {
		return
	}
	var policy RetentionPolicy
	ok, err := s.sysSetting.GetSystemJSON(ctx, retentionSettingKey, &policy)
	if err != nil || !ok {
		return
	}
	cfg := normalizeRetentionConfig(fromRetentionPolicy(policy))
	if err := validateRetentionConfig(cfg); err != nil {
		return
	}
	s.mu.Lock()
	s.cfg = cfg
	s.fileRet = NewFileRetentionProvider(append(append([]string{}, s.staticPaths...), cfg.FilePaths...))
	s.dbRet = NewDBRetentionProvider(s.db, cfg.BatchSize, cfg.MaxDeleteRowsPerRun, cfg.DBTables)
	s.mu.Unlock()
}

func normalizeRetentionConfig(cfg logcfg.RetentionConfig) logcfg.RetentionConfig {
	if strings.TrimSpace(cfg.Cron) == "" {
		cfg.Cron = "10 3 * * *"
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		cfg.Timezone = "Asia/Shanghai"
	}
	if cfg.DefaultRetentionDays <= 0 {
		cfg.DefaultRetentionDays = 30
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 5000
	}
	if cfg.MaxDeleteRowsPerRun <= 0 {
		cfg.MaxDeleteRowsPerRun = 200000
	}
	cfg.FilePaths = uniqueStrings(cfg.FilePaths)
	if len(cfg.DBTables) == 0 {
		cfg.DBTables = []logcfg.RetentionDBTable{
			{Name: "audit_event", TimeColumn: "occurred_at", RetentionDays: cfg.DefaultRetentionDays},
			{Name: "admin_console_audit_events", TimeColumn: "created_at", RetentionDays: cfg.DefaultRetentionDays},
			{Name: "runtime_audit_events", TimeColumn: "created_at", RetentionDays: 14},
		}
	}
	return cfg
}

func validateRetentionConfig(cfg logcfg.RetentionConfig) error {
	if strings.TrimSpace(cfg.Cron) == "" {
		return fmt.Errorf("cron is required")
	}
	cronFields := cronFieldRegexp.FindAllString(strings.TrimSpace(cfg.Cron), -1)
	if len(cronFields) < 5 || len(cronFields) > 6 {
		return fmt.Errorf("invalid cron expression")
	}
	if strings.TrimSpace(cfg.Timezone) == "" {
		return fmt.Errorf("timezone is required")
	}
	s := eventscheduler.NewService()
	if _, err := s.ComputeNextRun(eventscheduler.ComputeNextRunInput{
		CronExpr: cfg.Cron,
		Timezone: cfg.Timezone,
		Now:      time.Now(),
	}); err != nil {
		return fmt.Errorf("invalid retention schedule: %w", err)
	}
	if cfg.DefaultRetentionDays <= 0 {
		return fmt.Errorf("default_retention_days must be > 0")
	}
	if cfg.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be > 0")
	}
	if cfg.MaxDeleteRowsPerRun <= 0 {
		return fmt.Errorf("max_delete_rows_per_run must be > 0")
	}
	for i := range cfg.DBTables {
		if strings.TrimSpace(cfg.DBTables[i].Name) == "" || strings.TrimSpace(cfg.DBTables[i].TimeColumn) == "" {
			return fmt.Errorf("db_tables[%d] is invalid", i)
		}
	}
	return nil
}

func toRetentionPolicy(cfg logcfg.RetentionConfig) RetentionPolicy {
	out := RetentionPolicy{
		Enabled:              cfg.Enabled,
		Cron:                 strings.TrimSpace(cfg.Cron),
		Timezone:             strings.TrimSpace(cfg.Timezone),
		DefaultRetentionDays: cfg.DefaultRetentionDays,
		FilePaths:            append([]string{}, cfg.FilePaths...),
		BatchSize:            cfg.BatchSize,
		MaxDeleteRowsPerRun:  cfg.MaxDeleteRowsPerRun,
		DBTables:             make([]RetentionDBTableView, 0, len(cfg.DBTables)),
	}
	for i := range cfg.DBTables {
		out.DBTables = append(out.DBTables, RetentionDBTableView{
			Name:          strings.TrimSpace(cfg.DBTables[i].Name),
			TimeColumn:    strings.TrimSpace(cfg.DBTables[i].TimeColumn),
			RetentionDays: cfg.DBTables[i].RetentionDays,
		})
	}
	return out
}

func fromRetentionPolicy(policy RetentionPolicy) logcfg.RetentionConfig {
	out := logcfg.RetentionConfig{
		Enabled:              policy.Enabled,
		Cron:                 strings.TrimSpace(policy.Cron),
		Timezone:             strings.TrimSpace(policy.Timezone),
		DefaultRetentionDays: policy.DefaultRetentionDays,
		FilePaths:            append([]string{}, policy.FilePaths...),
		BatchSize:            policy.BatchSize,
		MaxDeleteRowsPerRun:  policy.MaxDeleteRowsPerRun,
		DBTables:             make([]logcfg.RetentionDBTable, 0, len(policy.DBTables)),
	}
	for i := range policy.DBTables {
		out.DBTables = append(out.DBTables, logcfg.RetentionDBTable{
			Name:          strings.TrimSpace(policy.DBTables[i].Name),
			TimeColumn:    strings.TrimSpace(policy.DBTables[i].TimeColumn),
			RetentionDays: policy.DBTables[i].RetentionDays,
		})
	}
	return out
}

func uniqueStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for i := range input {
		v := strings.TrimSpace(input[i])
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
