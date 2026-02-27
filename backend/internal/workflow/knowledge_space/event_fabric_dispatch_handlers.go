package knowledge_space

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	eventdomain "github.com/ArtisanCloud/PowerX/internal/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"gorm.io/gorm"
)

type eventFabricRetryDispatchPayload struct {
	TenantKey     string            `json:"TenantKey"`
	SubscriberID  string            `json:"SubscriberID"`
	EventID       string            `json:"EventID"`
	DeliveryID    string            `json:"DeliveryID"`
	EnvelopeID    string            `json:"EnvelopeID"`
	Version       string            `json:"Version"`
	PayloadFormat string            `json:"PayloadFormat"`
	TraceID       string            `json:"TraceID"`
	Payload       []byte            `json:"Payload"`
	Headers       map[string]string `json:"Headers"`
	Attempt       int32             `json:"Attempt"`
	MaxAttempts   int32             `json:"MaxAttempts"`
	Remaining     int32             `json:"Remaining"`
	DispatchedAt  time.Time         `json:"DispatchedAt"`
}

type EventFabricReprocessDispatchHandlerOptions struct {
	EventBus     event_bus.EventBus
	DB           *gorm.DB
	VectorStore  vectorstore.Store
	SubscriberID string
	Clock        func() time.Time
}

func RegisterEventFabricReprocessDispatchHandler(opts EventFabricReprocessDispatchHandlerOptions) (unsubscribe func()) {
	if opts.EventBus == nil || opts.DB == nil || opts.VectorStore == nil {
		return func() {}
	}
	subscriberID := strings.TrimSpace(opts.SubscriberID)
	if subscriberID == "" {
		subscriberID = eventdomain.SubscriberKnowledgeSpaceReprocess
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	exec := &ReprocessWorker{db: opts.DB, vector: opts.VectorStore, clock: clock}

	return opts.EventBus.Subscribe("event_fabric.retry.dispatch", func(evt event_bus.Event) error {
		dispatch, err := decodeEventFabricRetryDispatchPayload(evt.Payload)
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(dispatch.SubscriberID), subscriberID) {
			return nil
		}
		if len(dispatch.Payload) == 0 {
			return nil
		}

		jobSeq, in, decodeErr := decodeReprocessPayload(dispatch.Payload)
		if decodeErr != nil {
			return decodeErr
		}

		runCtx := evt.Ctx
		if runCtx == nil {
			runCtx = context.Background()
		}
		return exec.run(runCtx, jobSeq, in)
	})
}

type EventFabricCorpusCheckDispatchHandlerOptions struct {
	EventBus     event_bus.EventBus
	DB           *gorm.DB
	SubscriberID string
	Clock        func() time.Time
}

func RegisterEventFabricCorpusCheckDispatchHandler(opts EventFabricCorpusCheckDispatchHandlerOptions) (unsubscribe func()) {
	if opts.EventBus == nil || opts.DB == nil {
		return func() {}
	}
	subscriberID := strings.TrimSpace(opts.SubscriberID)
	if subscriberID == "" {
		subscriberID = eventdomain.SubscriberKnowledgeSpaceCorpusCheck
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	consumer := &EventFabricCorpusCheckConsumer{db: opts.DB, clock: clock}

	return opts.EventBus.Subscribe("event_fabric.retry.dispatch", func(evt event_bus.Event) error {
		dispatch, err := decodeEventFabricRetryDispatchPayload(evt.Payload)
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(dispatch.SubscriberID), subscriberID) {
			return nil
		}
		if len(dispatch.Payload) == 0 {
			return nil
		}

		in, decodeErr := decodeCorpusCheckPayload(dispatch.Payload)
		if decodeErr != nil {
			return decodeErr
		}

		runCtx := evt.Ctx
		if runCtx == nil {
			runCtx = context.Background()
		}
		return consumer.run(runCtx, strings.TrimSpace(dispatch.TenantKey), in)
	})
}

func decodeEventFabricRetryDispatchPayload(raw any) (eventFabricRetryDispatchPayload, error) {
	payloadBytes, err := json.Marshal(raw)
	if err != nil {
		return eventFabricRetryDispatchPayload{}, err
	}
	var payload eventFabricRetryDispatchPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return eventFabricRetryDispatchPayload{}, err
	}
	return payload, nil
}
