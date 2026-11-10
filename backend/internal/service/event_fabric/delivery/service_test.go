package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
)

func TestPublishRejectsDrainingPluginBeforeQueueSchedule(t *testing.T) {
	drainingErr := errors.New("plugin is draining")
	queue := &reliableQueueStub{}
	svc, err := NewService(Options{
		Scheduler:  NewBackoffScheduler(queue),
		Envelopes:  &envelopeStoreStub{},
		Deliveries: &deliveryStoreStub{},
		Topics:     &topicStoreStub{},
		ACL:        &aclStoreStub{},
		PluginUsageGuard: func(_ context.Context, pluginID string) error {
			if pluginID != "com.powerx.plugins.base" {
				t.Fatalf("unexpected plugin id: %s", pluginID)
			}
			return drainingErr
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.Publish(context.Background(), PublishRequest{
		TenantUUID: "tenant-demo",
		Topic:      "tenant-demo.orders.created",
		Attributes: map[string]string{
			"plugin_id": "com.powerx.plugins.base",
		},
	})
	if !errors.Is(err, drainingErr) {
		t.Fatalf("expected draining error, got %v", err)
	}
	if queue.scheduleCount != 0 {
		t.Fatalf("expected queue schedule not called, count=%d", queue.scheduleCount)
	}
}

type reliableQueueStub struct {
	scheduleCount int
}

type serviceReceiptStore struct {
	deliveryStoreStub
	id uuid.UUID
}

func (s *serviceReceiptStore) FindByUUID(context.Context, uuid.UUID) (*eventfabricmodel.DeliveryAttempt, error) {
	row := &eventfabricmodel.DeliveryAttempt{TenantKey: "tenant-demo", EventID: "efs1:isolated", SubscriberID: "com.powerx.plugins.scrm"}
	row.UUID = s.id
	return row, nil
}

type serviceReceiptQueue struct {
	reliableQueueStub
	id uuid.UUID
}

func (q *serviceReceiptQueue) PopDueRetries(context.Context, string, time.Time, int) ([]event_bus.RetryItem, error) {
	return []event_bus.RetryItem{{TenantKey: "tenant-demo", Metadata: map[string]string{"attempt_uuid": q.id.String()}}}, nil
}

func TestLegacyWorkerCannotClaimOrSettleServiceReceipts(t *testing.T) {
	id := uuid.New()
	svc, err := NewService(Options{Scheduler: NewBackoffScheduler(&serviceReceiptQueue{id: id}), Envelopes: &envelopeStoreStub{}, Deliveries: &serviceReceiptStore{id: id}, Topics: &topicStoreStub{}, ACL: &aclStoreStub{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), shared.ContextTenantKey, "tenant-demo")
	if !errors.Is(svc.Ack(ctx, id.String(), "com.powerx.plugins.scrm"), shared.ErrUnauthorized) {
		t.Fatal("legacy ACK accepted service receipt")
	}
	if _, err = svc.Nack(ctx, id.String(), "com.powerx.plugins.scrm", "test"); !errors.Is(err, shared.ErrUnauthorized) {
		t.Fatal("legacy NACK accepted service receipt")
	}
	items, err := svc.PollRetry(ctx, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("legacy worker claimed service event: %v %v", items, err)
	}
}

func (q *reliableQueueStub) ScheduleRetry(context.Context, event_bus.RetryItem) error {
	q.scheduleCount++
	return nil
}

func (q *reliableQueueStub) PopDueRetries(context.Context, string, time.Time, int) ([]event_bus.RetryItem, error) {
	return nil, nil
}

func (q *reliableQueueStub) RemoveRetry(context.Context, event_bus.RetryItem) error {
	return nil
}

func (q *reliableQueueStub) AcquireLease(context.Context, event_bus.DeliveryLease) (bool, error) {
	return true, nil
}

func (q *reliableQueueStub) ReleaseLease(context.Context, event_bus.DeliveryLease) error {
	return nil
}

type envelopeStoreStub struct{}

func (s *envelopeStoreStub) UpsertByEventID(context.Context, *eventfabricmodel.EventEnvelope) (*eventfabricmodel.EventEnvelope, bool, error) {
	return nil, false, nil
}

func (s *envelopeStoreStub) FindByEventID(context.Context, string, string) (*eventfabricmodel.EventEnvelope, error) {
	return nil, nil
}

func (s *envelopeStoreStub) FindByUUID(context.Context, uuid.UUID) (*eventfabricmodel.EventEnvelope, error) {
	return nil, nil
}

func (s *envelopeStoreStub) UpdateStatus(context.Context, uuid.UUID, map[string]interface{}) error {
	return nil
}

type deliveryStoreStub struct{}

func (s *deliveryStoreStub) UpsertAttempt(context.Context, *eventfabricmodel.DeliveryAttempt) (*eventfabricmodel.DeliveryAttempt, error) {
	return nil, nil
}

func (s *deliveryStoreStub) FindByEnvelopeAndSubscriber(context.Context, uuid.UUID, string) (*eventfabricmodel.DeliveryAttempt, error) {
	return nil, nil
}

func (s *deliveryStoreStub) FindByUUID(context.Context, uuid.UUID) (*eventfabricmodel.DeliveryAttempt, error) {
	return nil, nil
}

func (s *deliveryStoreStub) UpdateStatus(context.Context, uuid.UUID, map[string]interface{}) error {
	return nil
}

func (s *deliveryStoreStub) CountActiveAttempts(context.Context, uuid.UUID) (int64, error) {
	return 0, nil
}

type topicStoreStub struct{}

func (s *topicStoreStub) FindByComposite(context.Context, string, string, string) (*eventfabricmodel.TopicDefinition, error) {
	return nil, nil
}

func (s *topicStoreStub) FindByUUID(context.Context, uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	return nil, nil
}

type aclStoreStub struct{}

func (s *aclStoreStub) HasPermission(context.Context, string, uuid.UUID, string, string, time.Time) (bool, error) {
	return true, nil
}

func (s *aclStoreStub) ListByTopic(context.Context, string, uuid.UUID) ([]*eventfabricmodel.AclBinding, error) {
	return nil, nil
}
