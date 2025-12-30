package tenant_release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

var (
	ErrPolicyNotFound = errors.New("release: policy not found")
	ErrInvalidInput   = errors.New("release: invalid input")
	ErrBatchNotFound  = errors.New("release: batch not found")
	ErrBatchPaused    = errors.New("release: batch paused")
)

type Options struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	MetricsWriter   *instrumentation.ReleaseMetricsWriter
	Clock           func() time.Time
}

type Service struct {
	db      *gorm.DB
	inst    *instrumentation.Instrumentation
	metrics *instrumentation.ReleaseMetricsWriter
	clock   func() time.Time
}

type BatchSpec struct {
	Name    string   `json:"name"`
	Tenants []string `json:"tenants"`
}

type UpsertPolicyInput struct {
	MatrixVersion string
	PilotTenants  []string
	Batches       []BatchSpec
	Guardrails    map[string]string
	ApprovedBy    string
	CreatedBy     string
}

type PublishInput struct {
	PolicyID    uint64
	VersionID   string
	RequestedBy string
}

type PromoteInput struct {
	PolicyID    uint64
	VersionID   string
	BatchToken  string
	Alerts      []string
	RequestedBy string
}

type RollbackInput struct {
	PolicyID    uint64
	VersionID   string
	Reason      string
	RequestedBy string
}

type PublishResult struct {
	ReleaseID  string
	VersionID  string
	BatchToken string
	BatchIndex int
	Tenants    []string
}

type PromoteResult struct {
	BatchToken     string
	BatchIndex     int
	Tenants        []string
	State          string
	TenantCoverage float64
}

type RollbackResult struct {
	Status string
}

func NewService(opts Options) *Service {
	if opts.DB == nil {
		panic("tenant release service requires db")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	return &Service{
		db:      opts.DB,
		inst:    opts.Instrumentation,
		metrics: opts.MetricsWriter,
		clock:   opts.Clock,
	}
}

func (s *Service) UpsertPolicy(ctx context.Context, in UpsertPolicyInput) (*models.TenantReleasePolicy, error) {
	if strings.TrimSpace(in.MatrixVersion) == "" || len(in.Batches) == 0 {
		return nil, ErrInvalidInput
	}
	payload := &models.TenantReleasePolicy{
		MatrixVersion: strings.TrimSpace(in.MatrixVersion),
		PilotTenants:  encodeJSON(in.PilotTenants),
		Batches:       encodeJSON(in.Batches),
		Guardrails:    encodeJSON(in.Guardrails),
		ApprovedBy:    strings.TrimSpace(in.ApprovedBy),
		CreatedBy:     strings.TrimSpace(in.CreatedBy),
		Status:        "active",
	}
	repoPolicy := repo.NewTenantReleasePolicyRepository(s.db)
	created, err := repoPolicy.Create(ctx, payload)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "knowledge.release.policy.upsert", map[string]any{
		"policy_id":      created.ID,
		"matrix_version": created.MatrixVersion,
	})
	return created, nil
}

