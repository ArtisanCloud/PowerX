package knowledge_space

import (
	"context"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FusionService orchestrates multi-source strategy lifecycle and retrieval.
type FusionService struct {
	db          *gorm.DB
	inst        *instrumentation.Instrumentation
	vectorStore vectorstore.Store
	bus         event_bus.EventBus
	eventTopic  string
	clock       func() time.Time
}

// FusionServiceOptions configure the fusion service.
type FusionServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	VectorStore     vectorstore.Store
	EventBus        event_bus.EventBus
	EventTopic      string
	Clock           func() time.Time
}

// PublishStrategyInput captures strategy configuration.
type PublishStrategyInput struct {
	SpaceID         uuid.UUID
	Label           string
	BM25Weight      float64
	VectorWeight    float64
	GraphConstraint string
	RerankerModel   string
	ConflictPolicy  string
	RequestedBy     string
}

// RollbackStrategyInput describes rollback intent.
type RollbackStrategyInput struct {
	SpaceID     uuid.UUID
	StrategyID  uint64
	RequestedBy string
}

// FusionQueryInput defines retrieval parameters.
type FusionQueryInput struct {
	SpaceID   uuid.UUID
	Embedding []float32
	Filters   map[string]string
	TopK      int
	MinScore  float64
}

// FusionQueryMatch describes fused retrieval output.
type FusionQueryMatch struct {
	ChunkID  uuid.UUID
	Score    float64
	Source   string
	Metadata map[string]any
}

// FusionQueryResult aggregates vector/lexical results.
type FusionQueryResult struct {
	StrategyID uint64
	Matches    []FusionQueryMatch
}

