package scheduler

import (
	"testing"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

func TestComputeNextRun_Basic(t *testing.T) {
	svc := NewService()
	now := time.Date(2026, 2, 12, 2, 3, 10, 0, time.UTC)
	result, err := svc.ComputeNextRun(ComputeNextRunInput{
		CronExpr: "*/5 * * * *",
		Timezone: "UTC",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("ComputeNextRun should succeed: %v", err)
	}
	if result == nil || result.NextRunAt == nil {
		t.Fatalf("next run should not be nil")
	}
	expected := time.Date(2026, 2, 12, 2, 5, 0, 0, time.UTC)
	if !result.NextRunAt.Equal(expected) {
		t.Fatalf("unexpected next run, got=%s expected=%s", result.NextRunAt.Format(time.RFC3339), expected.Format(time.RFC3339))
	}
	if result.ShouldRunNow {
		t.Fatalf("shouldRunNow should be false")
	}
}

func TestComputeNextRun_MisfireFireNow(t *testing.T) {
	svc := NewService()
	now := time.Date(2026, 2, 12, 2, 10, 0, 0, time.UTC)
	prevNextRun := time.Date(2026, 2, 12, 2, 5, 0, 0, time.UTC)
	result, err := svc.ComputeNextRun(ComputeNextRunInput{
		CronExpr:      "*/5 * * * *",
		Timezone:      "UTC",
		MisfirePolicy: eventfabricmodel.ScheduledTaskMisfireFireNow,
		Now:           now,
		PrevNextRunAt: &prevNextRun,
	})
	if err != nil {
		t.Fatalf("ComputeNextRun should succeed: %v", err)
	}
	if !result.ShouldRunNow {
		t.Fatalf("shouldRunNow should be true")
	}
	expected := time.Date(2026, 2, 12, 2, 15, 0, 0, time.UTC)
	if result.NextRunAt == nil || !result.NextRunAt.Equal(expected) {
		t.Fatalf("unexpected next run, got=%v expected=%s", result.NextRunAt, expected.Format(time.RFC3339))
	}
}

func TestComputeNextRun_MisfireSkip(t *testing.T) {
	svc := NewService()
	now := time.Date(2026, 2, 12, 2, 10, 0, 0, time.UTC)
	prevNextRun := time.Date(2026, 2, 12, 2, 5, 0, 0, time.UTC)
	result, err := svc.ComputeNextRun(ComputeNextRunInput{
		CronExpr:      "*/5 * * * *",
		Timezone:      "UTC",
		MisfirePolicy: eventfabricmodel.ScheduledTaskMisfireSkip,
		Now:           now,
		PrevNextRunAt: &prevNextRun,
	})
	if err != nil {
		t.Fatalf("ComputeNextRun should succeed: %v", err)
	}
	if result.ShouldRunNow {
		t.Fatalf("shouldRunNow should be false")
	}
	expected := time.Date(2026, 2, 12, 2, 15, 0, 0, time.UTC)
	if result.NextRunAt == nil || !result.NextRunAt.Equal(expected) {
		t.Fatalf("unexpected next run, got=%v expected=%s", result.NextRunAt, expected.Format(time.RFC3339))
	}
}

func TestComputeNextRun_MisfireCatchUp(t *testing.T) {
	svc := NewService()
	now := time.Date(2026, 2, 12, 2, 10, 0, 0, time.UTC)
	prevNextRun := time.Date(2026, 2, 12, 2, 5, 0, 0, time.UTC)
	result, err := svc.ComputeNextRun(ComputeNextRunInput{
		CronExpr:      "*/5 * * * *",
		Timezone:      "UTC",
		MisfirePolicy: eventfabricmodel.ScheduledTaskMisfireCatchUp,
		Now:           now,
		PrevNextRunAt: &prevNextRun,
	})
	if err != nil {
		t.Fatalf("ComputeNextRun should succeed: %v", err)
	}
	if !result.ShouldRunNow {
		t.Fatalf("shouldRunNow should be true")
	}
	expected := time.Date(2026, 2, 12, 2, 10, 0, 0, time.UTC)
	if result.NextRunAt == nil || !result.NextRunAt.Equal(expected) {
		t.Fatalf("unexpected next run, got=%v expected=%s", result.NextRunAt, expected.Format(time.RFC3339))
	}
}

func TestComputeNextRun_InvalidTimezone(t *testing.T) {
	svc := NewService()
	_, err := svc.ComputeNextRun(ComputeNextRunInput{
		CronExpr: "*/5 * * * *",
		Timezone: "Asia/NotExists",
		Now:      time.Now().UTC(),
	})
	if err == nil {
		t.Fatalf("expected error for invalid timezone")
	}
}
