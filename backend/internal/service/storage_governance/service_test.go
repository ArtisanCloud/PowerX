package storagegov

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

type fixtureCollector struct{ calls int }

func (f *fixtureCollector) Collect(context.Context, string, []Resource) Snapshot {
	f.calls++
	used, known := int64(1000), int64(100)
	return Snapshot{Filesystem: Filesystem{ID: "fs", State: "observed", UsedBytes: &used}, Resources: []Usage{{Resource: Resource{ID: "registered", Path: "/private/should-not-leak"}, FilesystemID: "fs", AttributedBytes: &known}}}
}

func TestStorageEstimateUsesExplicitAssumptionsAndNeverEnables(t *testing.T) {
	rate := int64(300)
	for _, tc := range []struct {
		days                      int
		budget, required, deficit int64
		state                     string
	}{{7, 3000, 2730, 0, "budget_feasible"}, {14, 3000, 5460, 2460, "budget_insufficient"}, {14, 6000, 5460, 0, "budget_feasible"}} {
		out, err := CalculateEstimate(EstimateInput{TargetDays: tc.days, MaxBytes: tc.budget, DailyIncrementBytes: &rate})
		if err != nil || *out.RequiredBytes != tc.required || *out.DeficitBytes != tc.deficit || out.State != tc.state || out.CanEnable || out.Basis != "manual_assumption" {
			t.Fatalf("unexpected estimate: %+v, %v", out, err)
		}
	}
	out, err := CalculateEstimate(EstimateInput{TargetDays: 7, MaxBytes: 3000})
	if err != nil || out.State != "sampling_required" || out.RequiredBytes != nil || out.SupportedDays != nil {
		t.Fatalf("missing sample became estimate: %+v", out)
	}
	protected, err := CalculateEstimate(EstimateInput{TargetDays: 7, MaxBytes: 3000, ProtectedBytes: 500, DailyIncrementBytes: &rate})
	if err != nil || *protected.RequiredBytes != 3230 || *protected.DeficitBytes != 230 {
		t.Fatalf("protection not charged: %+v", protected)
	}
}

func TestStorageEstimateRejectsOverflowAndInvalidInput(t *testing.T) {
	for _, rate := range []int64{-1, 0, math.MaxInt64, maxSafeBytes} {
		if _, err := CalculateEstimate(EstimateInput{TargetDays: 3650, MaxBytes: 3000, DailyIncrementBytes: &rate}); err == nil {
			t.Fatalf("accepted rate %d", rate)
		}
	}
	for _, in := range []EstimateInput{{TargetDays: 0, MaxBytes: 1}, {TargetDays: 1, MaxBytes: 0}, {TargetDays: 3651, MaxBytes: 1}, {TargetDays: 1, MaxBytes: 1, ProtectedBytes: -1}} {
		if _, err := CalculateEstimate(in); err == nil {
			t.Fatalf("accepted %+v", in)
		}
	}
	rate := int64(1)
	out, err := CalculateEstimate(EstimateInput{TargetDays: 1, MaxBytes: 2, DailyIncrementBytes: &rate})
	if err != nil || *out.RequiredBytes != 2 {
		t.Fatal("must round capacity up")
	}
}

func TestStorageObservationCacheAndUnknownCoverage(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	collector := &fixtureCollector{}
	s, err := NewService(Options{FilesystemPath: "/", Collector: collector, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, e := s.Overview(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if out.Mode != "observe" || out.CleanupEnabled || out.Coverage != "partial" || *out.UnattributedBytes != 900 {
			t.Fatalf("false coverage: %+v", out)
		}
		b, _ := json.Marshal(out)
		if strings.Contains(string(b), "should-not-leak") {
			t.Fatal("path leaked")
		}
	}
	if collector.calls != 1 {
		t.Fatal("unbounded refresh scans")
	}
	now = now.Add(6 * time.Minute)
	_, _ = s.Overview(context.Background())
	if collector.calls != 2 {
		t.Fatal("cache never expires")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Overview(ctx); err == nil {
		t.Fatal("cancelled request returned success")
	}
}

func TestStorageRegistrationDoesNotGuessOwner(t *testing.T) {
	valid := Resource{ID: "logs", Name: "Logs", Kind: "application_logs", Scope: "prod", InstanceID: "installed-id", Path: "/logs"}
	for _, change := range []func(*Resource){func(r *Resource) { r.InstanceID = "" }, func(r *Resource) { r.Scope = "production" }, func(r *Resource) { r.Scope = "shared" }, func(r *Resource) { r.Path = "relative" }, func(r *Resource) { r.Kind = "shell" }} {
		r := valid
		change(&r)
		if _, err := NewService(Options{FilesystemPath: "/", Collector: &fixtureCollector{}, Resources: []Resource{r}}); err == nil {
			t.Fatalf("accepted invalid source %+v", r)
		}
	}
	if _, err := NewService(Options{FilesystemPath: "/", Collector: &fixtureCollector{}, Resources: []Resource{valid, valid}}); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
}
