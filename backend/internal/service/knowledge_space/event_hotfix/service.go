package event_hotfix

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

var (
	ErrInvalidEvent   = errors.New("event_hotfix: invalid event")
	ErrPolicyMissing  = errors.New("event_hotfix: policy missing")
	ErrDuplicateEvent = errors.New("event_hotfix: duplicate event")
)

type Options struct {
	Instrumentation *instrumentation.Instrumentation
	EventBus        event_bus.EventBus
	MetricsWriter   *instrumentation.EventMetricsWriter
	AgentNotifier   *AgentNotifier
	PoliciesPath    string
	ReportPath      string
	Clock           func() time.Time
	RetryMax        int
}

type Service struct {
	inst       *instrumentation.Instrumentation
	bus        event_bus.EventBus
	metrics    *instrumentation.EventMetricsWriter
	notifier   *AgentNotifier
	policies   map[string]Policy
	processed  sync.Map
	clock      func() time.Time
	reportPath string
	retryMax   int
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
	return &Service{
		inst:       opts.Instrumentation,
		bus:        opts.EventBus,
		metrics:    opts.MetricsWriter,
		notifier:   opts.AgentNotifier,
		policies:   policies,
		clock:      opts.Clock,
		reportPath: opts.ReportPath,
		retryMax:   opts.RetryMax,
	}
}

func loadPolicies(path string) map[string]Policy {
	result := make(map[string]Policy)
	if strings.TrimSpace(path) == "" {
		return result
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	var payload struct {
		Policies []Policy `json:"policies" yaml:"policies"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return result
	}
	for _, p := range payload.Policies {
		if p.EventType == "" {
			continue
		}
		result[strings.ToLower(p.EventType)] = p
	}
	return result
}

func (s *Service) Apply(ctx context.Context, in ApplyInput) (*ApplyResult, error) {
	if strings.TrimSpace(in.EventID) == "" || strings.TrimSpace(in.EventType) == "" {
		return nil, ErrInvalidEvent
	}
	key := strings.ToLower(strings.TrimSpace(in.EventType))
	policy, ok := s.policies[key]
	if !ok {
		return nil, ErrPolicyMissing
	}
	if _, loaded := s.processed.LoadOrStore(in.EventID, struct{}{}); loaded {
		s.recordMetrics(in, true, 0, false)
		return nil, ErrDuplicateEvent
	}
	if s.bus != nil {
		s.bus.Publish("knowledge.event.received", map[string]any{
			"event_id":   in.EventID,
			"event_type": in.EventType,
			"severity":   policy.Severity,
		}, ctx)
	}
	agentOK := false
	if s.notifier != nil {
		agentOK = s.notifier.Refresh(ctx, in.Payload)
	}
	s.persistReport(in, policy, agentOK)
	s.recordMetrics(in, false, in.RetryCount, agentOK)
	return &ApplyResult{
		Status:    "applied",
		EventID:   in.EventID,
		Processed: s.clock().UTC(),
	}, nil
}

func (s *Service) Retry(ctx context.Context, in ApplyInput) (*ApplyResult, error) {
	if in.RetryCount >= s.retryMax {
		return nil, ErrInvalidEvent
	}
	in.RetryCount++
	return s.Apply(ctx, in)
}

func (s *Service) persistReport(in ApplyInput, policy Policy, agentOK bool) {
	if strings.TrimSpace(s.reportPath) == "" {
		return
	}
	report := map[string]any{
		"eventId":     in.EventID,
		"eventType":   in.EventType,
		"policy":      policy,
		"agentSync":   agentOK,
		"processedAt": s.clock().UTC().Format(time.RFC3339Nano),
	}
	_ = os.MkdirAll(filepath.Dir(s.reportPath), 0o755)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.reportPath, data, 0o644)
}

func (s *Service) recordMetrics(in ApplyInput, idempotent bool, retries int, agentOK bool) {
	if s.metrics == nil {
		return
	}
	latency := int64(s.clock().UTC().Sub(in.ReceivedAt).Milliseconds())
	snapshot := instrumentation.EventMetricsSnapshot{
		EventID:        in.EventID,
		EventType:      in.EventType,
		LatencyMs:      latency,
		RetryCount:     retries,
		IdempotentSkip: idempotent,
		AgentRefreshOK: agentOK,
	}
	_ = s.metrics.Store(snapshot)
}
