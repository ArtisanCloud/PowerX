package monitorlogs

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"gorm.io/gorm"
)

type Service struct {
	driver    Driver
	providers map[Driver]Provider
	retention *RetentionService
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
	}
}

func (s *Service) GetConfig() (ConfigView, error) {
	p, err := s.currentProvider()
	if err != nil {
		return ConfigView{}, err
	}
	return p.Config(), nil
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
	if s == nil {
		return nil, fmt.Errorf("monitor logs service is nil")
	}
	p := s.providers[s.driver]
	if p == nil {
		return nil, fmt.Errorf("monitor logs provider unavailable for driver=%s", s.driver)
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
