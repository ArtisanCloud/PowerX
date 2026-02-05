package capabilityregistryload

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	eventtopics "github.com/ArtisanCloud/PowerX/internal/eventbus"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
)

func TestCapabilityRecordCacheServesFiveThousandReads(t *testing.T) {
	env := newCapabilityCacheEnv(t)
	ctx := env.ctx

	repo := caprepo.NewCapabilityRecordRepository(env.db, env.redisClient)

	record := &models.CapabilityRecord{
		CapabilityID:     "cap.load.cache",
		PluginID:         "plugin.load",
		PluginVersion:    "1.0.0",
		Title:            "Cache Load Demo",
		Description:      "ensure redis cache survives stampede",
		Categories:       datatypes.JSON([]byte(`["demo"]`)),
		Intents:          datatypes.JSON([]byte(`["intent.load"]`)),
		ToolScope:        datatypes.JSON([]byte(`["default"]`)),
		Protocols:        datatypes.JSON([]byte(`["mcp"]`)),
		Policy:           datatypes.JSON([]byte(`{"prefer":"mcp","fallback":["grpc"]}`)),
		CapabilitiesHash: "hash-cache-demo",
		ProtocolHash:     "proto-cache-demo",
		Status:           "published",
	}

	_, err := repo.Upsert(ctx, record)
	require.NoError(t, err)

	// 模拟数据库数据被清除，但缓存仍然可用，从而验证缓存击穿保护。
	require.NoError(t, env.db.WithContext(ctx).
		Where("capability_id = ?", record.CapabilityID).
		Delete(&models.CapabilityRecord{}).Error)

	const totalReads = 5500
	eg, egCtx := errgroup.WithContext(ctx)
	for i := 0; i < totalReads; i++ {
		eg.Go(func() error {
			rec, err := repo.GetByCapabilityID(egCtx, record.CapabilityID)
			if err != nil {
				return err
			}
			if rec.CapabilityID != record.CapabilityID {
				return fmt.Errorf("unexpected capability id %s", rec.CapabilityID)
			}
			return nil
		})
	}
	require.NoError(t, eg.Wait(), "cache should serve all reads even if DB row is gone")

	// 再次确认数据库中已经没有记录，证明读取完全来自 Redis 缓存。
	count, err := repo.Count(ctx, caprepo.CapabilityRecordFilter{
		CapabilityIDs: []string{record.CapabilityID},
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), count, "db row should remain deleted to prove cache served the load")
}

