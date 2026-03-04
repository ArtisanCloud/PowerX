package capability_registry

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPolicyGeneratorGenerateSnapshot(t *testing.T) {
	ctx := context.Background()
	db := newPolicyMemoryDB(t)
	client := newTestRedisClient(t)
	cache := NewCacheManager(CacheManagerOptions{Redis: client})

	record := &models.CapabilityRecord{
		CapabilityID:     "demo.capability",
		PluginID:         "demo.plugin",
		PluginVersion:    "1.0.0",
		Title:            "Demo Capability",
		Intents:          datatypes.JSON([]byte(`["demo.intent"]`)),
		ToolScope:        datatypes.JSON([]byte(`["global"]`)),
		Policy:           datatypes.JSON([]byte(`{"prefer":"mcp","fallback":["grpc"]}`)),
		Protocols:        datatypes.JSON([]byte(`[]`)),
		Status:           "published",
		CapabilitiesHash: "hash-demo",
		ProtocolHash:     "proto-demo",
	}
	require.NoError(t, db.Create(record).Error)

	sub := client.Subscribe(ctx, defaultBroadcastChannel)
	t.Cleanup(func() { _ = sub.Close() })
	_, err := sub.Receive(ctx)
	require.NoError(t, err)

	gen := NewPolicyGenerator(PolicyGeneratorOptions{
		RecordRepo: repo.NewCapabilityRecordRepository(db, nil),
		Cache:      cache,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})

	snapshot, err := gen.GenerateSnapshot(ctx, GeneratePolicyInput{TenantUUID: "tenant-corex"})
	require.NoError(t, err)
	require.Equal(t, "tenant-corex", snapshot.TenantID)
	require.False(t, snapshot.GeneratedAt.IsZero())
	require.Equal(t, "demo.capability", snapshot.IntentMappings["demo.intent"]["global"])
	require.Equal(t, "mcp", snapshot.PreferMatrix["demo.capability"].Prefer)
	require.ElementsMatch(t, []string{"grpc"}, snapshot.PreferMatrix["demo.capability"].Fallback)
	require.NotEmpty(t, snapshot.CapabilitiesHash)

	key := cache.PolicyCacheKey(snapshot.CapabilitiesHash)
	raw, err := client.Get(ctx, key).Bytes()
	require.NoError(t, err)
	var cached SelectorPolicySnapshot
	require.NoError(t, json.Unmarshal(raw, &cached))
	require.Equal(t, snapshot.CapabilitiesHash, cached.CapabilitiesHash)

	select {
	case msg := <-sub.Channel():
		var payload CacheBroadcastMessage
		require.NoError(t, json.Unmarshal([]byte(msg.Payload), &payload))
		require.Equal(t, "selector.policy.generated", payload.Event)
		require.Equal(t, snapshot.CapabilitiesHash, payload.PolicyHash)
		require.Equal(t, "tenant-corex", payload.TenantUUID)
	case <-time.After(time.Second):
		t.Fatal("expected broadcast message")
	}
}

func TestPolicyGeneratorNoCapabilities(t *testing.T) {
	ctx := context.Background()
	db := newPolicyMemoryDB(t)
	gen := NewPolicyGenerator(PolicyGeneratorOptions{
		RecordRepo: repo.NewCapabilityRecordRepository(db, nil),
	})

	_, err := gen.GenerateSnapshot(ctx, GeneratePolicyInput{TenantUUID: "tenant-corex"})
	require.Error(t, err)
}

func newPolicyMemoryDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	require.NoError(t, db.AutoMigrate(&models.CapabilityRecord{}))
	return db
}

func newTestRedisClient(t *testing.T) redis.UniversalClient {
	testutil.SkipIfNoLocalListener(t)
	srv, err := miniredis.Run()
	if err != nil {
		t.Skipf("miniredis unavailable: %v", err)
	}
	t.Cleanup(srv.Close)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}
