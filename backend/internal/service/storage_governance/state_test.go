package storagegov

import (
	"context"
	"encoding/json"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
	"testing"
	"time"
)

type memoryState struct {
	policy  *m.Policy
	samples []m.Sample
	fail    bool
}

func (f *memoryState) GetPolicy(context.Context, string) (*m.Policy, error) {
	if f.fail {
		return nil, ErrStateUnavailable
	}
	if f.policy == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.policy, nil
}
func (f *memoryState) SavePolicy(_ context.Context, p *m.Policy, rev int64) error {
	if f.fail {
		return ErrStateUnavailable
	}
	if (f.policy == nil && rev != 0) || (f.policy != nil && f.policy.Revision != rev) {
		return repo.ErrRevisionConflict
	}
	f.policy = p
	return nil
}
func (f *memoryState) SaveSample(_ context.Context, p *m.Sample) error {
	if f.fail {
		return ErrStateUnavailable
	}
	f.samples = append(f.samples, *p)
	return nil
}
func (f *memoryState) Samples(context.Context, string, time.Time) ([]m.Sample, error) {
	if f.fail {
		return nil, ErrStateUnavailable
	}
	return f.samples, nil
}

func storageStateFixture(t *testing.T) (*Service, *memoryState, context.Context) {
	t.Helper()
	s, err := NewService(Options{Identity: Identity{HostID: "host", FilesystemID: "disk", InstanceID: "installed", Environment: "dev"}, FilesystemPath: "/", Collector: &fixtureCollector{}})
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryState{}
	s.store = store
	ctx := reqctx.WithIsRoot(context.Background(), true)
	ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: 1, IsRoot: true})
	return s, store, ctx
}

func TestStorageDraftRevisionValidationAndLegacyIsolation(t *testing.T) {
	s, store, ctx := storageStateFixture(t)
	in := PolicyInput{SchemaVersion: 1, State: "draft", Budget: HostBudget{MaxManagedBytes: 90, MinFreeBytes: 20, CriticalFreeBytes: 10}}
	initial, err := s.Policy(ctx)
	if err != nil || initial.Revision != 0 {
		t.Fatal(initial, err)
	}
	got, err := s.SavePolicy(ctx, 1, in)
	if err != nil || got.Revision != 1 || got.CleanupEnabled || got.LegacyPoliciesChanged {
		t.Fatal(got, err)
	}
	if _, err = s.SavePolicy(ctx, 1, in); err != ErrPolicyConflict {
		t.Fatal("lost update", err)
	}
	in.ExpectedRevision = 1
	in.State = "enabled"
	if _, err = s.SavePolicy(ctx, 1, in); err != ErrPolicyInvalid {
		t.Fatal("enabled cleanup", err)
	}
	in.State = "draft"
	in.Resources = []ResourcePolicy{{ResourceID: "unknown", TargetDays: 7, MaxBytes: 3}}
	if _, err = s.SavePolicy(ctx, 1, in); err != ErrPolicyInvalid {
		t.Fatal("unknown source accepted", err)
	}
	if _, err = s.SavePolicy(context.Background(), 1, in); err != ErrRootRequired {
		t.Fatal("missing user accepted", err)
	}
	store.fail = true
	if _, err = s.Policy(ctx); err != ErrStateUnavailable {
		t.Fatal("DB error became empty policy", err)
	}
}

func TestStorageHistoryNeverInfersIngressFromNetUsage(t *testing.T) {
	s, store, ctx := storageStateFixture(t)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	s.opts.Now = func() time.Time { return now }
	for _, entry := range []struct {
		delta   time.Duration
		version string
		used    int64
	}{{-10 * time.Minute, SampleMetricVersion, 900}, {-5 * time.Minute, SampleMetricVersion, 100}, {-15 * time.Minute, "old", 100}} {
		observed := now.Add(entry.delta)
		raw, _ := json.Marshal(Overview{Identity: s.opts.Identity, ObservedAt: observed, Filesystem: Filesystem{UsedBytes: &entry.used}, Coverage: "partial"})
		store.samples = append(store.samples, m.Sample{Slot: observed, MetricVersion: entry.version, Document: raw})
	}
	out, err := s.History(ctx)
	if err != nil || out.SampleCount != 2 || out.MissingSamples != 4030 || out.MetricChanges != 1 || out.DailyIncrementBytes != nil || out.RateBasis != "ingress_not_instrumented" || out.State != "occupancy_only" {
		t.Fatal(out, err)
	}
	if err = s.CaptureSample(ctx); err != nil {
		t.Fatal(err)
	}
	if len(store.samples) != 4 || store.samples[3].MetricVersion != SampleMetricVersion {
		t.Fatal("sample not captured")
	}
}

func TestStorageCapacityThresholdsAndUnknown(t *testing.T) {
	total, used, available := int64(100<<30), int64(90<<30), int64(10<<30)
	out := EvaluateCapacity(Overview{Filesystem: Filesystem{State: "observed", TotalBytes: &total, UsedBytes: &used, AvailableBytes: &available}})
	if len(out) != 2 || out[0].Level != "critical" || out[1].Level != "critical" {
		t.Fatal(out)
	}
	out = EvaluateCapacity(Overview{})
	if len(out) != 1 || out[0].Level != "unknown" {
		t.Fatal("unknown became healthy", out)
	}
}