func (s *Service) Publish(ctx context.Context, in PublishInput) (*PublishResult, error) {
	if in.PolicyID == 0 || strings.TrimSpace(in.VersionID) == "" {
		return nil, ErrInvalidInput
	}
	repoPolicy := repo.NewTenantReleasePolicyRepository(s.db)
	policy, err := repoPolicy.FindByID(ctx, in.PolicyID)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, ErrPolicyNotFound
	}
	specs, err := decodeBatchSpecs(policy.Batches)
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, ErrInvalidInput
	}
	batchRepo := repo.NewTenantReleaseBatchRepository(s.db)
	if err := s.db.WithContext(ctx).
		Where("policy_id = ? AND version_id = ?", in.PolicyID, strings.TrimSpace(in.VersionID)).
		Delete(&models.TenantReleaseBatch{}).Error; err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	var first *models.TenantReleaseBatch
	for idx, spec := range specs {
		batch := &models.TenantReleaseBatch{
			PolicyID:   in.PolicyID,
			VersionID:  strings.TrimSpace(in.VersionID),
			BatchIndex: idx,
			Tenants:    encodeJSON(spec.Tenants),
			State:      "pending",
		}
		batch.EnsureToken()
		if idx == 0 {
			batch.State = "promoted"
			batch.PromotedAt = &now
			first = batch
		}
		if _, err := batchRepo.Create(ctx, batch); err != nil {
			return nil, err
		}
		if idx == 0 {
			first = batch
		}
	}
	tenants := decodeStringSlice(first.Tenants)
	res := &PublishResult{
		ReleaseID:  fmt.Sprintf("policy-%d:%s", in.PolicyID, strings.TrimSpace(in.VersionID)),
		VersionID:  strings.TrimSpace(in.VersionID),
		BatchToken: first.BatchToken,
		BatchIndex: first.BatchIndex,
		Tenants:    tenants,
	}
	s.recordMetrics("promoted", 0, coverage(1, len(specs)), nil)
	s.audit(ctx, "knowledge.release.publish", map[string]any{
		"policy_id":    in.PolicyID,
		"version_id":   in.VersionID,
		"requested_by": in.RequestedBy,
		"batch_index":  first.BatchIndex,
	})
	return res, nil
}

func (s *Service) Promote(ctx context.Context, in PromoteInput) (*PromoteResult, error) {
	if in.PolicyID == 0 || strings.TrimSpace(in.VersionID) == "" || strings.TrimSpace(in.BatchToken) == "" {
		return nil, ErrInvalidInput
	}
	batchRepo := repo.NewTenantReleaseBatchRepository(s.db)
	current, err := batchRepo.FindByToken(ctx, strings.TrimSpace(in.BatchToken))
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrBatchNotFound
	}
	specsCount, err := s.countBatches(ctx, in.PolicyID, strings.TrimSpace(in.VersionID))
	if err != nil {
		return nil, err
	}
	if len(in.Alerts) > 0 {
		current.State = "paused"
		current.Alerts = encodeJSON(in.Alerts)
		if _, err := batchRepo.SaveState(ctx, current); err != nil {
			return nil, err
		}
		s.recordMetrics("paused", 0, coverage(current.BatchIndex+1, specsCount), in.Alerts)
		return &PromoteResult{
			BatchToken:     current.BatchToken,
			BatchIndex:     current.BatchIndex,
			Tenants:        decodeStringSlice(current.Tenants),
			State:          current.State,
			TenantCoverage: coverage(current.BatchIndex+1, specsCount),
		}, ErrBatchPaused
	}
	now := s.clock().UTC()
	current.State = "completed"
	current.CompletedAt = &now
	if _, err := batchRepo.SaveState(ctx, current); err != nil {
		return nil, err
	}
	next, err := s.nextBatch(ctx, in.PolicyID, strings.TrimSpace(in.VersionID), current.BatchIndex)
	if err != nil {
		return nil, err
	}
	if next == nil {
		s.recordMetrics("completed", 0, 1, nil)
		s.audit(ctx, "knowledge.release.completed", map[string]any{
			"policy_id":  in.PolicyID,
			"version_id": in.VersionID,
		})
		return &PromoteResult{
			BatchToken:     "",
			BatchIndex:     current.BatchIndex,
			Tenants:        nil,
			State:          "completed",
			TenantCoverage: 1,
		}, nil
	}
	next.State = "promoted"
	tNow := s.clock().UTC()
	next.PromotedAt = &tNow
	if _, err := batchRepo.SaveState(ctx, next); err != nil {
		return nil, err
	}
	cov := coverage(next.BatchIndex+1, specsCount)
	s.recordMetrics("promoted", 0, cov, nil)
	s.audit(ctx, "knowledge.release.promote", map[string]any{
		"policy_id":   in.PolicyID,
		"version_id":  in.VersionID,
		"batch_index": next.BatchIndex,
	})
	return &PromoteResult{
		BatchToken:     next.BatchToken,
		BatchIndex:     next.BatchIndex,
		Tenants:        decodeStringSlice(next.Tenants),
		State:          next.State,
		TenantCoverage: cov,
	}, nil
}

