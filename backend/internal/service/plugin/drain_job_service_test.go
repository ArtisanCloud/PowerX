package plugin

import (
	"context"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateDrainJobAutoDrainsWhenNoRuntimeBlockers(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	err := repo.Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:   "com.powerx.plugins.base",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    true,
		Status:     dbsetting.PluginInstanceStatusEnabled,
	})
	if err != nil {
		t.Fatalf("seed tenant plugin instance: %v", err)
	}

	svc := NewPluginDrainJobService(db)
	job, err := svc.CreateDrainJob(ctx, CreateDrainJobInput{PluginID: "com.powerx.plugins.base", Reason: "retire"})
	if err != nil {
		t.Fatalf("CreateDrainJob() err = %v", err)
	}
	if job.AffectedTenantCount != 1 || job.DrainedTenantCount != 1 || job.Status != dbsetting.PluginDrainJobStatusReadyToUninstall {
		t.Fatalf("unexpected drain job: %+v", job)
	}
	cfg, err := repo.Get(ctx, "6b5d0240-9920-46da-b707-88200e0f51ea", "com.powerx.plugins.base", reposetting.KeyClientCredentials)
	if err != nil {
		t.Fatalf("load tenant plugin instance: %v", err)
	}
	if cfg.Enabled || cfg.Status != dbsetting.PluginInstanceStatusDrained || cfg.DrainJobID != job.JobID {
		t.Fatalf("tenant instance not auto drained: %+v", cfg)
	}
	if err := svc.EnsurePluginAcceptsNewUsage(ctx, "com.powerx.plugins.base"); err == nil {
		t.Fatal("EnsurePluginAcceptsNewUsage() err = nil, want conflict")
	}
}

func TestCreateDrainJobWaitsWhenRuntimeBlockerExists(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	err := repo.Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:   "com.powerx.plugins.base",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    true,
		Status:     dbsetting.PluginInstanceStatusEnabled,
	})
	if err != nil {
		t.Fatalf("seed tenant plugin instance: %v", err)
	}
	if err := db.Exec(`INSERT INTO scheduler_jobs (owner_type, owner_id, status) VALUES (?, ?, ?)`, "plugin", "com.powerx.plugins.base", "active").Error; err != nil {
		t.Fatalf("seed scheduler blocker: %v", err)
	}

	svc := NewPluginDrainJobService(db)
	job, err := svc.CreateDrainJob(ctx, CreateDrainJobInput{PluginID: "com.powerx.plugins.base", Reason: "retire"})
	if err != nil {
		t.Fatalf("CreateDrainJob() err = %v", err)
	}
	if job.AffectedTenantCount != 1 || job.DrainedTenantCount != 0 || job.Status != dbsetting.PluginDrainJobStatusDraining {
		t.Fatalf("unexpected drain job: %+v", job)
	}
	cfg, err := repo.Get(ctx, "6b5d0240-9920-46da-b707-88200e0f51ea", "com.powerx.plugins.base", reposetting.KeyClientCredentials)
	if err != nil {
		t.Fatalf("load tenant plugin instance: %v", err)
	}
	if cfg.Enabled || cfg.Status != dbsetting.PluginInstanceStatusDrainingRequested || cfg.DrainJobID != job.JobID {
		t.Fatalf("tenant instance not marked draining: %+v", cfg)
	}
	if err := svc.EnsurePluginAcceptsNewUsage(ctx, "com.powerx.plugins.base"); err == nil {
		t.Fatal("EnsurePluginAcceptsNewUsage() err = nil, want conflict")
	}
	if _, err := svc.RequireNoActiveTenantInstances(ctx, "com.powerx.plugins.base", ""); err == nil {
		t.Fatal("RequireNoActiveTenantInstances() err = nil, want drain conflict")
	}
	if err := db.Table("scheduler_jobs").
		Where("owner_type = ? AND owner_id = ?", "plugin", "com.powerx.plugins.base").
		Update("status", "completed").Error; err != nil {
		t.Fatalf("complete scheduler blocker: %v", err)
	}
	job, err = svc.RefreshDrainJobProgress(ctx, job.JobID)
	if err != nil {
		t.Fatalf("RefreshDrainJobProgress() err = %v", err)
	}
	if job.DrainedTenantCount != 1 || job.Status != dbsetting.PluginDrainJobStatusReadyToUninstall {
		t.Fatalf("drain job did not become ready after blockers cleared: %+v", job)
	}
	if _, err := svc.RequireNoActiveTenantInstances(ctx, "com.powerx.plugins.base", ""); err != nil {
		t.Fatalf("RequireNoActiveTenantInstances() err = %v", err)
	}
}

