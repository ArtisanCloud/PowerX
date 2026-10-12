package agent_run

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestValidateProductionRedisRealServer(t *testing.T) {
	addr := os.Getenv("POWERX_AGENT_RUN_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set POWERX_AGENT_RUN_TEST_REDIS_ADDR for a dedicated durable Redis test server")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	if err := ValidateProductionRedis(context.Background(), client); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProductionRedisFailsClosed(t *testing.T) {
	if err := ValidateProductionRedis(context.Background(), nil); err == nil {
		t.Fatal("missing Redis client passed production gate")
	}

	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	if err := ValidateProductionRedis(context.Background(), client); err == nil {
		t.Fatal("test Redis without declared durable settings passed production gate")
	}
}

func TestValidateProductionRedisRejectsUnreachable(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	err := ValidateProductionRedis(context.Background(), client)
	if err == nil || !strings.Contains(err.Error(), "ping") {
		t.Fatalf("expected Redis connectivity failure, got %v", err)
	}
}

func TestPersistenceHealthRejectsFailedWritesAndLoading(t *testing.T) {
	healthy := "loading:0\r\naof_enabled:1\r\naof_last_write_status:ok\r\naof_last_bgrewrite_status:ok\r\n"
	if err := validatePersistenceHealth(healthy); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", strings.Replace(healthy, "loading:0", "loading:1", 1), strings.Replace(healthy, "aof_enabled:1", "aof_enabled:0", 1), strings.Replace(healthy, "aof_last_write_status:ok", "aof_last_write_status:err", 1), strings.Replace(healthy, "aof_last_bgrewrite_status:ok", "aof_last_bgrewrite_status:err", 1)} {
		if err := validatePersistenceHealth(bad); err == nil {
			t.Fatal("unhealthy AOF passed startup gate")
		}
	}
}
