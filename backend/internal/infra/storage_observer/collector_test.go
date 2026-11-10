package storageobserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
)

func fixture(t *testing.T) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func source(id, path string) storagegov.Resource {
	return storagegov.Resource{ID: id, Name: id, Kind: "backup_files", Scope: "shared", Path: path}
}

func TestStorageCollectorDeduplicatesHardlinksAndOverlappingSources(t *testing.T) {
	root := fixture(t)
	file := filepath.Join(root, "payload")
	if err := os.WriteFile(file, make([]byte, 65536), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(root, "same-inode")); err != nil {
		t.Fatal(err)
	}
	out := (Collector{}).Collect(context.Background(), root, []storagegov.Resource{source("directory", root), source("overlap", file)})
	if out.Filesystem.State != "observed" {
		t.Fatalf("missing filesystem %+v", out.Filesystem)
	}
	if len(out.Resources) != 2 || out.Resources[0].State != "observed" || out.Resources[0].Files != 1 || *out.Resources[0].LogicalBytes != 65536 || *out.Resources[1].AttributedBytes != 0 {
		t.Fatalf("duplicate accounting: %+v", out.Resources)
	}
	if *out.Filesystem.UsedBytes+*out.Filesystem.AvailableBytes+*out.Filesystem.ReservedBytes != *out.Filesystem.TotalBytes {
		t.Fatal("filesystem ledger does not reconcile")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("collector changed fixture")
	}
}

func TestStorageCollectorBoundsAndMissingSourcesAreUnknown(t *testing.T) {
	root := fixture(t)
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := (Collector{MaxEntries: 2}).Collect(context.Background(), root, []storagegov.Resource{source("bounded", root), source("missing", filepath.Join(root, "absent"))})
	if out.Resources[0].State != "partial" || out.Resources[0].PhysicalBytes != nil || out.Resources[0].AttributedBytes != nil {
		t.Fatal("partial scan reported complete")
	}
	missing := (Collector{}).Collect(context.Background(), root, []storagegov.Resource{source("missing", filepath.Join(root, "absent"))}).Resources[0]
	if missing.State != "unavailable" || missing.PhysicalBytes != nil || missing.Problem != "source_missing" {
		t.Fatal("missing source reported zero")
	}
}

func TestStorageCollectorNeverFollowsSymlinks(t *testing.T) {
	root, outside := fixture(t), fixture(t)
	if err := os.WriteFile(filepath.Join(outside, "outside-data"), make([]byte, 100000), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "foreign")); err != nil {
		t.Fatal(err)
	}
	out := (Collector{}).Collect(context.Background(), root, []storagegov.Resource{source("nested", root), source("ancestor", filepath.Join(root, "foreign", "outside-data"))})
	if out.Resources[0].State != "partial" || out.Resources[0].Files != 0 || out.Resources[1].State != "unavailable" || out.Resources[1].Problem != "source_symlink" {
		t.Fatalf("symlink boundary failed: %+v", out.Resources)
	}
}

func TestStorageCollectorSharesUnusedBudgetWithLaterSources(t *testing.T) {
	root := fixture(t)
	for _, dir := range []string{"empty", "snapshots"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		if err := os.WriteFile(filepath.Join(root, "snapshots", fmt.Sprint(i)), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := (Collector{MaxEntries: 10}).Collect(context.Background(), root, []storagegov.Resource{source("empty", filepath.Join(root, "empty")), source("snapshots", filepath.Join(root, "snapshots"))})
	if out.Resources[1].State != "observed" || out.Resources[1].Files != 6 {
		t.Fatalf("unused scan budget lost: %+v", out.Resources[1])
	}
}

func TestStorageCollectorReportsPermissionFailureSeparately(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := fixture(t)
	if err := os.Chmod(root, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	out := (Collector{}).Collect(context.Background(), root, []storagegov.Resource{source("private", root)})
	if out.Resources[0].Problem != "source_permission_denied" || out.Resources[0].AttributedBytes != nil {
		t.Fatalf("wrong failure: %+v", out.Resources[0])
	}
}
