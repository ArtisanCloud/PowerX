package storagegov

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStorageEnvironmentPresetsDifferentiateCopiesDaysAndReferences(t *testing.T) {
	prod := policyOption(Resource{ID: "prod-backup", Kind: "backup_files", Scope: "prod", InstanceID: "prod"})
	dev := policyOption(Resource{ID: "dev-backup", Kind: "backup_files", Scope: "dev", InstanceID: "dev"})
	if prod.Preset.RetentionMode != "count" || prod.Preset.TargetDays != 0 || prod.Preset.RetentionCount != 2 || *prod.Preset.MaxBytes != 6<<30 || prod.Preset.SuggestedCadence != "daily" {
		t.Fatal(prod)
	}
	if dev.Preset.RetentionCount != 24 || *dev.Preset.MaxBytes != 1<<30 || dev.Preset.SuggestedCadence != "hourly" {
		t.Fatal(dev)
	}
	snapshot := policyOption(Resource{ID: "snap", Kind: "snapshot_files", Scope: "prod", InstanceID: "prod"})
	if snapshot.Preset.RetentionCount != 2 || snapshot.Preset.MinVerifiedPoints != 2 {
		t.Fatal(snapshot)
	}
	journal := policyOption(Resource{ID: "journal", Kind: "journal", Scope: "shared"})
	if journal.Preset.TargetDays != 2 || *journal.Preset.MaxBytes != 3<<30 || !journal.Preset.RequireVerifiedArchive {
		t.Fatal(journal)
	}
	archive := policyOption(Resource{ID: "cas", Kind: "archive_objects", Scope: "dev", InstanceID: "dev"})
	if archive.Preset.RetentionMode != "protected" || archive.Preset.TargetDays != 0 || archive.Preset.MaxBytes != nil {
		t.Fatal("CAS inherited a fake day/budget default", archive)
	}
}

func TestStorageEstimatesBindRegisteredEnvironmentAndDoNotApplyDailyMathToCopies(t *testing.T) {
	s, _, _ := storageStateFixture(t)
	s.opts.Resources = []Resource{{ID: "logs", Name: "Prod logs", Kind: "application_logs", Scope: "prod", InstanceID: "production-instance", Path: "/private/path"}, {ID: "backup", Name: "Dev backups", Kind: "backup_files", Scope: "dev", InstanceID: "development-instance", Path: "/private/backup"}}
	rate := int64(300)
	out, err := s.EstimateResource(EstimateInput{ResourceID: "logs", RetentionMode: "days", TargetDays: 14, MaxBytes: 3000, DailyIncrementBytes: &rate})
	if err != nil || out.Resource.Scope != "prod" || out.Resource.InstanceID != "production-instance" || out.Resource.ID != "logs" || *out.RequiredBytes != 5460 || out.CanEnable {
		t.Fatal(out, err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "/private/") {
		t.Fatal("source path leaked")
	}
	copyResult, err := s.EstimateResource(EstimateInput{ResourceID: "backup", RetentionMode: "count", TargetDays: 0, RetentionCount: 24, MaxBytes: 1000})
	if err != nil || copyResult.State != "artifact_inventory_required" || copyResult.RequiredBytes != nil || copyResult.SupportedDays != nil || copyResult.Resource.Scope != "dev" {
		t.Fatal(copyResult, err)
	}
	for _, in := range []EstimateInput{{ResourceID: "unknown", RetentionMode: "days", TargetDays: 7, MaxBytes: 1000}, {ResourceID: "backup", RetentionMode: "days", TargetDays: 7, MaxBytes: 1000}, {ResourceID: "logs", RetentionMode: "count", RetentionCount: 2, MaxBytes: 1000}, {ResourceID: "backup", RetentionMode: "count", RetentionCount: 24, MaxBytes: 1000, DailyIncrementBytes: &rate}} {
		if _, err = s.EstimateResource(in); err != ErrEstimateInvalid {
			t.Fatal("accepted mismatched source/method", in, err)
		}
	}
}

func TestStoragePolicyCatalogIsReadOnlyAndLegacyDraftsPreserveSemantics(t *testing.T) {
	s, store, ctx := storageStateFixture(t)
	s.opts.Resources = []Resource{{ID: "backup", Name: "Backups", Kind: "backup_files", Scope: "dev", InstanceID: "installed", Path: "/private/backup"}}
	p, err := s.Policy(ctx)
	if err != nil || p.CatalogVersion != 1 || len(p.Catalog) != 1 || store.policy != nil {
		t.Fatal(p, err)
	}
	in := PolicyInput{SchemaVersion: 1, State: "draft", Budget: HostBudget{MaxManagedBytes: 90, MinFreeBytes: 20, CriticalFreeBytes: 10}, Resources: []ResourcePolicy{{ResourceID: "backup", TargetDays: 9, MaxBytes: 2, RetentionCount: 12}}}
	got, err := s.SavePolicy(ctx, 1, in)
	if err != nil || got.Resources[0].TargetDays != 9 || got.Resources[0].RetentionMode != "" {
		t.Fatal("legacy silently converted", got, err)
	}
	if strings.Contains(string(store.policy.Document), "catalog") {
		t.Fatal("runtime catalog persisted into audit/document")
	}
	in.ExpectedRevision = 1
	in.Resources[0].RetentionMode = "count"
	in.Resources[0].TargetDays = 0
	in.Resources[0].RetentionCount = 24
	got, err = s.SavePolicy(ctx, 1, in)
	if err != nil || got.Resources[0].TargetDays != 0 || got.Resources[0].RetentionCount != 24 || got.CatalogVersion != 1 {
		t.Fatal(got, err)
	}
}
