package capability_registry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestCacheManagerCacheCapabilityRecord(t *testing.T) {
	ctx := context.Background()
	_, client := newTestRedis(t)

	manager := NewCacheManager(CacheManagerOptions{Redis: client})
	require.NotNil(t, manager)

	record := &models.CapabilityRecord{
		CapabilityID:     "demo.capability",
		PluginID:         "demo.plugin",
		PluginVersion:    "1.0.0",
		Title:            "Demo Capability",
		Description:      "demo",
		Intents:          datatypes.JSON([]byte(`[]`)),
		ToolScope:        datatypes.JSON([]byte(`[]`)),
		Protocols:        datatypes.JSON([]byte(`[]`)),
		Policy:           datatypes.JSON([]byte(`{"prefer":"mcp"}`)),
		CapabilitiesHash: "hash-123",
		ProtocolHash:     "proto-123",
		Status:           "published",
	}

	require.NoError(t, manager.CacheCapabilityRecord(ctx, record))

	key := manager.CapabilityCacheKey(record.CapabilityID)
	raw, err := client.Get(ctx, key).Bytes()
	require.NoError(t, err)

	var cached models.CapabilityRecord
	require.NoError(t, json.Unmarshal(raw, &cached))
	require.Equal(t, record.CapabilityID, cached.CapabilityID)
	ttl, err := client.TTL(ctx, key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 2*time.Minute)
	require.LessOrEqual(t, ttl, defaultCapabilityCacheTTL)
}

func TestCacheManagerCachePolicySnapshot(t *testing.T) {
	ctx := context.Background()
	_, client := newTestRedis(t)
	manager := NewCacheManager(CacheManagerOptions{Redis: client})
	require.NotNil(t, manager)

	snapshot := map[string]interface{}{
		"tenant_id":         "tenant-001",
		"capabilities_hash": "hash-xyz",
		"intent_mappings": map[string]map[string]string{
			"demo.intent": {"default": "demo.capability"},
		},
		"generated_at": time.Unix(1700000000, 0).UTC().Format(time.RFC3339),
	}

	require.NoError(t, manager.CachePolicySnapshot(ctx, "hash-xyz", snapshot))

	key := manager.PolicyCacheKey("hash-xyz")
	raw, err := client.Get(ctx, key).Bytes()
	require.NoError(t, err)

	var cached map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &cached))
	require.Equal(t, "hash-xyz", cached["capabilities_hash"])
	ttl, err := client.TTL(ctx, key).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, 4*time.Minute)
	require.LessOrEqual(t, ttl, defaultPolicyCacheTTL)
}

func TestCacheManagerBroadcast(t *testing.T) {
	ctx := context.Background()
	_, client := newTestRedis(t)
	manager := NewCacheManager(CacheManagerOptions{Redis: client})
	require.NotNil(t, manager)

	sub := client.Subscribe(ctx, defaultBroadcastChannel)
	t.Cleanup(func() { _ = sub.Close() })
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	msg := CacheBroadcastMessage{
		Event:            "capability.updated",
		CapabilityID:     "demo.capability",
		CapabilitiesHash: "hash-abc",
		PluginID:         "demo.plugin",
	}

	require.NoError(t, manager.Broadcast(ctx, msg))

	select {
	case payload := <-sub.Channel():
		var received CacheBroadcastMessage
		require.NoError(t, json.Unmarshal([]byte(payload.Payload), &received))
		require.Equal(t, msg.Event, received.Event)
		require.Equal(t, msg.CapabilityID, received.CapabilityID)
		require.False(t, received.Timestamp.IsZero())
	case <-time.After(2 * time.Second):
		t.Fatal("expected broadcast message")
	}
}

func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return srv, client
}
