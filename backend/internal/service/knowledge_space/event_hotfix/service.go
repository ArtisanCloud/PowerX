package event_hotfix

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gopkg.in/yaml.v3"
)

var (
	ErrInvalidEvent   = errors.New("event_hotfix: invalid event")
	ErrPolicyMissing  = errors.New("event_hotfix: policy missing")
	ErrDuplicateEvent = errors.New("event_hotfix: duplicate event")
)

type Options struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	EventBus        event_bus.EventBus
	MetricsWriter   *instrumentation.EventMetricsWriter
	AgentNotifier   *AgentNotifier
	PoliciesPath    string
	ReportPath      string
	Clock           func() time.Time
	RetryMax        int
	ReplayWindow    time.Duration
}

type Service struct {
	db         *gorm.DB
	inst       *instrumentation.Instrumentation
	bus        event_bus.EventBus
	metrics    *instrumentation.EventMetricsWriter
	notifier   *AgentNotifier
	policies   map[string]Policy
	processed  sync.Map
	clock      func() time.Time
	reportPath string
	retryMax   int
	replayWin  time.Duration
}

type Policy struct {
	EventType string   `json:"eventType" yaml:"eventType"`
	Actions   []string `json:"actions" yaml:"actions"`
	Severity  string   `json:"severity" yaml:"severity"`
}

type ApplyInput struct {
	EventID    string         `json:"eventId"`
	EventType  string         `json:"eventType"`
	Payload    map[string]any `json:"payload"`
	ReceivedAt time.Time      `json:"receivedAt"`
	RetryCount int            `json:"retryCount"`
}

type ApplyResult struct {
	Status    string    `json:"status"`
	EventID   string    `json:"eventId"`
	Processed time.Time `json:"processedAt"`
}

type processedMarker struct {
	Status     string
	RecordedAt time.Time
}

func NewService(opts Options) *Service {
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	policies := loadPolicies(opts.PoliciesPath)
	if opts.RetryMax <= 0 {
		opts.RetryMax = 3
	}
	if opts.ReplayWindow <= 0 {
		opts.ReplayWindow = 5 * time.Minute
	}
	return &Service{
		db:         opts.DB,
		inst:       opts.Instrumentation,
		bus:        opts.EventBus,
		metrics:    opts.MetricsWriter,
		notifier:   opts.AgentNotifier,
		policies:   policies,
		clock:      opts.Clock,
		reportPath: opts.ReportPath,
		retryMax:   opts.RetryMax,
		replayWin:  opts.ReplayWindow,
	}
}

