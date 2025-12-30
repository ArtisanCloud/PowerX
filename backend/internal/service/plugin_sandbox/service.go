package plugin_sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_sandbox/instrumentation"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_sandbox"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_sandbox"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Options configure sandbox service behaviour.
type Options struct {
	Suite       *Suite
	Instruments *instrumentation.Instruments
	Now         func() time.Time
	Component   string
}

// Service orchestrates sandbox dataset and validation runs.
type Service struct {
	repo        *repo.RunRepository
	now         func() time.Time
	suite       *Suite
	instruments *instrumentation.Instruments
	component   string
}

// DeployRequest initializes a sandbox run.
type DeployRequest struct {
	TenantUUID string `json:"tenant_uuid"`
	PluginID   string `json:"pluginId"`
	Dataset    string `json:"dataset"` // dataset ID
}

// DatasetRequest attaches dataset metadata to a run.
type DatasetRequest struct {
	RunID      uuid.UUID `json:"runId"`
	DatasetID  string    `json:"datasetId"`
	Version    string    `json:"datasetVersion"`
	TenantUUID string    `json:"-"`
}

// TestRequest finalizes the sandbox execution.
type TestRequest struct {
	RunID      uuid.UUID      `json:"runId"`
	Outcome    string         `json:"outcome"`
	Metrics    map[string]any `json:"metrics"`
	Report     string         `json:"reportUri"`
	Warnings   []string       `json:"warnings"`
	TenantUUID string         `json:"-"`
}

// NewService constructs the sandbox service.
func NewService(repo *repo.RunRepository, opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Instruments == nil {
		opts.Instruments = instrumentation.NewInstruments(opts.Component)
	}
	return &Service{
		repo:        repo,
		now:         opts.Now,
		suite:       opts.Suite,
		instruments: opts.Instruments,
		component:   opts.Component,
	}
}

// Deploy registers a new sandbox deployment run.
func (s *Service) Deploy(ctx context.Context, req DeployRequest) (*model.SandboxValidationRun, error) {
	if s.repo == nil {
		return nil, errors.New("sandbox repository unavailable")
	}
	tenantUUID := strings.TrimSpace(req.TenantUUID)
	if tenantUUID == "" {
		return nil, errors.New("tenant_uuid is required")
	}
	if strings.TrimSpace(req.PluginID) == "" {
		return nil, errors.New("pluginId is required")
	}
	datasetID := strings.TrimSpace(req.Dataset)
	if datasetID == "" && s.suite != nil && len(s.suite.Datasets) > 0 {
		datasetID = s.suite.Datasets[0].ID
	}
	if datasetID == "" {
		return nil, errors.New("dataset is required")
	}
	spec, ok := s.suite.Lookup(datasetID)
	if s.suite != nil && !ok {
		return nil, fmt.Errorf("dataset %s is not registered", datasetID)
	}
	start := s.now()
	run := &model.SandboxValidationRun{
		TenantUUID:    tenantUUID,
		PluginID:      strings.TrimSpace(req.PluginID),
		Status:        "deploying",
		Dataset:       datasetID,
		SuiteID:       spec.ID,
		StartedAt:     start,
		CorrelationID: uuid.New(),
	}
	created, err := s.repo.Create(ctx, run)
	if err != nil {
		return nil, err
	}
	if s.instruments != nil {
		s.instruments.RecordDeploy(ctx, s.now().Sub(start))
	}
	return created, nil
}

// LoadDataset annotates a run with dataset information.
func (s *Service) LoadDataset(ctx context.Context, req DatasetRequest) error {
	if s.repo == nil {
		return errors.New("sandbox repository unavailable")
	}
	if req.RunID == uuid.Nil {
		return errors.New("runId is required")
	}
	tenantUUID := strings.TrimSpace(req.TenantUUID)
	if tenantUUID == "" {
		return errors.New("tenant_uuid is required")
	}
	run, err := s.repo.GetForTenant(ctx, req.RunID, tenantUUID)
	if err != nil {
		return err
	}
	if req.DatasetID != "" && !strings.EqualFold(run.Dataset, req.DatasetID) {
		return fmt.Errorf("dataset mismatch: run=%s request=%s", run.Dataset, req.DatasetID)
	}
	values := map[string]any{
		"dataset_version": strings.TrimSpace(req.Version),
		"status":          "dataset_loaded",
	}
	return s.repo.UpdateFieldsForTenant(ctx, req.RunID, tenantUUID, values)
}

// RunTests finalizes the sandbox execution.
func (s *Service) RunTests(ctx context.Context, req TestRequest) (*model.SandboxValidationRun, error) {
	if s.repo == nil {
		return nil, errors.New("sandbox repository unavailable")
	}
	if req.RunID == uuid.Nil {
		return nil, errors.New("runId is required")
	}
	tenantUUID := strings.TrimSpace(req.TenantUUID)
	if tenantUUID == "" {
		return nil, errors.New("tenant_uuid is required")
	}
	now := s.now()
	summary := map[string]any{
		"metrics":  req.Metrics,
		"warnings": req.Warnings,
	}
	update := map[string]any{
		"status":       strings.TrimSpace(req.Outcome),
		"summary":      marshalJSON(summary),
		"warnings":     marshalStringSlice(req.Warnings),
		"report_uri":   strings.TrimSpace(req.Report),
		"completed_at": &now,
	}
	if err := s.repo.UpdateFieldsForTenant(ctx, req.RunID, tenantUUID, update); err != nil {
		return nil, err
	}
	if s.instruments != nil {
		s.instruments.RecordTest(ctx, strings.TrimSpace(req.Outcome))
	}
	return s.repo.GetForTenant(ctx, req.RunID, tenantUUID)
}

// Get fetches a sandbox run.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*model.SandboxValidationRun, error) {
	if s.repo == nil {
		return nil, errors.New("sandbox repository unavailable")
	}
	return s.repo.Get(ctx, id)
}

func marshalJSON(v map[string]any) datatypes.JSON {
	if len(v) == 0 {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}

func marshalStringSlice(items []string) datatypes.JSON {
	if len(items) == 0 {
		return datatypes.JSON("[]")
	}
	data, err := json.Marshal(items)
	if err != nil {
		return datatypes.JSON("[]")
	}
	return datatypes.JSON(data)
}
