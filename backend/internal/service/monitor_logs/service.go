package monitorlogs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/gorm"
)

type Service struct {
	driver    Driver
	providers map[Driver]Provider
	retention *RetentionService
	cfg       *config.Config
}

func NewService(db *gorm.DB) *Service {
	_ = db
	cfg := config.GetGlobalConfig()
	driver := detectDriver(cfg)

	providers := map[Driver]Provider{}
	providers[DriverLoki] = NewLokiProvider(cfg)
	providers[DriverFile] = NewFileProvider(cfg)
	providers[DriverStdio] = NewStdioProvider(cfg)

	return &Service{
		driver:    driver,
		providers: providers,
		retention: GetRetentionService(),
		cfg:       cfg,
	}
}

func (s *Service) GetConfig() (ConfigView, error) {
	p, err := s.currentProvider()
	if err != nil {
		return ConfigView{}, err
	}
	cfg := p.Config()
	cfg.OutputChannels = outputChannelsFromConfig(s.cfg)
	return cfg, nil
}

func (s *Service) Query(req QueryRequest) (QueryResult, error) {
	p, err := s.currentProvider()
	if err != nil {
		return QueryResult{}, err
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 200 {
		req.PageSize = 200
	}
	return p.Query(req)
}

func (s *Service) QueryByDriver(req QueryRequest, driver Driver) (QueryResult, error) {
	p, err := s.providerByDriver(driver)
	if err != nil {
		return QueryResult{}, err
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 50
	}
	if req.PageSize > 200 {
		req.PageSize = 200
	}
	return p.Query(req)
}

func (s *Service) RetentionRuns(limit int) RetentionRunList {
	if s == nil || s.retention == nil {
		return RetentionRunList{}
	}
	return s.retention.ListRuns(limit)
}

func (s *Service) TriggerRetentionNow(ctx context.Context, operator string) (RetentionRun, error) {
	if s == nil || s.retention == nil {
		return RetentionRun{}, fmt.Errorf("retention service unavailable")
	}
	return s.retention.TriggerNow(ctx, operator), nil
}

func (s *Service) TriggerRetentionDryRun(ctx context.Context, operator string, retentionDays *int) (RetentionRun, error) {
	if s == nil || s.retention == nil {
		return RetentionRun{}, fmt.Errorf("retention service unavailable")
	}
	return s.retention.TriggerDryRun(ctx, operator, retentionDays), nil
}

func (s *Service) ExportRetentionDryRun(ctx context.Context, operator string, retentionDays *int, cutoffAt *time.Time, format string) (RetentionExport, error) {
	if s == nil || s.retention == nil {
		return RetentionExport{}, fmt.Errorf("retention service unavailable")
	}
	return s.retention.ExportDryRun(ctx, operator, retentionDays, cutoffAt, format)
}

func (s *Service) GetRetentionPolicy() (RetentionPolicy, error) {
	if s == nil || s.retention == nil {
		return RetentionPolicy{}, fmt.Errorf("retention service unavailable")
	}
	return s.retention.Policy(), nil
}

func (s *Service) UpdateRetentionPolicy(ctx context.Context, policy RetentionPolicy, operator string) (RetentionPolicy, error) {
	if s == nil || s.retention == nil {
		return RetentionPolicy{}, fmt.Errorf("retention service unavailable")
	}
	return s.retention.UpdatePolicy(ctx, policy, operator)
}

func (s *Service) currentProvider() (Provider, error) {
	return s.providerByDriver(s.driver)
}

func (s *Service) providerByDriver(driver Driver) (Provider, error) {
	if s == nil {
		return nil, fmt.Errorf("monitor logs service is nil")
	}
	p := s.providers[driver]
	if p == nil {
		return nil, fmt.Errorf("monitor logs provider unavailable for driver=%s", driver)
	}
	return p, nil
}

func detectDriver(cfg *config.Config) Driver {
	if cfg != nil && cfg.LogConfig.Loki.Enable {
		return DriverLoki
	}
	if cfg != nil && cfg.LogConfig.File.Enable {
		return DriverFile
	}
	if strings.TrimSpace(strings.ToLower(getEnv("POWERX_LOG_DRIVER"))) == string(DriverFile) {
		return DriverFile
	}
	if strings.TrimSpace(strings.ToLower(getEnv("POWERX_LOG_DRIVER"))) == string(DriverLoki) {
		return DriverLoki
	}
	return DriverStdio
}

var getEnv = func(k string) string {
	return strings.TrimSpace(os.Getenv(k))
}

func outputChannelsFromConfig(cfg *config.Config) []Driver {
	channels := make([]Driver, 0, 3)
	if cfg == nil {
		return channels
	}
	if cfg.LogConfig.Console {
		channels = append(channels, DriverStdio)
	}
	if cfg.LogConfig.File.Enable {
		channels = append(channels, DriverFile)
	}
	if cfg.LogConfig.Loki.Enable {
		channels = append(channels, DriverLoki)
	}
	return channels
}
