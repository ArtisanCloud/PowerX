package knowledge_space

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/corpus_check"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventFabricCorpusCheckPipelineOptions struct {
	Delivery      delivery.Service
	Directory     *directory.DirectoryService
	ACL           *acl.ACLService
	SubscriberID  string
	Namespace     string
	Name          string
	PayloadFormat string
	MaxRetry      int32
	AckTimeoutSec int32
	Clock         func() time.Time
}

// EventFabricCorpusCheckPipeline schedules corpus check jobs via Event Fabric.
type EventFabricCorpusCheckPipeline struct {
	delivery     delivery.Service
	directory    *directory.DirectoryService
	acl          *acl.ACLService
	subscriberID string
	namespace    string
	name         string
	format       string
	maxRetry     int32
	ackTimeout   int32
	clock        func() time.Time
}

func NewEventFabricCorpusCheckPipeline(opts EventFabricCorpusCheckPipelineOptions) *EventFabricCorpusCheckPipeline {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	subscriber := strings.TrimSpace(opts.SubscriberID)
	if subscriber == "" {
		subscriber = "core.knowledge_space.corpus_check"
	}
	namespace := strings.TrimSpace(opts.Namespace)
	if namespace == "" {
		// Event Fabric namespace pattern: ^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$
		// underscores are not allowed.
		namespace = "knowledge.space.corpuscheck"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "run"
	}
	format := strings.TrimSpace(opts.PayloadFormat)
	if format == "" {
		format = "json"
	}
	maxRetry := opts.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 5
	}
	ackTimeout := opts.AckTimeoutSec
	if ackTimeout <= 0 {
		ackTimeout = 30
	}
	return &EventFabricCorpusCheckPipeline{
		delivery:     opts.Delivery,
		directory:    opts.Directory,
		acl:          opts.ACL,
		subscriberID: subscriber,
		namespace:    namespace,
		name:         name,
		format:       format,
		maxRetry:     maxRetry,
		ackTimeout:   ackTimeout,
		clock:        opts.Clock,
	}
}

func (p *EventFabricCorpusCheckPipeline) Schedule(ctx context.Context, input CorpusCheckInput) (CorpusCheckTask, error) {
	if p == nil || p.delivery == nil || p.directory == nil || p.acl == nil {
		return CorpusCheckTask{}, fmt.Errorf("event_fabric corpus_check pipeline not configured")
	}
	if input.JobUUID == uuid.Nil || input.SpaceID == uuid.Nil {
		return CorpusCheckTask{}, fmt.Errorf("missing job_uuid/space_id")
	}

	tenantKey := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantKey == "" {
		return CorpusCheckTask{}, fmt.Errorf("tenant context missing")
	}
	tenantKey, err := reqctx.CanonicalTenantUUID(tenantKey)
	if err != nil {
		return CorpusCheckTask{}, fmt.Errorf("invalid tenant uuid: %w", err)
	}

	if err := p.ensureTopic(ctx, tenantKey); err != nil {
		return CorpusCheckTask{}, err
	}

	fullTopic := fmt.Sprintf("%s.%s.%s", tenantKey, strings.TrimSpace(p.namespace), strings.TrimSpace(p.name))
	payload := map[string]any{
		"job_uuid":    input.JobUUID.String(),
		"space_id":    input.SpaceID.String(),
		"requestedBy": input.RequestedBy,
	}
	body, _ := json.Marshal(payload)
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))

	if err := p.delivery.Publish(ctx, delivery.PublishRequest{
		TenantUUID:     tenantKey,
		Topic:          fullTopic,
		EventID:        fmt.Sprintf("knowledge-corpus-check-%s", input.JobUUID.String()),
		TraceID:        traceID,
		Version:        "v1",
		Payload:        body,
		PayloadFormat:  p.format,
		IdempotencyKey: input.JobUUID.String(),
		Attributes: map[string]string{
			"subscriber_id": p.subscriberID,
			"space_id":      input.SpaceID.String(),
			"job_uuid":      input.JobUUID.String(),
			"principal_id":  p.subscriberID,
		},
	}); err != nil {
		return CorpusCheckTask{}, err
	}

	return CorpusCheckTask{
		JobUUID:     input.JobUUID,
		Status:      "scheduled",
		ScheduledAt: p.clock().UTC(),
	}, nil
}

