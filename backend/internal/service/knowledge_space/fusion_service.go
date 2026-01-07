package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// FusionService orchestrates multi-source strategy lifecycle and retrieval.
type FusionService struct {
	db          *gorm.DB
	inst        *instrumentation.Instrumentation
	vectorStore vectorstore.Store
	sparseIndex SparseIndex
	bus         event_bus.EventBus
	eventTopic  string
	clock       func() time.Time
}

// FusionServiceOptions configure the fusion service.
type FusionServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	VectorStore     vectorstore.Store
	SparseIndex     SparseIndex
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
	QueryText string
	Embedding []float32
	Filters   map[string]string
	TopK      int
	MinScore  float64
}

// FusionQueryMatch describes fused retrieval output.
type FusionQueryMatch struct {
	ChunkID   uuid.UUID
	Score     float64
	Source    string
	RawScore  float64
	NormScore float64
	Metadata  map[string]any
}

// FusionQueryResult aggregates vector/lexical results.
type FusionQueryResult struct {
	StrategyID     uint64
	Matches        []FusionQueryMatch
	Degraded       bool
	DegradeReasons []string
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
		sparseIndex: opts.SparseIndex,
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
	requestedWeights := map[string]float64{
		"bm25":   in.BM25Weight,
		"vector": in.VectorWeight,
	}

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

		available := s.availableSources(ctx)
		weights, degradeReasons := normalizeAvailableWeights(requestedWeights, available)
		if weights["bm25"]+weights["vector"] <= 0 {
			return ErrInvalidInput
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
			SpaceUUID:        in.SpaceID,
			Label:            strings.TrimSpace(in.Label),
			BM25Weight:       weights["bm25"],
			VectorWeight:     weights["vector"],
			GraphConstraint:  strings.TrimSpace(in.GraphConstraint),
			RerankerModel:    strings.TrimSpace(in.RerankerModel),
			ConflictPolicy:   normalizedPolicy,
			DeploymentState:  state,
			PublishedAt:      publishedAt,
			PublishedBy:      in.RequestedBy,
			BenchmarkMetrics: strategyMetricsSnapshot(weights, available, degradeReasons),
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
			"degraded":        len(degradeReasons) > 0,
			"degrade_reason":  strings.Join(degradeReasons, ";"),
		}
		if err := s.writeAudit(ctx, tx, space, "fusion.published", in.RequestedBy, payload); err != nil {
			return err
		}
		if state == models.FusionDeploymentActive {
			s.publishFusionEvent(ctx, "published", space, strategy)
			for _, reason := range degradeReasons {
				s.publishFusionAlert(ctx, in.SpaceID, strategy.ID, "publish", reason, errors.New(reason))
			}
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
	if in.SpaceID == uuid.Nil || (len(in.Embedding) == 0 && strings.TrimSpace(in.QueryText) == "") {
		return FusionQueryResult{}, ErrInvalidInput
	}
	strategy, err := repo.NewFusionStrategyRepository(s.db).FindActiveBySpace(ctx, in.SpaceID)
	if err != nil {
		return FusionQueryResult{}, err
	}
	if strategy == nil {
		return FusionQueryResult{}, ErrFusionStrategyNotFound
	}
	topK := in.TopK
	if topK <= 0 {
		topK = 10
	}

	weights, degradeReasons := strategyWeights(strategy)
	available := s.availableSources(ctx)
	weights, degradeReasons = normalizeAvailableWeights(weights, available, degradeReasons...)

	type accum struct {
		score     float64
		perSource []FusionQueryMatch
	}
	acc := make(map[uuid.UUID]*accum)

	vectorErr := error(nil)
	if weights["vector"] > 0 && s.vectorStore != nil && len(in.Embedding) > 0 {
		resp, err := s.vectorStore.Query(ctx, vectorstore.QueryRequest{
			SpaceID:   in.SpaceID,
			Embedding: in.Embedding,
			TopK:      topK,
			Filters:   in.Filters,
			MinScore:  in.MinScore,
		})
		if err != nil {
			vectorErr = err
			degradeReasons = append(degradeReasons, "vector_query_failed")
			s.publishFusionAlert(ctx, in.SpaceID, strategy.ID, "vector", "vector_query_failed", err)
			weights["vector"] = 0
		} else {
			for _, match := range resp.Matches {
				norm := clamp01(match.Score)
				item := FusionQueryMatch{
					ChunkID:   match.ChunkID,
					RawScore:  match.Score,
					NormScore: norm,
					Score:     norm * weights["vector"],
					Source:    "vector",
					Metadata:  match.Metadata,
				}
				slot := acc[match.ChunkID]
				if slot == nil {
					slot = &accum{}
					acc[match.ChunkID] = slot
				}
				slot.score += item.Score
				slot.perSource = append(slot.perSource, item)
			}
		}
	}

	sparseErr := error(nil)
	queryText := strings.TrimSpace(in.QueryText)
	if weights["bm25"] > 0 && s.sparseIndex != nil && queryText != "" {
		resp, err := s.sparseIndex.Query(ctx, SparseQueryRequest{
			SpaceID:  in.SpaceID,
			Query:    queryText,
			TopK:     topK,
			Filters:  in.Filters,
			MinScore: in.MinScore,
		})
		if err != nil {
			sparseErr = err
			degradeReasons = append(degradeReasons, "bm25_query_failed")
			s.publishFusionAlert(ctx, in.SpaceID, strategy.ID, "bm25", "bm25_query_failed", err)
			weights["bm25"] = 0
		} else {
			for _, match := range resp.Matches {
				norm := normalizeSparseScore(match.Score)
				meta := match.Metadata
				if meta == nil {
					meta = map[string]any{}
				}
				if match.Provenance != nil {
					meta["provenance"] = match.Provenance
				}
				item := FusionQueryMatch{
					ChunkID:   match.ChunkID,
					RawScore:  match.Score,
					NormScore: norm,
					Score:     norm * weights["bm25"],
					Source:    "bm25",
					Metadata:  meta,
				}
				slot := acc[match.ChunkID]
				if slot == nil {
					slot = &accum{}
					acc[match.ChunkID] = slot
				}
				slot.score += item.Score
				slot.perSource = append(slot.perSource, item)
			}
		}
	}

	if weights["vector"] == 0 && weights["bm25"] == 0 {
		if s.tryAutoRollback(ctx, strategy, in.SpaceID, []error{vectorErr, sparseErr}) {
			return FusionQueryResult{}, ErrFusionConflict
		}
		return FusionQueryResult{}, errors.New("no available fusion sources")
	}

	out := make([]FusionQueryMatch, 0, len(acc))
	for chunkID, slot := range acc {
		best := FusionQueryMatch{
			ChunkID:  chunkID,
			Score:    slot.score,
			Source:   "fused",
			Metadata: map[string]any{"sources": slot.perSource},
		}
		out = append(out, best)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > topK {
		out = out[:topK]
	}
	return FusionQueryResult{
		StrategyID:     strategy.ID,
		Matches:        out,
		Degraded:       len(degradeReasons) > 0,
		DegradeReasons: uniqueStrings(degradeReasons),
	}, nil
}

func (s *FusionService) publishFusionAlert(ctx context.Context, space uuid.UUID, strategyID uint64, source string, degradeReason string, cause error) {
	if s.bus == nil {
		return
	}
	traceID := reqctx.GetTraceID(ctx)
	payload := map[string]any{
		"space_id":       space.String(),
		"strategy_id":    strategyID,
		"source":         source,
		"degrade_reason": degradeReason,
		"trace_id":       traceID,
		"event":          "fusion.source.failed",
		"error":          errString(cause),
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

func (s *FusionService) availableSources(ctx context.Context) map[string]bool {
	available := map[string]bool{
		"vector": s.vectorStore != nil,
		"bm25":   s.sparseIndex != nil,
	}
	if s.vectorStore != nil {
		if err := s.vectorStore.Health(ctx); err != nil {
			available["vector"] = false
		}
	}
	if s.sparseIndex != nil {
		if err := s.sparseIndex.Health(ctx); err != nil {
			available["bm25"] = false
		}
	}
	return available
}

type strategyMetrics struct {
	Weights        map[string]float64 `json:"weights"`
	Available      map[string]bool    `json:"available"`
	DegradeReasons []string           `json:"degrade_reasons,omitempty"`
}

func strategyMetricsSnapshot(weights map[string]float64, available map[string]bool, degradeReasons []string) datatypes.JSON {
	snap := strategyMetrics{
		Weights:   weights,
		Available: available,
	}
	if len(degradeReasons) > 0 {
		snap.DegradeReasons = uniqueStrings(degradeReasons)
	}
	raw, _ := json.Marshal(snap)
	return datatypes.JSON(raw)
}

func strategyWeights(strategy *models.FusionStrategyVersion) (map[string]float64, []string) {
	if strategy == nil {
		return map[string]float64{"bm25": 0, "vector": 0}, nil
	}
	weights := map[string]float64{
		"bm25":   strategy.BM25Weight,
		"vector": strategy.VectorWeight,
	}
	var snap strategyMetrics
	if len(strategy.BenchmarkMetrics) > 0 && json.Unmarshal(strategy.BenchmarkMetrics, &snap) == nil {
		if len(snap.Weights) > 0 {
			for k, v := range snap.Weights {
				weights[k] = v
			}
		}
		return weights, snap.DegradeReasons
	}
	return weights, nil
}

func normalizeAvailableWeights(weights map[string]float64, available map[string]bool, extraReasons ...string) (map[string]float64, []string) {
	out := map[string]float64{}
	reasons := append([]string{}, extraReasons...)
	for k, v := range weights {
		if v < 0 {
			v = 0
		}
		if available != nil {
			if ok, exists := available[k]; exists && !ok && v > 0 {
				reasons = append(reasons, k+"_unavailable")
				v = 0
			}
		}
		out[k] = v
	}
	sum := 0.0
	for _, v := range out {
		sum += v
	}
	if sum <= 0 {
		return out, uniqueStrings(reasons)
	}
	for k, v := range out {
		out[k] = v / sum
	}
	return out, uniqueStrings(reasons)
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func normalizeSparseScore(score float64) float64 {
	if score <= 0 {
		return 0
	}
	// Normalize any positive score into (0,1) using a smooth curve.
	return score / (score + 1.0 + math.SmallestNonzeroFloat64)
}

func (s *FusionService) tryAutoRollback(ctx context.Context, strategy *models.FusionStrategyVersion, space uuid.UUID, causes []error) bool {
	if strategy == nil || strategy.RollbackFromVersionID == nil || *strategy.RollbackFromVersionID == 0 {
		return false
	}
	if strategy.PublishedAt == nil {
		return false
	}
	if s.clock().Sub(*strategy.PublishedAt) > 5*time.Minute {
		return false
	}
	reason := "auto_rollback_on_source_failure"
	for _, cause := range causes {
		if cause != nil {
			reason = reason + ":" + cause.Error()
			break
		}
	}
	_, err := s.RollbackStrategy(ctx, RollbackStrategyInput{
		SpaceID:     space,
		StrategyID:  *strategy.RollbackFromVersionID,
		RequestedBy: "auto",
	})
	if err != nil {
		s.publishFusionAlert(ctx, space, strategy.ID, "auto_rollback", "auto_rollback_failed", err)
		return false
	}
	s.publishFusionAlert(ctx, space, strategy.ID, "auto_rollback", "auto_rollback_triggered", errors.New(reason))
	return true
}