// NewFusionService constructs an instance.
func NewFusionService(opts FusionServiceOptions) *FusionService {
	if opts.DB == nil {
		panic("fusion service requires db")
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	return &FusionService{
		db:          opts.DB,
		inst:        opts.Instrumentation,
		vectorStore: opts.VectorStore,
		bus:         opts.EventBus,
		eventTopic:  strings.TrimSpace(opts.EventTopic),
		clock:       opts.Clock,
	}
}

// PublishStrategy stores a new strategy version following conflict policy.
func (s *FusionService) PublishStrategy(ctx context.Context, in PublishStrategyInput) (*models.FusionStrategyVersion, error) {
	if in.SpaceID == uuid.Nil || strings.TrimSpace(in.Label) == "" {
		return nil, ErrInvalidInput
	}
	normalizedPolicy := normalizeConflictPolicy(in.ConflictPolicy)
	bm25, vector := normalizeWeights(in.BM25Weight, in.VectorWeight)

	var created *models.FusionStrategyVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces := repo.NewKnowledgeSpaceRepository(tx)
		strategies := repo.NewFusionStrategyRepository(tx)

		space, err := spaces.FindByUUID(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if space == nil || space.Status == models.KnowledgeSpaceStatusRetired {
			return ErrSpaceNotFound
		}

		active, err := strategies.FindActiveBySpace(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if active != nil && normalizedPolicy == "block" {
			return ErrFusionConflict
		}

		state := models.FusionDeploymentActive
		var publishedAt *time.Time
		if active != nil && normalizedPolicy == "queue" {
			state = models.FusionDeploymentDraft
		}
		now := s.clock()
		if state == models.FusionDeploymentActive {
			publishedAt = &now
		}

		strategy := &models.FusionStrategyVersion{
			SpaceUUID:       in.SpaceID,
			Label:           strings.TrimSpace(in.Label),
			BM25Weight:      bm25,
			VectorWeight:    vector,
			GraphConstraint: strings.TrimSpace(in.GraphConstraint),
			RerankerModel:   strings.TrimSpace(in.RerankerModel),
			ConflictPolicy:  normalizedPolicy,
			DeploymentState: state,
			PublishedAt:     publishedAt,
			PublishedBy:     in.RequestedBy,
		}
		if active != nil && state == models.FusionDeploymentActive {
			strategy.RollbackFromVersionID = &active.ID
			if err := tx.Model(active).
				Where("id = ?", active.ID).
				Update("deployment_state", models.FusionDeploymentRollback).Error; err != nil {
				return err
			}
		}

		if _, err := strategies.Create(ctx, strategy); err != nil {
			return err
		}
		created = strategy

		payload := map[string]any{
			"strategy_id":     strategy.ID,
			"label":           strategy.Label,
			"bm25_weight":     strategy.BM25Weight,
			"vector_weight":   strategy.VectorWeight,
			"conflict_policy": strategy.ConflictPolicy,
		}
		if err := s.writeAudit(ctx, tx, space, "fusion.published", in.RequestedBy, payload); err != nil {
			return err
		}
		if state == models.FusionDeploymentActive {
			s.publishFusionEvent(ctx, "published", space, strategy)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// RollbackStrategy re-activates a previous strategy version.
func (s *FusionService) RollbackStrategy(ctx context.Context, in RollbackStrategyInput) (*models.FusionStrategyVersion, error) {
	if in.SpaceID == uuid.Nil || in.StrategyID == 0 {
		return nil, ErrInvalidInput
	}
	start := time.Now()
	var updated *models.FusionStrategyVersion
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces := repo.NewKnowledgeSpaceRepository(tx)
		strategies := repo.NewFusionStrategyRepository(tx)

		target, err := strategies.FindByID(ctx, in.StrategyID)
		if err != nil {
			return err
		}
		if target == nil || target.SpaceUUID != in.SpaceID {
			return ErrFusionStrategyNotFound
		}

		space, err := spaces.FindByUUID(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if space == nil {
			return ErrSpaceNotFound
		}

		active, err := strategies.FindActiveBySpace(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if active != nil && active.ID != target.ID {
			if err := tx.Model(active).Update("deployment_state", models.FusionDeploymentRollback).Error; err != nil {
				return err
			}
		}

		now := s.clock()
		if err := tx.Model(target).Updates(map[string]any{
			"deployment_state": models.FusionDeploymentActive,
			"published_at":     now,
		}).Error; err != nil {
			return err
		}
		target.DeploymentState = models.FusionDeploymentActive
		target.PublishedAt = &now
		updated = target

		payload := map[string]any{
			"strategy_id": target.ID,
			"label":       target.Label,
		}
		if err := s.writeAudit(ctx, tx, space, "fusion.rollback", in.RequestedBy, payload); err != nil {
			return err
		}
		s.publishFusionEvent(ctx, "rollback", space, target)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.inst.RecordFusionRollbackLatency(time.Since(start))
	return updated, nil
}

// ListStrategies returns the latest strategy versions for a space.
func (s *FusionService) ListStrategies(ctx context.Context, space uuid.UUID, limit int) ([]*models.FusionStrategyVersion, error) {
	if space == uuid.Nil {
		return nil, ErrInvalidInput
	}
	strategies, err := repo.NewFusionStrategyRepository(s.db).ListBySpace(ctx, space, limit)
	if err != nil {
		return nil, err
	}
	return strategies, nil
}

// Query merges retrieval outputs based on the active strategy.
func (s *FusionService) Query(ctx context.Context, in FusionQueryInput) (FusionQueryResult, error) {
	if in.SpaceID == uuid.Nil || len(in.Embedding) == 0 {
		return FusionQueryResult{}, ErrInvalidInput
	}
	strategy, err := repo.NewFusionStrategyRepository(s.db).FindActiveBySpace(ctx, in.SpaceID)
	if err != nil {
		return FusionQueryResult{}, err
	}
	if strategy == nil {
		return FusionQueryResult{}, ErrFusionStrategyNotFound
	}
	if s.vectorStore == nil {
		return FusionQueryResult{StrategyID: strategy.ID}, nil
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 10
	}
	resp, err := s.vectorStore.Query(ctx, vectorstore.QueryRequest{
		SpaceID:   in.SpaceID,
		Embedding: in.Embedding,
		TopK:      topK,
		Filters:   in.Filters,
		MinScore:  in.MinScore,
	})
	if err != nil {
		s.publishFusionAlert(ctx, in.SpaceID, err)
		return FusionQueryResult{}, err
	}
	matches := make([]FusionQueryMatch, 0, len(resp.Matches))
	for _, match := range resp.Matches {
		matches = append(matches, FusionQueryMatch{
			ChunkID:  match.ChunkID,
			Score:    match.Score * strategy.VectorWeight,
			Source:   "vector",
			Metadata: match.Metadata,
		})
	}
	return FusionQueryResult{
		StrategyID: strategy.ID,
		Matches:    matches,
	}, nil
}

func (s *FusionService) publishFusionAlert(ctx context.Context, space uuid.UUID, cause error) {
	if s.bus == nil {
		return
	}
	payload := map[string]any{
		"space_id": space.String(),
		"event":    "fusion.source.failed",
		"error":    cause.Error(),
	}
	topic := s.eventTopic
	if topic == "" {
		topic = "knowledge.space.fusion"
	}
	s.bus.Publish(topic, payload, ctx)
}

func (s *FusionService) publishFusionEvent(ctx context.Context, verb string, space *models.KnowledgeSpace, strategy *models.FusionStrategyVersion) {
	if s.bus == nil || space == nil || strategy == nil {
		return
	}
	payload := map[string]any{
		"event":          verb,
		"space_id":       space.UUID.String(),
		"strategy_id":    strategy.ID,
		"label":          strategy.Label,
		"bm25_weight":    strategy.BM25Weight,
		"vector_weight":  strategy.VectorWeight,
		"deployment":     strategy.DeploymentState,
		"conflictPolicy": strategy.ConflictPolicy,
	}
	topic := s.eventTopic
	if topic == "" {
		topic = "knowledge.space.fusion"
	}
	s.bus.Publish(topic, payload, ctx)
}

func (s *FusionService) writeAudit(ctx context.Context, tx *gorm.DB, space *models.KnowledgeSpace, action, actor string, payload map[string]any) error {
	if space == nil {
		return ErrSpaceNotFound
	}
	entry := &models.AuditTrailEntry{
		SpaceUUID:     space.UUID,
		Action:        action,
		Actor:         actor,
		Metadata:      marshalJSON(payload),
		OccurredAt:    s.clock(),
		RollbackToken: space.AuditToken,
		PayloadHash:   computePayloadHash(payload),
	}
	_, err := repo.NewAuditTrailRepository(tx).Create(ctx, entry)
	return err
}

func normalizeConflictPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "block":
		return "block"
	case "queue":
		return "queue"
	case "allow_with_flag":
		return "allow_with_flag"
	default:
		return "allow_with_flag"
	}
}

func normalizeWeights(bm25, vector float64) (float64, float64) {
	if bm25 < 0 {
		bm25 = 0
	}
	if vector < 0 {
		vector = 0
	}
	sum := bm25 + vector
	if sum == 0 {
		return 0.5, 0.5
	}
	return bm25 / sum, vector / sum
}