func loadPolicies(path string) map[string]Policy {
	result := make(map[string]Policy)
	if strings.TrimSpace(path) == "" {
		return withBuiltinPolicies(result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return withBuiltinPolicies(result)
	}
	var payload struct {
		Policies []Policy `json:"policies" yaml:"policies"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		_ = yaml.Unmarshal(data, &payload)
	}
	for _, p := range payload.Policies {
		if p.EventType == "" {
			continue
		}
		result[strings.ToLower(p.EventType)] = p
	}
	return withBuiltinPolicies(result)
}

func withBuiltinPolicies(policies map[string]Policy) map[string]Policy {
	if policies == nil {
		policies = make(map[string]Policy)
	}
	// Built-ins ensure admin/ops endpoints work even when policy file is minimal.
	if _, ok := policies["agent.weight.refresh"]; !ok {
		policies["agent.weight.refresh"] = Policy{
			EventType: "agent.weight.refresh",
			Actions:   []string{"agent-refresh"},
			Severity:  "p1",
		}
	}
	if _, ok := policies["index.hot_update"]; !ok {
		policies["index.hot_update"] = Policy{
			EventType: "index.hot_update",
			Actions:   []string{"hot-update"},
			Severity:  "p1",
		}
	}
	return policies
}

func (s *Service) Apply(ctx context.Context, in ApplyInput) (*ApplyResult, error) {
	if strings.TrimSpace(in.EventID) == "" || strings.TrimSpace(in.EventType) == "" {
		return nil, ErrInvalidEvent
	}
	now := s.clock().UTC()
	if !in.ReceivedAt.IsZero() && now.Sub(in.ReceivedAt) > s.replayWin {
		return nil, ErrInvalidEvent
	}
	key := strings.ToLower(strings.TrimSpace(in.EventType))
	policy, ok := s.policies[key]
	if !ok {
		return nil, ErrPolicyMissing
	}

	marker := processedMarker{Status: "in_flight", RecordedAt: now}
	if existing, loaded := s.processed.LoadOrStore(in.EventID, marker); loaded {
		if prev, ok := existing.(processedMarker); ok {
			// Allow retries after replay window expiry.
			if now.Sub(prev.RecordedAt) <= s.replayWin {
				s.recordMetrics(in, policy, true, in.RetryCount, false, false)
				return nil, ErrDuplicateEvent
			}
		} else {
			s.recordMetrics(in, policy, true, in.RetryCount, false, false)
			return nil, ErrDuplicateEvent
		}
		s.processed.Store(in.EventID, marker)
	}

	if s.bus != nil {
		s.bus.Publish("knowledge.event.received", map[string]any{
			"event_id":   in.EventID,
			"event_type": in.EventType,
			"severity":   policy.Severity,
		}, ctx)
	}
	result, err := s.applyActions(ctx, in, policy)
	if err != nil {
		s.processed.Delete(in.EventID)
		return nil, err
	}
	s.processed.Store(in.EventID, processedMarker{Status: "completed", RecordedAt: now})
	return result, nil
}

func (s *Service) Retry(ctx context.Context, in ApplyInput) (*ApplyResult, error) {
	if in.RetryCount >= s.retryMax {
		return nil, ErrInvalidEvent
	}
	in.RetryCount++
	return s.Apply(ctx, in)
}

func (s *Service) applyActions(ctx context.Context, in ApplyInput, policy Policy) (*ApplyResult, error) {
	actions := normalizeActions(policy.Actions)
	agentOK := false
	hotfixOK := false
	for _, action := range actions {
		switch action {
		case "agent-refresh":
			if s.notifier != nil {
				agentOK = s.notifier.Refresh(ctx, in.EventType, in.Payload)
			}
		case "hot-update":
			hotfixOK = s.tryHotUpdate(ctx, in)
		default:
			// fetch/patch are treated as no-op in this slice; real implementation can plug in.
		}
	}

	s.recordMetrics(in, policy, false, in.RetryCount, agentOK, hotfixOK)

	processed := s.clock().UTC()
	return &ApplyResult{
		Status:    "applied",
		EventID:   in.EventID,
		Processed: processed,
	}, nil
}

func normalizeActions(actions []string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		out = append(out, a)
	}
	return out
}

func (s *Service) tryHotUpdate(ctx context.Context, in ApplyInput) bool {
	if s == nil || s.db == nil {
		return true
	}
	rawSpace, _ := in.Payload["spaceId"].(string)
	if rawSpace == "" {
		rawSpace, _ = in.Payload["space_id"].(string)
	}
	spaceID := strings.TrimSpace(rawSpace)
	if spaceID == "" {
		return false
	}
	now := s.clock().UTC()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry := &models.AuditTrailEntry{
			Action:     "event.hot_update",
			Actor:      "event_hotfix",
			OccurredAt: now,
			Metadata:   mustJSON(map[string]any{"event_id": in.EventID, "event_type": in.EventType}),
			PayloadHash: computePayloadHash(map[string]any{
				"event_id":   in.EventID,
				"event_type": in.EventType,
				"space_id":   spaceID,
			}),
			RollbackToken: "event-hotfix:" + spaceID,
		}
		// SpaceUUID is required; keep best-effort without failing whole flow if parse fails.
		if parsed, err := parseUUID(spaceID); err == nil {
			entry.SpaceUUID = parsed
		}
		return tx.Create(entry).Error
	}) == nil
}

func (s *Service) recordMetrics(in ApplyInput, policy Policy, idempotent bool, retries int, agentOK bool, hotfixOK bool) {
	if s.metrics == nil {
		return
	}
	latency := int64(s.clock().UTC().Sub(in.ReceivedAt).Milliseconds())
	snapshot := instrumentation.EventMetricsSnapshot{
		EventID:        in.EventID,
		EventType:      in.EventType,
		PolicySeverity: strings.TrimSpace(policy.Severity),
		Actions:        normalizeActions(policy.Actions),
		LatencyMs:      latency,
		RetryCount:     retries,
		IdempotentSkip: idempotent,
		AgentRefreshOK: agentOK,
		HotUpdateOK:    hotfixOK,
	}
	_ = s.metrics.Store(snapshot)
}

func mustJSON(v any) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	data, err := json.Marshal(v)
	if err != nil || len(data) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(data)
}

func computePayloadHash(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	sum := sha256Sum(data)
	return sum
}

func sha256Sum(data []byte) string {
	h := sha256.New()
	_, _ = h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func parseUUID(raw string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(raw))
}