func (p *EventFabricCorpusCheckPipeline) ensureTopic(ctx context.Context, tenantKey string) error {
	if p == nil || p.directory == nil || p.acl == nil {
		return fmt.Errorf("directory/acl not configured")
	}
	namespace := strings.ToLower(strings.TrimSpace(p.namespace))
	name := strings.ToLower(strings.TrimSpace(p.name))
	existing, err := p.directory.FindTopicByFullName(ctx, tenantKey, namespace, name)
	if err != nil {
		return err
	}
	var topicID string
	if existing == nil {
		created, err := p.directory.CreateTopic(ctx, directory.CreateTopicInput{
			TenantUUID:     tenantKey,
			Namespace:      namespace,
			Name:           name,
			PayloadFormat:  p.format,
			MaxRetry:       p.maxRetry,
			AckTimeoutSec:  p.ackTimeout,
			VersioningMode: "any",
			Metadata: map[string]any{
				"module": "knowledge_space",
				"kind":   "corpus_check",
			},
			CreatedBy: "knowledge-space",
		})
		if err != nil {
			return err
		}
		topicID = created.ID
	} else {
		topicID = existing.UUID.String()
	}

	if topicID == "" {
		return fmt.Errorf("topic id empty")
	}

	_, err = p.acl.Grant(ctx, acl.GrantRequest{
		TenantUUID:    tenantKey,
		TopicUUID:     topicID,
		PrincipalType: "service",
		PrincipalID:   p.subscriberID,
		Actions: []acl.PrincipalAction{
			acl.PrincipalActionSubscribe,
			acl.PrincipalActionPublish,
		},
		Justification: "knowledge_space corpus check worker",
		OperatorID:    "knowledge-space",
	})
	return err
}

type EventFabricCorpusCheckConsumerOptions struct {
	Delivery       delivery.Service
	DB             *gorm.DB
	Directory      *directory.DirectoryService
	ACL            *acl.ACLService
	SubscriberID   string
	Namespace      string
	Name           string
	PayloadFormat  string
	TenantProvider func(context.Context) ([]string, error)
	Interval       time.Duration
	BatchSize      int
	Clock          func() time.Time
}

type EventFabricCorpusCheckConsumer struct {
	delivery       delivery.Service
	db             *gorm.DB
	directory      *directory.DirectoryService
	acl            *acl.ACLService
	subscriberID   string
	namespace      string
	name           string
	tenantProvider func(context.Context) ([]string, error)
	interval       time.Duration
	batchSize      int
	clock          func() time.Time
	cancel         context.CancelFunc
	ensuredTenants map[string]time.Time
	ensureEvery    time.Duration
}

func NewEventFabricCorpusCheckConsumer(opts EventFabricCorpusCheckConsumerOptions) *EventFabricCorpusCheckConsumer {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 50
	}
	subscriber := strings.TrimSpace(opts.SubscriberID)
	if subscriber == "" {
		subscriber = "core.knowledge_space.corpus_check"
	}
	namespace := strings.TrimSpace(opts.Namespace)
	if namespace == "" {
		// Event Fabric namespace pattern: ^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$
		// underscores are not allowed.
		namespace = "knowledge.space.corpuscheck"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "run"
	}
	provider := opts.TenantProvider
	if provider == nil {
		provider = func(context.Context) ([]string, error) { return []string{"global"}, nil }
	}
	ensureEvery := 10 * time.Minute
	return &EventFabricCorpusCheckConsumer{
		delivery:       opts.Delivery,
		db:             opts.DB,
		directory:      opts.Directory,
		acl:            opts.ACL,
		subscriberID:   subscriber,
		namespace:      namespace,
		name:           name,
		tenantProvider: provider,
		interval:       interval,
		batchSize:      batch,
		clock:          opts.Clock,
		ensuredTenants: map[string]time.Time{},
		ensureEvery:    ensureEvery,
	}
}

func (c *EventFabricCorpusCheckConsumer) Start() {
	if c == nil || c.cancel != nil || c.delivery == nil || c.db == nil || c.directory == nil || c.acl == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.loop(ctx)
}

func (c *EventFabricCorpusCheckConsumer) Stop() {
	if c == nil || c.cancel == nil {
		return
	}
	c.cancel()
	c.cancel = nil
}