func TestSelectorChaosFallbackLoad(t *testing.T) {
	ctx := context.Background()
	bus := event_bus.NewLocalEventBus()
	t.Cleanup(func() { _ = bus.Close() })

	var failureEvents atomic.Int64
	unsub := bus.Subscribe(eventtopics.TopicIntegrationGatewayInvocationFailed, func(evt event_bus.Event) error {
		failureEvents.Add(1)
		return nil
	})
	t.Cleanup(unsub)

	snapshot := capservice.SelectorPolicySnapshot{
		TenantID:         "6e7f8091-6666-4f4f-9a9a-777788889999",
		CapabilitiesHash: "hash-chaos",
		IntentMappings: map[string]map[string]string{
			"intent.chaos": {"default": "cap.chaos"},
		},
		PreferMatrix: map[string]capservice.ProtocolPreference{
			"cap.chaos": {Prefer: "mcp", Fallback: []string{"grpc"}},
		},
		GeneratedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	store := capservice.SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (capservice.SelectorPolicySnapshot, error) {
		return snapshot, nil
	})

	invoker := &chaosInvoker{
		fallbackEvery: 5,
		failEvery:     37,
	}

	selector := capservice.NewSelector(capservice.SelectorOptions{
		Store:    store,
		Invoker:  invoker,
		EventBus: bus,
		Metrics:  capmetrics.NewCapabilityRegistryMetrics(nil),
	})

	type outcome struct {
		fallback    bool
		err         error
		selectorErr bool
	}

	const totalInvocations = 6000
	results := make(chan outcome, totalInvocations)
	const invalidIntentEvery = 113
	var wg sync.WaitGroup
	wg.Add(totalInvocations)
	for i := 0; i < totalInvocations; i++ {
		go func(seq int) {
			defer wg.Done()
			intent := "intent.chaos"
			expectSelectorErr := false
			if seq%invalidIntentEvery == 0 {
				intent = "intent.invalid"
				expectSelectorErr = true
			}
			resp, err := selector.Invoke(ctx, capservice.CapabilityInvokeRequest{
				TenantUUID: "6e7f8091-6666-4f4f-9a9a-777788889999",
				Intent:     intent,
				ToolScope:  "default",
				Context: map[string]interface{}{
					"intent":     intent,
					"tool_scope": "default",
				},
				Payload: map[string]interface{}{"seq": seq},
			})
			if err != nil {
				results <- outcome{err: err, selectorErr: expectSelectorErr}
				return
			}
			results <- outcome{fallback: resp.FallbackUsed}
		}(i)
	}
	wg.Wait()
	close(results)

	var fallbackCount, errorCount, selectorErrorCount int
	for res := range results {
		if res.err != nil {
			errorCount++
			if res.selectorErr {
				selectorErrorCount++
				require.ErrorIs(t, res.err, capservice.ErrSelectorCapabilityRequired)
			}
			continue
		}
		if res.fallback {
			fallbackCount++
		}
	}

	require.Greater(t, invoker.invocations.Load(), int64(totalInvocations*3/4), "majority of requests should reach the invoker")
	require.Greater(t, fallbackCount, 0, "chaos invoker should trigger fallback at scale")
	require.Equal(t, int64(selectorErrorCount), failureEvents.Load(), "selector-level errors should emit failure events")
	require.Less(t, errorCount, totalInvocations/4, "chaos errors should remain bounded")
}

type capabilityCacheEnv struct {
	ctx         context.Context
	db          *gorm.DB
	redisServer *miniredis.Miniredis
	redisClient redis.UniversalClient
}

func newCapabilityCacheEnv(t *testing.T) *capabilityCacheEnv {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(&models.CapabilityRecord{}))

	testutil.SkipIfNoLocalListener(t)
	redisSrv, err := miniredis.Run()
	require.NoError(t, err)

	redisClient := redis.NewClient(&redis.Options{Addr: redisSrv.Addr()})

	env := &capabilityCacheEnv{
		ctx:         ctx,
		db:          db,
		redisServer: redisSrv,
		redisClient: redisClient,
	}
	t.Cleanup(env.Close)
	return env
}

func (e *capabilityCacheEnv) Close() {
	if e.redisClient != nil {
		_ = e.redisClient.Close()
	}
	if e.redisServer != nil {
		e.redisServer.Close()
	}
	if e.db != nil {
		sqlDB, err := e.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

type chaosInvoker struct {
	invocations   atomic.Int64
	fallbackEvery int64
	failEvery     int64
}

func (c *chaosInvoker) Invoke(ctx context.Context, in capservice.InvocationInput) (capservice.InvocationResult, error) {
	seq := c.invocations.Add(1)

	if c.failEvery > 0 && seq%c.failEvery == 0 {
		return capservice.InvocationResult{
			TraceID: fmt.Sprintf("trace-%d", seq),
			Status:  "failed",
		}, errors.New("primary transport timeout")
	}

	result := capservice.InvocationResult{
		TraceID: fmt.Sprintf("trace-%d", seq),
		Status:  "completed",
		Result: map[string]interface{}{
			"seq": seq,
		},
	}

	if c.fallbackEvery > 0 && seq%c.fallbackEvery == 0 {
		result.ProtocolUsed = "grpc"
		result.FallbackUsed = true
		result.Result["fallback"] = true
		return result, nil
	}

	result.ProtocolUsed = strings.TrimSpace(in.PreferredProtocol)
	if result.ProtocolUsed == "" {
		result.ProtocolUsed = "mcp"
	}
	return result, nil
}
