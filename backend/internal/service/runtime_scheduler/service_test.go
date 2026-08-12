package runtimescheduler

import (
	"context"
	"fmt"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	modelsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const schedulerTestTenant = "6b5d0240-9920-46da-b707-88200e0f51ea"

func TestServiceCreateTriggerAndRuns(t *testing.T) {
	svc, ctx := newTestService(t, nil)

	job, err := svc.CreateJob(ctx, JobSpec{
		TenantUUID:     schedulerTestTenant,
		OwnerType:      models.OwnerTypePlugin,
		OwnerID:        "com.powerx.plugins.ai-craft",
		Name:           "delivery_reminder",
		ScheduleType:   models.ScheduleTypeInterval,
		ScheduleExpr:   "15m",
		Payload:        map[string]any{"business_action": "delivery_reminder", "order_id": "order-1"},
		IdempotencyKey: "tenant:plugin:delivery:order-1",
	}, "tester", "trace-1")
	if err != nil {
		t.Fatalf("CreateJob() err = %v", err)
	}
	if job.UUID.String() == "" || job.NextRunAt == nil {
		t.Fatalf("created job missing uuid or next_run_at: %+v", job)
	}
	if job.ActorMemberID != 22 || job.ActorMemberUUID != "member-1" || job.ActorUserID != 33 || job.ActorUserUUID != "user-1" {
		t.Fatalf("created job actor mismatch: %+v", job)
	}

	triggered, err := svc.TriggerJob(ctx, job.UUID.String(), "tester", "trace-trigger")
	if err != nil {
		t.Fatalf("TriggerJob() err = %v", err)
	}
	if triggered.Run == nil || triggered.Run.Status != models.RunStatusTriggered {
		t.Fatalf("unexpected trigger result: %+v", triggered)
	}
	if triggered.Run.ActorMemberID != 22 || triggered.Run.ActorMemberUUID != "member-1" || triggered.Run.ActorUserID != 33 || triggered.Run.ActorUserUUID != "user-1" {
		t.Fatalf("triggered run actor mismatch: %+v", triggered.Run)
	}

	runs, total, err := svc.ListRuns(ctx, ListRunsInput{JobID: job.UUID.String(), Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListRuns() err = %v", err)
	}
	if total != 1 || len(runs) != 1 {
		t.Fatalf("unexpected runs total=%d len=%d", total, len(runs))
	}
}

func TestServiceRejectsCreateJobWhenPluginDraining(t *testing.T) {
	svc, ctx := newTestService(t, nil)
	err := reposetting.NewPluginInstanceConfigRepository(svc.db).Upsert(ctx, &modelsetting.PluginInstanceConfig{
		TenantUUID: schedulerTestTenant,
		PluginID:   "com.powerx.plugins.ai-craft",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    false,
		Status:     modelsetting.PluginInstanceStatusDrainingRequested,
	})
	if err != nil {
		t.Fatalf("seed draining plugin instance: %v", err)
	}
	_, err = svc.CreateJob(ctx, JobSpec{
		TenantUUID:   schedulerTestTenant,
		OwnerType:    models.OwnerTypePlugin,
		OwnerID:      "com.powerx.plugins.ai-craft",
		Name:         "blocked",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "15m",
		Payload:      map[string]any{"business_action": "blocked"},
	}, "tester", "trace-blocked")
	if err == nil {
		t.Fatal("CreateJob() err = nil, want plugin draining conflict")
	}
}

func TestServiceRejectsCrossTenant(t *testing.T) {
	svc, ctx := newTestService(t, nil)
	_, err := svc.CreateJob(ctx, JobSpec{
		TenantUUID:   "1edd4132-1644-412d-abb4-d5f1e9487052",
		OwnerType:    models.OwnerTypePlugin,
		OwnerID:      "com.powerx.plugins.ai-craft",
		Name:         "cross_tenant",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "15m",
		Payload:      map[string]any{"business_action": "x"},
	}, "tester", "trace")
	if err == nil {
		t.Fatal("CreateJob() err = nil, want cross tenant rejection")
	}
}

func TestServiceRejectsCrossPluginOwner(t *testing.T) {
	svc, ctx := newTestService(t, nil)
	_, err := svc.CreateJob(ctx, JobSpec{
		TenantUUID:   schedulerTestTenant,
		OwnerType:    models.OwnerTypePlugin,
		OwnerID:      "com.powerx.plugins.other",
		Name:         "cross_plugin",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "15m",
		Payload:      map[string]any{"business_action": "x"},
	}, "tester", "trace")
	if err == nil {
		t.Fatal("CreateJob() err = nil, want cross plugin rejection")
	}
}

func TestServiceAllowsAPIKeyCallerForPluginOwner(t *testing.T) {
	svc, ctx := newTestService(t, func(c *reqctx.CoreXClaims) {
		c.MemberUUID = "apikey:7"
		c.MemberID = 1
		c.Scope = "api_key"
		c.Platforms = []string{"api_key"}
		c.RegisteredClaims.Audience = jwt.ClaimStrings{"api_key"}
	})
	_, err := svc.CreateJob(ctx, JobSpec{
		OwnerType:    models.OwnerTypePlugin,
		OwnerID:      "com.powerx.plugins.ai-craft",
		Name:         "api_key_plugin_owner",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "15m",
		Payload:      map[string]any{"business_action": "x"},
	}, "apikey:7", "trace")
	if err != nil {
		t.Fatalf("CreateJob() err = %v", err)
	}
}

func TestServiceRejectsAPIKeyCallerForCoreOwner(t *testing.T) {
	svc, ctx := newTestService(t, func(c *reqctx.CoreXClaims) {
		c.MemberUUID = "apikey:7"
		c.MemberID = 1
		c.Scope = "api_key"
		c.Platforms = []string{"api_key"}
		c.RegisteredClaims.Audience = jwt.ClaimStrings{"api_key"}
	})
	_, err := svc.CreateJob(ctx, JobSpec{
		OwnerType:    models.OwnerTypeCore,
		OwnerID:      "core.test",
		Name:         "api_key_core_owner",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "15m",
		Payload:      map[string]any{"business_action": "x"},
	}, "apikey:7", "trace")
	if err == nil {
		t.Fatal("CreateJob() err = nil, want core owner rejection")
	}
}

func TestServicePauseResume(t *testing.T) {
	svc, ctx := newTestService(t, func(c *reqctx.CoreXClaims) {
		c.IsRoot = true
		c.RegisteredClaims.Audience = jwt.ClaimStrings{"powerx:api"}
	})
	job, err := svc.CreateJob(ctx, JobSpec{
		TenantUUID:   schedulerTestTenant,
		OwnerType:    models.OwnerTypeCore,
		OwnerID:      "core.test",
		Name:         "core_job",
		ScheduleType: models.ScheduleTypeOnce,
		ScheduleExpr: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		Payload:      map[string]any{"business_action": "core"},
	}, "root", "trace")
	if err != nil {
		t.Fatalf("CreateJob() err = %v", err)
	}
	paused, err := svc.PauseJob(ctx, job.UUID.String(), "root", "trace")
	if err != nil {
		t.Fatalf("PauseJob() err = %v", err)
	}
	if paused.Status != models.JobStatusPaused {
		t.Fatalf("status=%s, want paused", paused.Status)
	}
	resumed, err := svc.ResumeJob(ctx, job.UUID.String(), "root", "trace")
	if err != nil {
		t.Fatalf("ResumeJob() err = %v", err)
	}
	if resumed.Status != models.JobStatusActive {
		t.Fatalf("status=%s, want active", resumed.Status)
	}
}

func TestServiceDispatchDueOnceJobOnlyOnce(t *testing.T) {
	now := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	var published int
	var publishedPayload map[string]any
	svc, ctx := newTestServiceWithClock(t, func() time.Time { return now }, eventBusFunc(func(topic string, payload interface{}, ctx context.Context) {
		if topic == models.TopicSchedulerTriggeredV1 {
			published++
			if typed, ok := payload.(map[string]any); ok {
				publishedPayload = typed
			}
		}
	}))
	job, err := svc.CreateJob(ctx, JobSpec{
		TenantUUID:   schedulerTestTenant,
		OwnerType:    models.OwnerTypePlugin,
		OwnerID:      "com.powerx.plugins.ai-craft",
		Name:         "once_due",
		ScheduleType: models.ScheduleTypeOnce,
		ScheduleExpr: now.Add(-time.Minute).Format(time.RFC3339),
		Payload:      map[string]any{"business_action": "once_due"},
	}, "tester", "trace")
	if err != nil {
		t.Fatalf("CreateJob() err = %v", err)
	}
	result, err := svc.DispatchDue(ctx, DispatchDueInput{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("DispatchDue() err = %v", err)
	}
	if result.DispatchedCount != 1 || published != 1 {
		t.Fatalf("dispatch=%+v published=%d", result, published)
	}
	actor, _ := publishedPayload["actor"].(map[string]any)
	if actor["type"] != "system" || actor["subject"] != "runtime_scheduler" {
		t.Fatalf("dispatch actor mismatch: %#v", actor)
	}
	result, err = svc.DispatchDue(ctx, DispatchDueInput{Now: now.Add(time.Second), Limit: 10})
	if err != nil {
		t.Fatalf("DispatchDue second() err = %v", err)
	}
	if result.DispatchedCount != 0 || published != 1 {
		t.Fatalf("second dispatch=%+v published=%d", result, published)
	}
	updated, err := svc.GetJob(ctx, job.UUID.String())
	if err != nil {
		t.Fatalf("GetJob() err = %v", err)
	}
	if updated.NextRunAt != nil {
		t.Fatalf("once job next_run_at=%v, want nil after dispatch", updated.NextRunAt)
	}
	if updated.Status != models.JobStatusCompleted {
		t.Fatalf("once job status=%s, want completed", updated.Status)
	}
	if _, err := svc.ResumeJob(ctx, job.UUID.String(), "tester", "trace"); err == nil {
		t.Fatal("ResumeJob() err = nil, want completed job rejection")
	}
}

func newTestService(t *testing.T, mutateClaims func(*reqctx.CoreXClaims)) (*Service, context.Context) {
	return newTestServiceWithClockAndClaims(t, func() time.Time {
		return time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	}, nil, mutateClaims)
}

func newTestServiceWithClock(t *testing.T, clock func() time.Time, bus event_bus.EventBus) (*Service, context.Context) {
	return newTestServiceWithClockAndClaims(t, clock, bus, nil)
}

func newTestServiceWithClockAndClaims(t *testing.T, clock func() time.Time, bus event_bus.EventBus, mutateClaims func(*reqctx.CoreXClaims)) (*Service, context.Context) {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&parseTime=true&_loc=UTC", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SchedulerJob{}, &models.SchedulerJobRun{}, &modelsetting.PluginInstanceConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := NewService(Options{DB: db, Clock: clock, EventBus: bus})
	claims := &reqctx.CoreXClaims{
		TenantUUID: schedulerTestTenant,
		MemberUUID: "member-1",
		MemberID:   22,
		UserUUID:   "user-1",
		UserID:     33,
		PluginID:   "com.powerx.plugins.ai-craft",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	}
	if mutateClaims != nil {
		mutateClaims(claims)
	}
	ctx := reqctx.WithClaims(context.Background(), claims)
	ctx = reqctx.WithTenantUUID(ctx, schedulerTestTenant)
	ctx = reqctx.WithSubject(ctx, claims.MemberUUID)
	ctx = reqctx.WithMemberID(ctx, claims.MemberID)
	ctx = reqctx.WithMemberUUID(ctx, claims.MemberUUID)
	ctx = reqctx.WithUserID(ctx, claims.UserID)
	ctx = reqctx.WithUserUUID(ctx, claims.UserUUID)
	return svc, ctx
}

type eventBusFunc func(topic string, payload interface{}, ctx context.Context)

func (f eventBusFunc) Publish(topic string, payload interface{}, ctx context.Context) {
	f(topic, payload, ctx)
}

func (eventBusFunc) Subscribe(topic string, handler event_bus.Handler) func() {
	return func() {}
}

func (eventBusFunc) Close() error { return nil }
