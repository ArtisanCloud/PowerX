package knowledge_space

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	sharedsvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type EventFabricReprocessPipelineOptions struct {
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

// EventFabricReprocessPipeline schedules feedback reprocess jobs via Event Fabric (topic + delivery + DLQ).
type EventFabricReprocessPipeline struct {
	delivery     delivery.Service
	directory    *directory.DirectoryService
	acl          *acl.ACLService
	subscriberID string
	namespace    string
	name         string
	format       string
	maxRetry     int32
	ackTimeout   int32
	mu           sync.Mutex
	counter      uint64
	clock        func() time.Time
}

func NewEventFabricReprocessPipeline(opts EventFabricReprocessPipelineOptions) *EventFabricReprocessPipeline {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	subscriber := strings.TrimSpace(opts.SubscriberID)
	if subscriber == "" {
		subscriber = "core.knowledge_space.reprocess"
	}
	namespace := strings.TrimSpace(opts.Namespace)
	if namespace == "" {
		namespace = "knowledge.space.feedback"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "reprocess"
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
	return &EventFabricReprocessPipeline{
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

func (p *EventFabricReprocessPipeline) Schedule(ctx context.Context, input ReprocessInput) (ReprocessTask, error) {
	if p == nil || p.delivery == nil || p.directory == nil || p.acl == nil {
		return ReprocessTask{}, status.Error(codes.FailedPrecondition, "event_fabric reprocess pipeline not configured")
	}
	p.mu.Lock()
	p.counter++
	jobID := p.counter
	p.mu.Unlock()

	tenantKey := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantKey == "" {
		return ReprocessTask{}, status.Error(codes.Unauthenticated, "tenant context missing")
	}
	tenantKey, err := reqctx.CanonicalTenantUUID(tenantKey)
	if err != nil {
		return ReprocessTask{}, status.Errorf(codes.InvalidArgument, "invalid tenant uuid: %v", err)
	}

	if err := p.ensureTopic(ctx, tenantKey); err != nil {
		return ReprocessTask{}, err
	}

	fullTopic := fmt.Sprintf("%s.%s.%s", tenantKey, strings.TrimSpace(p.namespace), strings.TrimSpace(p.name))
	payload := map[string]any{
		"job_id":      jobID,
		"space_id":    input.SpaceID.String(),
		"case_id":     input.CaseID.String(),
		"severity":    input.Severity,
		"issue_type":  input.IssueType,
		"chunk_ids":   stringifyChunks(input.ChunkIDs),
		"requestedBy": input.RequestedBy,
	}
	body, _ := json.Marshal(payload)
	traceID := strings.TrimSpace(reqctx.GetTraceID(ctx))

	if err := p.delivery.Publish(ctx, delivery.PublishRequest{
		TenantUUID:    tenantKey,
		Topic:         fullTopic,
		EventID:       fmt.Sprintf("knowledge-reprocess-%s-%d", input.CaseID.String(), jobID),
		TraceID:       traceID,
		Version:       "v1",
		Payload:       body,
		PayloadFormat: p.format,
		Attributes: map[string]string{
			"subscriber_id": p.subscriberID,
			"space_id":      input.SpaceID.String(),
			"case_id":       input.CaseID.String(),
		},
	}); err != nil {
		return ReprocessTask{}, err
	}

	return ReprocessTask{
		JobID:        jobID,
		Status:       "scheduled",
		ScheduledAt:  p.clock(),
		RollbackHint: "previous_bundle",
	}, nil
}

func (p *EventFabricReprocessPipeline) ensureTopic(ctx context.Context, tenantKey string) error {
	namespace := strings.ToLower(strings.TrimSpace(p.namespace))
	name := strings.ToLower(strings.TrimSpace(p.name))
	existing, err := p.directory.FindTopicByFullName(ctx, tenantKey, namespace, name)
	if err != nil {
		return err
	}
	var topicID string
	if existing == nil {
		created, err := p.directory.CreateTopic(ctx, directory.CreateTopicInput{
			TenantUUID:    tenantKey,
			Namespace:     namespace,
			Name:          name,
			PayloadFormat: p.format,
			MaxRetry:      p.maxRetry,
			AckTimeoutSec: p.ackTimeout,
			VersioningMode: "any",
			Metadata: map[string]any{
				"module": "knowledge_space",
				"kind":   "feedback_reprocess",
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

	_, err = p.acl.Grant(ctx, acl.GrantRequest{
		TenantUUID:    tenantKey,
		TopicUUID:     topicID,
		PrincipalType: "service",
		PrincipalID:   p.subscriberID,
		Actions: []acl.PrincipalAction{
			acl.PrincipalActionSubscribe,
			acl.PrincipalActionPublish,
		},
		Justification: "knowledge_space feedback reprocess worker",
		OperatorID:    "knowledge-space",
	})
	return err
}

type EventFabricReprocessConsumerOptions struct {
	Delivery       delivery.Service
	DB             *gorm.DB
	VectorStore    vectorstore.Store
	Directory      *directory.DirectoryService
	ACL            *acl.ACLService
	SubscriberID   string
	Namespace      string
	Name           string
	PayloadFormat  string
	MaxRetry       int32
	AckTimeoutSec  int32
	TenantProvider func(ctx context.Context) ([]string, error)
	Interval       time.Duration
	BatchSize      int
	Clock          func() time.Time
}

// EventFabricReprocessConsumer polls Event Fabric delivery queue and acks/nacks for retry/DLQ governance.
type EventFabricReprocessConsumer struct {
	delivery       delivery.Service
	db             *gorm.DB
	vector         vectorstore.Store
	directory      *directory.DirectoryService
	acl            *acl.ACLService
	subscriberID   string
	namespace      string
	name           string
	format         string
	maxRetry       int32
	ackTimeoutSec  int32
	tenantProvider func(ctx context.Context) ([]string, error)
	interval       time.Duration
	batchSize      int
	clock          func() time.Time
	cancel         context.CancelFunc
}

func NewEventFabricReprocessConsumer(opts EventFabricReprocessConsumerOptions) *EventFabricReprocessConsumer {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 50
	}
	subscriber := strings.TrimSpace(opts.SubscriberID)
	if subscriber == "" {
		subscriber = "core.knowledge_space.reprocess"
	}
	namespace := strings.TrimSpace(opts.Namespace)
	if namespace == "" {
		namespace = "knowledge.space.feedback"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "reprocess"
	}
	format := strings.TrimSpace(opts.PayloadFormat)
	if format == "" {
		format = "json"
	}
	provider := opts.TenantProvider
	if provider == nil {
		provider = func(context.Context) ([]string, error) { return []string{"global"}, nil }
	}
	maxRetry := opts.MaxRetry
	if maxRetry <= 0 {
		maxRetry = 5
	}
	ackTimeout := opts.AckTimeoutSec
	if ackTimeout <= 0 {
		ackTimeout = 30
	}
	return &EventFabricReprocessConsumer{
		delivery:       opts.Delivery,
		db:             opts.DB,
		vector:         opts.VectorStore,
		directory:      opts.Directory,
		acl:            opts.ACL,
		subscriberID:   subscriber,
		namespace:      namespace,
		name:           name,
		format:         format,
		maxRetry:       maxRetry,
		ackTimeoutSec:  ackTimeout,
		tenantProvider: provider,
		interval:       interval,
		batchSize:      batch,
		clock:          opts.Clock,
	}
}

func (c *EventFabricReprocessConsumer) Start() {
	if c == nil || c.delivery == nil || c.db == nil || c.vector == nil || c.directory == nil || c.acl == nil {
		return
	}
	if c.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.loop(ctx)
}

func (c *EventFabricReprocessConsumer) Stop() {
	if c == nil || c.cancel == nil {
		return
	}
	c.cancel()
	c.cancel = nil
}

func (c *EventFabricReprocessConsumer) loop(ctx context.Context) {
	exec := &ReprocessWorker{db: c.db, vector: c.vector, clock: c.clock}
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
				_ = c.ensureTopic(ctx, tenantKey)
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
						jobSeq, in, decodeErr := decodeReprocessPayload(attempt.Payload)
						if decodeErr != nil {
							_, _ = c.delivery.Nack(runCtx, attempt.DeliveryUUID, c.subscriberID, decodeErr.Error())
							continue
						}
						if err := exec.run(runCtx, jobSeq, in); err != nil {
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

func decodeReprocessPayload(payload []byte) (uint64, ReprocessInput, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return 0, ReprocessInput{}, err
	}
	jobID := uint64(0)
	if v, ok := raw["job_id"].(float64); ok {
		jobID = uint64(v)
	}
	spaceID, _ := uuid.Parse(fmt.Sprintf("%v", raw["space_id"]))
	caseID, _ := uuid.Parse(fmt.Sprintf("%v", raw["case_id"]))
	severity := fmt.Sprintf("%v", raw["severity"])
	issueType := fmt.Sprintf("%v", raw["issue_type"])
	requestedBy := fmt.Sprintf("%v", raw["requestedBy"])
	var chunkIDs []uuid.UUID
	if arr, ok := raw["chunk_ids"].([]any); ok {
		for _, item := range arr {
			id, err := uuid.Parse(fmt.Sprintf("%v", item))
			if err == nil {
				chunkIDs = append(chunkIDs, id)
			}
		}
	}
	if spaceID == uuid.Nil || caseID == uuid.Nil {
		return jobID, ReprocessInput{}, fmt.Errorf("missing space_id/case_id")
	}
	return jobID, ReprocessInput{
		SpaceID:     spaceID,
		CaseID:      caseID,
		Severity:    severity,
		IssueType:   issueType,
		ChunkIDs:    chunkIDs,
		RequestedBy: requestedBy,
	}, nil
}

func (c *EventFabricReprocessConsumer) ensureTopic(ctx context.Context, tenantKey string) error {
	namespace := strings.ToLower(strings.TrimSpace(c.namespace))
	name := strings.ToLower(strings.TrimSpace(c.name))
	existing, err := c.directory.FindTopicByFullName(ctx, tenantKey, namespace, name)
	if err != nil {
		return err
	}
	var topicID string
	if existing == nil {
		created, err := c.directory.CreateTopic(ctx, directory.CreateTopicInput{
			TenantUUID:    tenantKey,
			Namespace:     namespace,
			Name:          name,
			PayloadFormat: c.format,
			MaxRetry:      c.maxRetry,
			AckTimeoutSec: c.ackTimeoutSec,
			VersioningMode: "any",
			Metadata: map[string]any{
				"module": "knowledge_space",
				"kind":   "feedback_reprocess",
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
	_, err = c.acl.Grant(ctx, acl.GrantRequest{
		TenantUUID:    tenantKey,
		TopicUUID:     topicID,
		PrincipalType: "service",
		PrincipalID:   c.subscriberID,
		Actions: []acl.PrincipalAction{
			acl.PrincipalActionSubscribe,
			acl.PrincipalActionPublish,
		},
		Justification: "knowledge_space feedback reprocess worker",
		OperatorID:    "knowledge-space",
	})
	if err != nil {
		return err
	}
	return nil
}
