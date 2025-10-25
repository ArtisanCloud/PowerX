package eventfabric

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
)

func BenchmarkEventFabricDeliveryLatency(b *testing.B) {
	env := newBenchEnv(b)
	publishCtx := context.Background()

	tenantCtx := context.WithValue(context.Background(), shared.ContextTenantKey, env.tenant)
	tenantCtx = context.WithValue(tenantCtx, shared.ContextSubscriberKey, env.subscriber)
	tenantCtx = context.WithValue(tenantCtx, shared.ContextCompatibilityMode, "backward")
	tenantCtx = context.WithValue(tenantCtx, shared.ContextAcceptedVersions, []string{"v1"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eventID := fmt.Sprintf("bench-latency-%d", i)
		req := delivery.PublishRequest{
			TenantID:   env.tenant,
			Topic:      env.topic.FullTopic,
			EventID:    eventID,
			TraceID:    fmt.Sprintf("trace-%d", i),
			Version:    "v1",
			Payload:    []byte(`{"bench":"latency"}`),
			Attributes: map[string]string{"principal_id": "svc-bench-publisher"},
		}
		if err := env.service.Publish(publishCtx, req); err != nil {
			b.Fatalf("publish failed: %v", err)
		}

		var (
			attempt delivery.DeliveryAttempt
			ok      bool
			err     error
		)
		for {
			attempt, ok, err = env.pollOnce(tenantCtx)
			if err != nil {
				b.Fatalf("poll retry failed: %v", err)
			}
			if ok {
				break
			}
			time.Sleep(50 * time.Microsecond)
		}

		if err := env.service.Ack(publishCtx, attempt.DeliveryUUID, attempt.SubscriberID); err != nil {
			b.Fatalf("ack failed: %v", err)
		}
	}
}
