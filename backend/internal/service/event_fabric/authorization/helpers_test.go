package authorization

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testEnv struct {
	t           *testing.T
	db          *gorm.DB
	service     Service
	cache       *testCache
	alerts      *testAlerts
	rateLimiter *testRateLimiter
	audit       *testAudit
	metrics     eventmetrics.Recorder
	clock       func() time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	prevSchema := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() {
		model.PowerXSchema = prevSchema
	})

	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Ping())
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	require.NoError(t, db.AutoMigrate(
		&eventfabricmodel.AuthorizationCapability{},
		&eventfabricmodel.AuthorizationGrant{},
		&eventfabricmodel.AuthorizationGrantCapability{},
		&eventfabricmodel.AuthorizationGrantCondition{},
		&eventfabricmodel.AuthorizationApprovalTicket{},
		&eventfabricmodel.AuthorizationGrantTemplate{},
	))

	cache := newTestCache()
	alerts := &testAlerts{}
	limiter := &testRateLimiter{}
	audit := &testAudit{}
	metrics := eventmetrics.NewRecorder()
	clock := func() time.Time { return time.Now().UTC() }

	repo := eventfabricrepo.NewAuthorizationRepository(db)
	svc, err := NewService(ServiceOptions{
		Repository:   repo,
		Cache:        cache,
		ChallengeSLA: time.Hour,
		Audit:        audit,
		Metrics:      metrics,
		RateLimiter:  limiter,
		Alerts:       alerts,
		Logger:       pxlog.GetGlobalLogger(),
		Clock:        clock,
	})
	require.NoError(t, err)

	return &testEnv{
		t:           t,
		db:          db,
		service:     svc,
		cache:       cache,
		alerts:      alerts,
		rateLimiter: limiter,
		audit:       audit,
		metrics:     metrics,
		clock:       clock,
	}
}

type testCache struct {
	mu   sync.Mutex
	data map[string]*GrantCacheEntry
}

func newTestCache() *testCache {
	return &testCache{
		data: make(map[string]*GrantCacheEntry),
	}
}

func (c *testCache) Get(_ context.Context, key GrantCacheKey, expectedVersion uint64) (*GrantCacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.data[key.String()]
	if !ok {
		return nil, nil
	}
	if expectedVersion > 0 && entry.Version != expectedVersion {
		return nil, nil
	}
	if entry.ExpiresAt.Before(time.Now()) {
		delete(c.data, key.String())
		return nil, nil
	}
	copy := *entry
	copy.Payload = append([]byte(nil), entry.Payload...)
	return &copy, nil
}

func (c *testCache) Set(_ context.Context, key GrantCacheKey, value *GrantCacheEntry) error {
	if value == nil {
		return fmt.Errorf("cache entry is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := *value
	copy.Payload = append([]byte(nil), value.Payload...)
	c.data[key.String()] = &copy
	return nil
}

func (c *testCache) Invalidate(_ context.Context, key GrantCacheKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key.String())
	return nil
}

func (c *testCache) ListenInvalidations(context.Context) error {
	return nil
}

type testRateLimiter struct {
	mu       sync.Mutex
	response RateLimitResult
	err      error
	calls    []string
}

func (r *testRateLimiter) Allow(_ context.Context, key string, policy RateLimitPolicy) (RateLimitResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, fmt.Sprintf("%s:%d", key, policy.Limit))
	if r.err != nil {
		return RateLimitResult{}, r.err
	}
	if r.response == (RateLimitResult{}) {
		return RateLimitResult{Allowed: true, Remaining: -1}, nil
	}
	return r.response, nil
}

func (r *testRateLimiter) setResponse(resp RateLimitResult, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.response = resp
	r.err = err
	r.calls = nil
}

type testAlerts struct {
	mu     sync.Mutex
	events []AlertEvent
}

func (a *testAlerts) Emit(_ context.Context, event AlertEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
}

func (a *testAlerts) snapshot() []AlertEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]AlertEvent, len(a.events))
	copy(out, a.events)
	return out
}

type testAudit struct {
	mu      sync.Mutex
	records []eventaudit.Record
}

func (a *testAudit) Write(_ context.Context, record eventaudit.Record) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = append(a.records, record)
	return nil
}

func (env *testEnv) insertCapability(namespace, action string) eventfabricmodel.AuthorizationCapability {
	cap := eventfabricmodel.AuthorizationCapability{
		Namespace: namespace,
		Action:    action,
		RiskLevel: eventfabricmodel.RiskLevelLow,
	}
	require.NoError(env.t, env.db.Create(&cap).Error)
	return cap
}

func (env *testEnv) insertGrant(tenantID, subjectID uuid.UUID, subjectType, status string, ttl time.Time) eventfabricmodel.AuthorizationGrant {
	grant := eventfabricmodel.AuthorizationGrant{
		TenantID:     tenantID,
		SubjectType:  subjectType,
		SubjectID:    subjectID,
		Status:       status,
		Source:       eventfabricmodel.GrantSourceSystemTemplate,
		TTLExpiresAt: &ttl,
		Version:      1,
	}
	require.NoError(env.t, env.db.Create(&grant).Error)
	return grant
}

func (env *testEnv) insertGrantCapability(grantID, capabilityID uuid.UUID) {
	record := eventfabricmodel.AuthorizationGrantCapability{
		GrantID:      grantID,
		CapabilityID: capabilityID,
	}
	require.NoError(env.t, env.db.Create(&record).Error)
}

func (env *testEnv) insertGrantCondition(grantID uuid.UUID, condType string, expression []byte) {
	record := eventfabricmodel.AuthorizationGrantCondition{
		GrantID:    grantID,
		Type:       condType,
		Expression: expression,
	}
	require.NoError(env.t, env.db.Create(&record).Error)
}

func (env *testEnv) insertTicket(grantID uuid.UUID, status string, sla time.Time) eventfabricmodel.AuthorizationApprovalTicket {
	ticket := eventfabricmodel.AuthorizationApprovalTicket{
		TenantID:           uuid.New(),
		GrantID:            &grantID,
		RequestFingerprint: uuid.New(),
		Status:             status,
		SLAExpiresAt:       sla,
	}
	require.NoError(env.t, env.db.Create(&ticket).Error)
	return ticket
}