func TestDrainJobReadDoesNotAutoAdvanceProgress(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	if err := repo.Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:   "com.powerx.plugins.base",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    false,
		Status:     dbsetting.PluginInstanceStatusDrainingRequested,
		DrainJobID: "drain-job-read-only",
	}); err != nil {
		t.Fatalf("seed tenant plugin instance: %v", err)
	}
	jobRepo := reposetting.NewPluginDrainJobRepository(db)
	if err := jobRepo.Create(ctx, &dbsetting.PluginDrainJob{
		JobID:               "drain-job-read-only",
		PluginID:            "com.powerx.plugins.base",
		Status:              dbsetting.PluginDrainJobStatusDraining,
		AffectedTenantCount: 1,
		DrainedTenantCount:  0,
	}); err != nil {
		t.Fatalf("seed drain job: %v", err)
	}

	svc := NewPluginDrainJobService(db)
	if _, err := svc.GetDrainJob(ctx, "drain-job-read-only"); err != nil {
		t.Fatalf("GetDrainJob() err = %v", err)
	}
	if _, err := svc.ListDrainJobs(ctx, "com.powerx.plugins.base", 20); err != nil {
		t.Fatalf("ListDrainJobs() err = %v", err)
	}
	cfg, err := repo.Get(ctx, "6b5d0240-9920-46da-b707-88200e0f51ea", "com.powerx.plugins.base", reposetting.KeyClientCredentials)
	if err != nil {
		t.Fatalf("load tenant plugin instance: %v", err)
	}
	if cfg.Status != dbsetting.PluginInstanceStatusDrainingRequested {
		t.Fatalf("read operation advanced tenant instance status: %+v", cfg)
	}
	job, err := jobRepo.GetByJobID(ctx, "drain-job-read-only")
	if err != nil {
		t.Fatalf("load drain job: %v", err)
	}
	if job.Status != dbsetting.PluginDrainJobStatusDraining || job.DrainedTenantCount != 0 {
		t.Fatalf("read operation advanced drain job: %+v", job)
	}
}

func TestListDrainBlockersAutoAdvancesWhenNoBlockers(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	if err := repo.Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:   "com.powerx.plugins.base",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    false,
		Status:     dbsetting.PluginInstanceStatusDrainingRequested,
		DrainJobID: "drain-job-blocker-read",
	}); err != nil {
		t.Fatalf("seed tenant plugin instance: %v", err)
	}
	jobRepo := reposetting.NewPluginDrainJobRepository(db)
	if err := jobRepo.Create(ctx, &dbsetting.PluginDrainJob{
		JobID:               "drain-job-blocker-read",
		PluginID:            "com.powerx.plugins.base",
		Status:              dbsetting.PluginDrainJobStatusDraining,
		AffectedTenantCount: 1,
		DrainedTenantCount:  0,
	}); err != nil {
		t.Fatalf("seed drain job: %v", err)
	}

	svc := NewPluginDrainJobService(db)
	page, err := svc.ListRuntimeBlockers(ctx, ListDrainBlockersInput{
		PluginID: "com.powerx.plugins.base",
		Kind:     "scheduler_job",
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListRuntimeBlockers() err = %v", err)
	}
	if page.Pagination == nil || page.Pagination.Total != 0 {
		t.Fatalf("unexpected blocker page: %+v", page)
	}
	cfg, err := repo.Get(ctx, "6b5d0240-9920-46da-b707-88200e0f51ea", "com.powerx.plugins.base", reposetting.KeyClientCredentials)
	if err != nil {
		t.Fatalf("load tenant plugin instance: %v", err)
	}
	if cfg.Enabled || cfg.Status != dbsetting.PluginInstanceStatusDrained {
		t.Fatalf("tenant instance not auto drained after blocker inspection: %+v", cfg)
	}
	job, err := jobRepo.GetByJobID(ctx, "drain-job-blocker-read")
	if err != nil {
		t.Fatalf("load drain job: %v", err)
	}
	if job.Status != dbsetting.PluginDrainJobStatusReadyToUninstall || job.DrainedTenantCount != 1 {
		t.Fatalf("drain job not advanced after blocker inspection: %+v", job)
	}
}

