package eventfabric

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
)

func BenchmarkEventFabricDeliveryThroughput(b *testing.B) {
	env := newBenchEnv(b)
	publishCtx := context.Background()

	tenantCtx := context.WithValue(context.Background(), shared.ContextTenantKey, env.tenant)
	tenantCtx = context.WithValue(tenantCtx, shared.ContextSubscriberKey, env.subscriber)
	tenantCtx = context.WithValue(tenantCtx, shared.ContextCompatibilityMode, "backward")
	tenantCtx = context.WithValue(tenantCtx, shared.ContextAcceptedVersions, []string{"v1"})

	var seq atomic.Uint64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := seq.Add(1)
			eventID := fmt.Sprintf("bench-throughput-%d", id)
			req := delivery.PublishRequest{
				TenantID:   env.tenant,
				Topic:      env.topic.FullTopic,
				EventID:    eventID,
				TraceID:    fmt.Sprintf("trace-%d", id),
				Version:    "v1",
				Payload:    []byte(`{"bench":"throughput"}`),
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
				time.Sleep(20 * time.Microsecond)
			}

			if err := env.service.Ack(publishCtx, attempt.DeliveryUUID, attempt.SubscriberID); err != nil {
				b.Fatalf("ack failed: %v", err)
			}
		}
	})
}
