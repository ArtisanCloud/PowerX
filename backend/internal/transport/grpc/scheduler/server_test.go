package scheduler

import (
	"context"
	"testing"
	"time"

	schedulerv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/scheduler/v1"
	runtimescheduler "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const schedulerGRPCTestTenant = "6b5d0240-9920-46da-b707-88200e0f51ea"

func TestServerCreateAndListJobs(t *testing.T) {
	server, ctx := newTestServer(t)
	created, err := server.CreateJob(ctx, &schedulerv1.CreateJobRequest{Job: &schedulerv1.SchedulerJob{
		TenantUuid:   schedulerGRPCTestTenant,
		OwnerType:    models.OwnerTypePlugin,
		OwnerId:      "com.powerx.plugins.ai-craft",
		Name:         "grpc_job",
		ScheduleType: models.ScheduleTypeInterval,
		ScheduleExpr: "10m",
		PayloadJson:  []byte(`{"business_action":"grpc_job"}`),
	}})
	if err != nil {
		t.Fatalf("CreateJob() err = %v", err)
	}
	if created.GetJob().GetJobId() == "" {
		t.Fatalf("created job missing id: %+v", created.GetJob())
	}
	listed, err := server.ListJobs(ctx, &schedulerv1.ListJobsRequest{TenantUuid: schedulerGRPCTestTenant, Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs() err = %v", err)
	}
	if len(listed.GetJobs()) != 1 {
		t.Fatalf("len(jobs)=%d, want 1", len(listed.GetJobs()))
	}
}

func newTestServer(t *testing.T) (*Server, context.Context) {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.SchedulerJob{}, &models.SchedulerJobRun{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := runtimescheduler.NewService(runtimescheduler.Options{
		DB:    db,
		Clock: func() time.Time { return time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC) },
	})
	ctx := context.Background()
	claims := &reqctx.CoreXClaims{
		TenantUUID: schedulerGRPCTestTenant,
		MemberUUID: "member-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"plugin:com.powerx.plugins.ai-craft"},
		},
	}
	ctx = reqctx.WithClaims(ctx, claims)
	ctx = reqctx.WithTenantUUID(ctx, schedulerGRPCTestTenant)
	ctx = reqctx.WithSubject(ctx, claims.MemberUUID)
	return &Server{svc: svc}, ctx
}