func TestFinalUninstallCompletesReadyDrainAndCleansDrainedInstances(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7})
	repo := reposetting.NewPluginInstanceConfigRepository(db)
	err := repo.Upsert(ctx, &dbsetting.PluginInstanceConfig{
		TenantUUID: "6b5d0240-9920-46da-b707-88200e0f51ea",
		PluginID:   "com.powerx.plugins.base",
		Key:        reposetting.KeyClientCredentials,
		Enabled:    false,
		Status:     dbsetting.PluginInstanceStatusDrained,
		DrainJobID: "drain-job-1",
	})
	if err != nil {
		t.Fatalf("seed drained tenant plugin instance: %v", err)
	}
	jobRepo := reposetting.NewPluginDrainJobRepository(db)
	err = jobRepo.Create(ctx, &dbsetting.PluginDrainJob{
		JobID:               "drain-job-1",
		PluginID:            "com.powerx.plugins.base",
		Status:              dbsetting.PluginDrainJobStatusReadyToUninstall,
		AffectedTenantCount: 1,
		DrainedTenantCount:  1,
	})
	if err != nil {
		t.Fatalf("seed drain job: %v", err)
	}

	svc := NewPluginDrainJobService(db)
	if _, err := svc.RequireNoActiveTenantInstances(ctx, "com.powerx.plugins.base", ""); err != nil {
		t.Fatalf("RequireNoActiveTenantInstances() err = %v", err)
	}
	if err := svc.CompleteFinalUninstall(ctx, "com.powerx.plugins.base"); err != nil {
		t.Fatalf("CompleteFinalUninstall() err = %v", err)
	}

	count, err := repo.CountTenantPluginBindings(ctx, reposetting.ListTenantPluginOptions{
		PluginIDs: []string{"com.powerx.plugins.base"},
		Key:       reposetting.KeyClientCredentials,
	})
	if err != nil {
		t.Fatalf("count tenant plugin bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("tenant plugin bindings count=%d, want 0", count)
	}
	job, err := jobRepo.GetByJobID(ctx, "drain-job-1")
	if err != nil {
		t.Fatalf("load drain job: %v", err)
	}
	if job.Status != dbsetting.PluginDrainJobStatusCompleted || job.CompletedAt == nil {
		t.Fatalf("drain job not completed: %+v", job)
	}
}

func newPluginDrainTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&dbsetting.PluginInstanceConfig{}, &dbsetting.PluginDrainJob{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS scheduler_jobs (
		id integer primary key autoincrement,
		uuid text,
		name text,
		owner_type text,
		owner_id text,
		status text,
		next_run_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create scheduler_jobs: %v", err)
	}
	if err := db.Exec(`CREATE TABLE IF NOT EXISTS event_task_histories (
		id integer primary key autoincrement,
		task_id text,
		metadata text,
		subscriber_id text,
		topic text,
		status text,
		error_message text,
		last_seen_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create event_task_histories: %v", err)
	}
	return db
}
