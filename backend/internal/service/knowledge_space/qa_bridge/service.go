package qa_bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/compliance"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/context_snapshot"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/toolchain"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
)

// Errors returned by the QA bridge service.
var (
	ErrInvalidInput  = errors.New("qa_bridge: invalid input")
	ErrSpacesMissing = errors.New("qa_bridge: no knowledge spaces available")
)

// Options aggregates dependencies for Service.
type Options struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	VectorStore     vectorstore.Store
	SnapshotStore   *context_snapshot.Store
	ToolRegistry    *toolchain.Registry
	Guard           *compliance.Guard
	Clock           func() time.Time
	ReportPath      string
}

// Service exposes orchestration-facing helpers for QA flows.
type Service struct {
	db          *gorm.DB
	inst        *instrumentation.Instrumentation
	vectors     vectorstore.Store
	snapshots   *context_snapshot.Store
	tools       *toolchain.Registry
	guard       *compliance.Guard
	clock       func() time.Time
	coverageMin float64
	reportPath  string
}

// NewService constructs a QA bridge service.
func NewService(opts Options) *Service {
	if opts.DB == nil {
		panic("qa_bridge.Service requires DB")
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.SnapshotStore == nil {
		opts.SnapshotStore = context_snapshot.NewStore()
	}
	if opts.ToolRegistry == nil {
		opts.ToolRegistry = toolchain.NewRegistry()
	}
	if opts.Guard == nil {
		opts.Guard = compliance.NewGuard()
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if strings.TrimSpace(opts.ReportPath) == "" {
		opts.ReportPath = filepath.Join("reports", "_state", "qa-reasoning.json")
	}
	return &Service{
		db:          opts.DB,
		inst:        opts.Instrumentation,
		vectors:     opts.VectorStore,
		snapshots:   opts.SnapshotStore,
		tools:       opts.ToolRegistry,
		guard:       opts.Guard,
		clock:       opts.Clock,
		coverageMin: 0.65,
		reportPath:  opts.ReportPath,
	}
}

// PlanInput describes the payload required to generate a retrieval plan.
type PlanInput struct {
	TenantID        uuid.UUID
	Intent          string
	DomainTags      []string
	SessionID       string
	LatencyBudgetMs int
}

// CandidateSpace reflects a potential space for QA orchestration.
type CandidateSpace struct {
	SpaceID          uuid.UUID
	SpaceName        string
	Strategy         string
	CitationCoverage float64
	DegradeReason    string
}

// PlanOutput is the response envelope for Plan.
type PlanOutput struct {
	TenantID        uuid.UUID
	Intent          string
	DomainTags      []string
	CandidateSpaces []CandidateSpace
	Toolings        []toolchain.Metadata
	TraceID         string
	RecordedAt      time.Time
	DegradeCount    int
	SessionID       string
	LatencyBudgetMs int
	Metadata        map[string]any
}

// MemoryInput captures snapshot requests.
type MemoryInput struct {
	TenantID  uuid.UUID
	SessionID string
	Updates   []context_snapshot.Citation
}

// MemoryOutput describes persisted citations.
type MemoryOutput struct {
	TenantID  uuid.UUID
	SessionID string
	Citations []context_snapshot.Citation
}

// Plan builds a cross-space retrieval plan for QA orchestrators.
func (s *Service) Plan(ctx context.Context, in PlanInput) (*PlanOutput, error) {
	if in.TenantID == uuid.Nil || strings.TrimSpace(in.Intent) == "" {
		return nil, ErrInvalidInput
	}
	var spaces []models.KnowledgeSpace
	if err := s.db.WithContext(ctx).
		Where("tenant_id = ?", in.TenantID).
		Order("created_at ASC").
		Find(&spaces).Error; err != nil {
		return nil, err
	}
	if len(spaces) == 0 {
		return nil, ErrSpacesMissing
	}

	candidates := make([]CandidateSpace, 0, len(spaces))
	degradeCount := 0
	toolings := make([]toolchain.Metadata, 0, len(spaces)*2)

	for _, space := range spaces {
		candidate := CandidateSpace{
			SpaceID:   space.UUID,
			SpaceName: space.SpaceName,
			Strategy:  "hybrid",
		}
		reason := s.guard.Evaluate(in.TenantID, &space)
		if reason != "" {
			candidate.DegradeReason = reason
			degradeCount++
			candidate.CitationCoverage = s.coverageMin
		} else {
			candidate.CitationCoverage = s.queryCoverage(ctx, space.UUID)
		}
		candidates = append(candidates, candidate)
		toolings = append(toolings, s.tools.Resolve(&space)...)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].SpaceName < candidates[j].SpaceName
	})

	traceID := uuid.NewString()

	output := &PlanOutput{
		TenantID:        in.TenantID,
		Intent:          in.Intent,
		DomainTags:      uniqueStrings(in.DomainTags),
		CandidateSpaces: candidates,
		Toolings:        toolings,
		TraceID:         traceID,
		RecordedAt:      s.clock().UTC(),
		DegradeCount:    degradeCount,
		SessionID:       in.SessionID,
		LatencyBudgetMs: in.LatencyBudgetMs,
		Metadata: map[string]any{
			"candidate_total": len(candidates),
		},
	}
	s.writeReport(output)
	return output, nil
}

func (s *Service) queryCoverage(ctx context.Context, spaceID uuid.UUID) float64 {
	if s.vectors == nil {
		return 0.9
	}
	resp, err := s.vectors.Query(ctx, vectorstore.QueryRequest{
		SpaceID: spaceID,
		TopK:    5,
	})
	if err != nil {
		return 0.9
	}
	coverage := s.coverageMin + 0.05*float64(len(resp.Matches))
	if coverage > 0.99 {
		coverage = 0.99
	}
	if coverage < s.coverageMin {
		coverage = s.coverageMin
	}
	return coverage
}

// UpsertMemorySnapshot persists deltas and returns the current snapshot.
func (s *Service) UpsertMemorySnapshot(ctx context.Context, in MemoryInput) (*MemoryOutput, error) {
	if in.TenantID == uuid.Nil || strings.TrimSpace(in.SessionID) == "" {
		return nil, ErrInvalidInput
	}
	citations := s.snapshots.Upsert(ctx, in.TenantID, in.SessionID, in.Updates)
	return &MemoryOutput{
		TenantID:  in.TenantID,
		SessionID: in.SessionID,
		Citations: citations,
	}, nil
}

// Snapshot fetches an existing memory snapshot without mutating state.
func (s *Service) Snapshot(ctx context.Context, tenant uuid.UUID, sessionID string) *MemoryOutput {
	if tenant == uuid.Nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	citations := s.snapshots.Snapshot(ctx, tenant, sessionID)
	return &MemoryOutput{
		TenantID:  tenant,
		SessionID: sessionID,
		Citations: citations,
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		key := strings.TrimSpace(v)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func (s *Service) writeReport(out *PlanOutput) {
	if out == nil || strings.TrimSpace(s.reportPath) == "" {
		return
	}
	payload := map[string]any{
		"traceId":        out.TraceID,
		"tenantId":       out.TenantID.String(),
		"intent":         out.Intent,
		"candidateTotal": len(out.CandidateSpaces),
		"degradeCount":   out.DegradeCount,
		"timestamp":      out.RecordedAt.Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(s.reportPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(s.reportPath, data, 0o644)
}
