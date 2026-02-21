package toolstore

import (
	"context"
	"sync"
	"testing"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	capregistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

func TestStoreCachesSnapshots(t *testing.T) {
	gen := &fakeGenerator{
		snapshots: []capregistry.SelectorPolicySnapshot{
			newGeneratorSnapshot("tenant-corex", "hash-1"),
		},
	}
	store := NewStore(StoreOptions{
		Generator: gen,
		CacheTTL:  time.Minute,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})
	t.Cleanup(store.Close)

	ctx := context.Background()
	first, err := store.GetSnapshot(ctx, "tenant-corex", nil)
	require.NoError(t, err)
	require.Equal(t, "hash-1", first.CapabilitiesHash)

	second, err := store.GetSnapshot(ctx, "tenant-corex", nil)
	require.NoError(t, err)
	require.Equal(t, first.CapabilitiesHash, second.CapabilitiesHash)
	require.Equal(t, 1, gen.CallCount())
}

func TestStoreInvalidatesOnCatalogEvent(t *testing.T) {
	gen := &fakeGenerator{
		snapshots: []capregistry.SelectorPolicySnapshot{
			newGeneratorSnapshot("tenant-corex", "hash-1"),
			newGeneratorSnapshot("tenant-corex", "hash-2"),
		},
	}
	bus := event_bus.NewLocalEventBus()
	t.Cleanup(func() { _ = bus.Close() })

	store := NewStore(StoreOptions{
		Generator: gen,
		EventBus:  bus,
		CacheTTL:  time.Minute,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})
	t.Cleanup(store.Close)

	ctx := context.Background()
	first, err := store.GetSnapshot(ctx, "tenant-corex", nil)
	require.NoError(t, err)
	require.Equal(t, "hash-1", first.CapabilitiesHash)

	bus.Publish(eventbus.TopicCapabilityCatalogSyncSucceeded, capregistry.CatalogSyncEvent{
		CapabilityID: "demo.capability",
	}, context.Background())
	time.Sleep(50 * time.Millisecond)

	second, err := store.GetSnapshot(ctx, "tenant-corex", nil)
	require.NoError(t, err)
	require.Equal(t, "hash-2", second.CapabilitiesHash)
	require.Equal(t, 2, gen.CallCount())
}

func TestStoreRequiresTenant(t *testing.T) {
	gen := &fakeGenerator{
		snapshots: []capregistry.SelectorPolicySnapshot{
			newGeneratorSnapshot("tenant-corex", "hash-1"),
		},
	}
	store := NewStore(StoreOptions{Generator: gen})
	t.Cleanup(store.Close)

	_, err := store.GetSnapshot(context.Background(), "", nil)
	require.Error(t, err)
}

type fakeGenerator struct {
	mu        sync.Mutex
	snapshots []capregistry.SelectorPolicySnapshot
	calls     int
}

func (f *fakeGenerator) GenerateSnapshot(ctx context.Context, in capregistry.GeneratePolicyInput) (capregistry.SelectorPolicySnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.calls
	if index >= len(f.snapshots) {
		index = len(f.snapshots) - 1
	}
	f.calls++
	return f.snapshots[index], nil
}

func (f *fakeGenerator) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newGeneratorSnapshot(tenant, hash string) capregistry.SelectorPolicySnapshot {
	return capregistry.SelectorPolicySnapshot{
		TenantID:         tenant,
		CapabilitiesHash: hash,
		IntentMappings: map[string]map[string]string{
			"demo.intent": {"default": "demo.capability"},
		},
		PreferMatrix: map[string]capregistry.ProtocolPreference{
			"demo.capability": {Prefer: "mcp"},
		},
		GeneratedAt: time.Unix(1700000000, 0).UTC(),
		Metadata:    map[string]string{"source": "test"},
	}
}