func (c *EventFabricCorpusCheckConsumer) loop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tenants, err := c.tenantProvider(ctx)
			if err != nil {
				continue
			}
			for _, tenantKey := range tenants {
				tenantKey = strings.TrimSpace(tenantKey)
				if tenantKey == "" {
					continue
				}
				// Avoid re-ensuring topic/ACL bindings on every poll tick (can spam audit logs).
				// Ensure at most once per TTL per tenant; on failure we retry on next tick.
				if last, ok := c.ensuredTenants[tenantKey]; !ok || time.Since(last) >= c.ensureEvery {
					if err := c.ensureTopic(ctx, tenantKey); err == nil {
						c.ensuredTenants[tenantKey] = time.Now()
					}
				}
				fullTopic := fmt.Sprintf("%s.%s.%s", tenantKey, strings.TrimSpace(c.namespace), strings.TrimSpace(c.name))
				runCtx := context.WithValue(ctx, sharedsvc.ContextTenantKey, tenantKey)
				runCtx = context.WithValue(runCtx, sharedsvc.ContextSubscriberKey, c.subscriberID)
				runCtx = context.WithValue(runCtx, sharedsvc.ContextTopicsKey, map[string]struct{}{
					strings.ToLower(fullTopic): {},
				})

				results, err := c.delivery.PollRetry(runCtx, c.batchSize)
				if err != nil {
					continue
				}
				for _, attempts := range results {
					for _, attempt := range attempts {
						in, decodeErr := decodeCorpusCheckPayload(attempt.Payload)
						if decodeErr != nil {
							_, _ = c.delivery.Nack(runCtx, attempt.DeliveryUUID, c.subscriberID, decodeErr.Error())
							continue
						}
						if err := c.run(runCtx, tenantKey, in); err != nil {
							_, _ = c.delivery.Nack(runCtx, attempt.DeliveryUUID, c.subscriberID, err.Error())
							continue
						}
						_ = c.delivery.Ack(runCtx, attempt.DeliveryUUID, c.subscriberID)
					}
				}
			}
		}
	}
}

type corpusCheckPayload struct {
	JobUUID     string `json:"job_uuid"`
	SpaceID     string `json:"space_id"`
	RequestedBy string `json:"requestedBy"`
}

func decodeCorpusCheckPayload(payload []byte) (CorpusCheckInput, error) {
	var raw corpusCheckPayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return CorpusCheckInput{}, err
	}
	jobUUID, err := uuid.Parse(strings.TrimSpace(raw.JobUUID))
	if err != nil {
		return CorpusCheckInput{}, err
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(raw.SpaceID))
	if err != nil {
		return CorpusCheckInput{}, err
	}
	return CorpusCheckInput{
		JobUUID:     jobUUID,
		SpaceID:     spaceID,
		RequestedBy: strings.TrimSpace(raw.RequestedBy),
	}, nil
}

func (c *EventFabricCorpusCheckConsumer) ensureTopic(ctx context.Context, tenantKey string) error {
	p := &EventFabricCorpusCheckPipeline{
		directory:    c.directory,
		acl:          c.acl,
		subscriberID: c.subscriberID,
		namespace:    c.namespace,
		name:         c.name,
		format:       "json",
		maxRetry:     5,
		ackTimeout:   30,
		clock:        c.clock,
	}
	return p.ensureTopic(ctx, tenantKey)
}

func (c *EventFabricCorpusCheckConsumer) run(ctx context.Context, tenantKey string, input CorpusCheckInput) error {
	if c == nil || c.db == nil {
		return nil
	}
	now := c.clock().UTC()
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		jobs := repo.NewCorpusCheckJobRepository(tx)
		job, err := jobs.FindByUUID(ctx, input.JobUUID)
		if err != nil {
			return err
		}
		if job == nil {
			return nil
		}
		if !strings.EqualFold(job.TenantUUID, tenantKey) || job.SpaceUUID != input.SpaceID {
			return nil
		}
		if job.Status == models.CorpusCheckStatusCompleted {
			return nil
		}
		job.Status = models.CorpusCheckStatusRunning
		job.StartedAt = &now
		job.ErrorReason = ""
		job.UpdatedAt = now
		if _, err := jobs.Update(ctx, job); err != nil {
			return err
		}

		var sample []models.IngestionJob
		if err := tx.Model(&models.IngestionJob{}).
			Where("space_uuid = ?", input.SpaceID).
			Order("id DESC").
			Limit(50).
			Find(&sample).Error; err != nil {
			return err
		}

		metrics, recs := corpus_check.BuildMetrics(sample)
		job.Metrics = metrics
		job.Recommendations = recs
		job.SampleJobUUIDs = sampleJobUUIDs(sample)
		doneAt := c.clock().UTC()
		job.Status = models.CorpusCheckStatusCompleted
		job.CompletedAt = &doneAt
		job.UpdatedAt = doneAt
		_, err = jobs.Update(ctx, job)
		return err
	})
}
