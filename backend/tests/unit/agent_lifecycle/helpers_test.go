package agentlifecycle_unit

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testResources struct {
	t          *testing.T
	db         *gorm.DB
	service    *agent_lifecycle.Service
	profiles   *agentrepo.AgentProfileLifecycleRepository
	events     *agentrepo.AgentLifecycleEventRepository
	health     *agentrepo.AgentHealthSnapshotRepository
	bus        event_bus.EventBus
	notifier   *mockNotifier
	clockMu    sync.Mutex
	currentTS  time.Time
	prevSchema string
}

func newTestResources(t *testing.T) *testResources {
	t.Helper()

	res := &testResources{
		t:          t,
		currentTS:  time.Now().UTC().Truncate(time.Millisecond),
		prevSchema: coremodel.PowerXSchema,
	}
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = res.prevSchema
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	require.NoError(t, db.AutoMigrate(
		&agentmodel.AgentProfileLifecycle{},
		&agentmodel.AgentLifecycleEventRecord{},
		&agentmodel.AgentHealthSnapshotRecord{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_health_window ON agent_health_snapshots(agent_uuid, window_started_at)").Error)
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_profile_tenant_alias_unique ON agent_profiles(tenant_uuid, alias)").Error)

	profileRepo := agentrepo.NewAgentProfileLifecycleRepository(db)
	eventRepo := agentrepo.NewAgentLifecycleEventRepository(db)
	healthRepo := agentrepo.NewAgentHealthSnapshotRepository(db)

	notifier := newMockNotifier()
	inst := agentinstr.New(agentinstr.Options{
		AlertCooldown: time.Minute,
	})
	bus := event_bus.NewLocalEventBus()

	res.db = db
	res.profiles = profileRepo
	res.events = eventRepo
	res.health = healthRepo
	res.notifier = notifier
	res.bus = bus

	service := agent_lifecycle.NewService(agent_lifecycle.ServiceOptions{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   eventRepo,
		HealthRepo:      healthRepo,
		EventBus:        bus,
		Instrumentation: inst,
		Notifier:        notifier,
		Config: agent_lifecycle.Config{
			DefaultCapacityInstances: 2,
			SubscriptionCacheTTL:     time.Minute,
			AlertCooldown:            time.Minute,
			EventTopics: agent_lifecycle.EventTopics{
				LifecyclePrefix: "agent.lifecycle",
				HealthPrefix:    "agent.health",
			},
		},
		Clock: func() time.Time {
			res.clockMu.Lock()
			defer res.clockMu.Unlock()
			return res.currentTS
		},
	})
	res.service = service

	t.Cleanup(func() {
		_ = bus.Close()
	})

	return res
}

func (r *testResources) advanceClock(d time.Duration) {
	r.clockMu.Lock()
	defer r.clockMu.Unlock()
	r.currentTS = r.currentTS.Add(d)
}

func (r *testResources) seedProfile(status string) *agentmodel.AgentProfileLifecycle {
	ctx := context.Background()
	profile := &agentmodel.AgentProfileLifecycle{
		TenantUUID:               uuid.NewString(),
		Alias:                    "agent-" + uuid.NewString()[:8],
		DisplayName:              "agent",
		Status:                   status,
		ToolGrants:               datatypes.JSON([]byte("[]")),
		TelemetryContractVersion: "otel-agent-v1",
		DefaultCapacityInstances: 2,
		EventTopicPrefix:         "agent.lifecycle.sample",
		NotificationChannel:      "",
		Metadata:                 datatypes.JSON([]byte("{}")),
	}
	created, err := r.profiles.Create(ctx, profile)
	require.NoError(r.t, err)
	return created
}

type mockNotifier struct {
	mu       sync.Mutex
	messages []imnotify.Message
}

func newMockNotifier() *mockNotifier {
	return &mockNotifier{
		messages: make([]imnotify.Message, 0),
	}
}

func (m *mockNotifier) Send(_ context.Context, msg imnotify.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockNotifier) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func (m *mockNotifier) Last() *imnotify.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return nil
	}
	msg := m.messages[len(m.messages)-1]
	return &msg
}
