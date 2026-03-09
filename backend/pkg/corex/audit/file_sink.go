package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	lumberjack "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
)

// FileSinkOptions controls audit file output and rotation.
type FileSinkOptions struct {
	Enable      bool   `yaml:"enable"`
	Dir         string `yaml:"dir"`
	FilePrefix  string `yaml:"file_prefix"`
	MaxSize     int    `yaml:"max_size"`
	MaxBackups  int    `yaml:"max_backups"`
	MaxAge      int    `yaml:"max_age"`
	Compress    bool   `yaml:"compress"`
	UseUTC      bool   `yaml:"use_utc"`
	IncludeMeta bool   `yaml:"include_meta"`
}

// FileSink writes one JSON line per audit event and rotates by date + size.
type FileSink struct {
	opt    FileSinkOptions
	mu     sync.Mutex
	dayKey string
	logger *lumberjack.Logger
}

func NewFileSink(opt FileSinkOptions) (*FileSink, error) {
	if !opt.Enable {
		return nil, nil
	}
	if opt.Dir == "" {
		opt.Dir = "logs/audit"
	}
	if opt.FilePrefix == "" {
		opt.FilePrefix = "audit_event"
	}
	if opt.MaxSize <= 0 {
		opt.MaxSize = 100
	}
	if opt.MaxBackups <= 0 {
		opt.MaxBackups = 30
	}
	if opt.MaxAge <= 0 {
		opt.MaxAge = 30
	}
	if err := os.MkdirAll(opt.Dir, 0o755); err != nil {
		return nil, err
	}
	return &FileSink{opt: opt}, nil
}

func (s *FileSink) Emit(_ context.Context, evt *dbm.AuditEvent) error {
	if s == nil || evt == nil || !s.opt.Enable {
		return nil
	}
	now := time.Now()
	if s.opt.UseUTC {
		now = now.UTC()
	}
	dayKey := now.Format("2006-01-02")

	payload := map[string]any{
		"occurred_at":     evt.OccurredAt,
		"source":          evt.Source,
		"operation":       evt.Operation,
		"outcome":         evt.Outcome,
		"severity":        evt.Severity,
		"resource_type":   evt.ResourceType,
		"resource_id":     evt.ResourceID,
		"resource_name":   evt.ResourceName,
		"correlation_id":  evt.CorrelationID,
		"tenant_uuid":     evt.TenantUUID,
		"actor_user_id":   evt.ActorUserID,
		"actor_user_name": evt.ActorUserName,
		"actor_display":   evt.ActorDisplay,
		"client_ip":       evt.ClientIP,
		"client_ua":       evt.ClientUA,
		"created_at":      evt.CreatedAt,
	}
	if s.opt.IncludeMeta {
		var meta any
		if len(evt.Meta) > 0 && json.Unmarshal(evt.Meta, &meta) == nil {
			payload["meta"] = meta
		} else if len(evt.Meta) > 0 {
			payload["meta_raw"] = string(evt.Meta)
		}
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureWriterLocked(dayKey); err != nil {
		return err
	}
	_, err = s.logger.Write(line)
	return err
}

func (s *FileSink) ensureWriterLocked(dayKey string) error {
	if s.logger != nil && s.dayKey == dayKey {
		return nil
	}
	if s.logger != nil {
		_ = s.logger.Close()
		s.logger = nil
	}
	filename := filepath.Join(s.opt.Dir, fmt.Sprintf("%s-%s.log", s.opt.FilePrefix, dayKey))
	s.logger = &lumberjack.Logger{
		Filename:   filename,
		MaxSize:    s.opt.MaxSize,
		MaxBackups: s.opt.MaxBackups,
		MaxAge:     s.opt.MaxAge,
		Compress:   s.opt.Compress,
	}
	s.dayKey = dayKey
	return nil
}