func (s *Service) Rollback(ctx context.Context, in RollbackInput) (*RollbackResult, error) {
	if in.PolicyID == 0 || strings.TrimSpace(in.VersionID) == "" {
		return nil, ErrInvalidInput
	}
	batches, err := s.batches(ctx, in.PolicyID, strings.TrimSpace(in.VersionID))
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	var count int
	for _, batch := range batches {
		batch.State = "rolled_back"
		batch.RolledBackAt = &now
		if _, err := repo.NewTenantReleaseBatchRepository(s.db).SaveState(ctx, batch); err != nil {
			return nil, err
		}
		count++
	}
	alerts := []string{fmt.Sprintf("rollback: %s", strings.TrimSpace(in.Reason))}
	s.recordMetrics("rolled_back", count, 0, alerts)
	s.audit(ctx, "knowledge.release.rollback", map[string]any{
		"policy_id":    in.PolicyID,
		"version_id":   in.VersionID,
		"reason":       in.Reason,
		"requested_by": in.RequestedBy,
	})
	return &RollbackResult{Status: "rolled_back"}, nil
}

func (s *Service) batches(ctx context.Context, policyID uint64, versionID string) ([]*models.TenantReleaseBatch, error) {
	return repo.NewTenantReleaseBatchRepository(s.db).ListByPolicyAndVersion(ctx, policyID, versionID)
}

func (s *Service) nextBatch(ctx context.Context, policyID uint64, versionID string, currentIndex int) (*models.TenantReleaseBatch, error) {
	batches, err := s.batches(ctx, policyID, versionID)
	if err != nil {
		return nil, err
	}
	for _, batch := range batches {
		if batch.BatchIndex > currentIndex {
			return batch, nil
		}
	}
	return nil, nil
}

func (s *Service) countBatches(ctx context.Context, policyID uint64, versionID string) (int, error) {
	batches, err := s.batches(ctx, policyID, versionID)
	if err != nil {
		return 0, err
	}
	return len(batches), nil
}

func (s *Service) recordMetrics(state string, rollbackCount int, coverage float64, alerts []string) {
	if s.metrics == nil {
		return
	}
	snapshot := instrumentation.ReleaseMetricsSnapshot{
		GrayState:      state,
		RollbackCount:  rollbackCount,
		TenantCoverage: coverage,
		Alerts:         alerts,
	}
	_ = s.metrics.Store(snapshot)
}

func (s *Service) audit(ctx context.Context, action string, payload map[string]any) {
	if s.inst == nil {
		return
	}
	raw, _ := json.Marshal(payload)
	resourceID := ""
	if v, ok := payload["policy_id"]; ok {
		resourceID = fmt.Sprint(v)
	}
	s.inst.Audit(ctx, &dbm.AuditEvent{
		OccurredAt:   s.clock(),
		TenantUUID:   strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		Source:       "knowledge.release",
		Operation:    action,
		ResourceType: "knowledge_release",
		ResourceID:   resourceID,
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         raw,
	})
}

func encodeJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("null"))
	}
	data, _ := json.Marshal(v)
	return datatypes.JSON(data)
}

func decodeBatchSpecs(data datatypes.JSON) ([]BatchSpec, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var specs []BatchSpec
	if err := json.Unmarshal(data, &specs); err != nil {
		return nil, err
	}
	return specs, nil
}

func decodeStringSlice(data datatypes.JSON) []string {
	if len(data) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil
	}
	return values
}

func coverage(currentBatchCount, total int) float64 {
	if total <= 0 {
		return 0
	}
	ratio := float64(currentBatchCount) / float64(total)
	if ratio > 1 {
		return 1
	}
	if ratio < 0 {
		return 0
	}
	return ratio
}
