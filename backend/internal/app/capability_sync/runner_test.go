package capability_sync

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestDiscoverArtifactPathsIncludesInstalledVersionDirectories(t *testing.T) {
	root := t.TempDir()

	mustWriteFile(t, filepath.Join(root, "com.powerx.plugins.scrm", "0.1.0", "plugin.yaml"))
	mustWriteFile(t, filepath.Join(root, "com.powerx.plugins.mediax-studio", "0.1.0", "plugin.yaml"))
	mustWriteFile(t, filepath.Join(root, "flat-plugin", "plugin.yaml"))
	mustWriteFile(t, filepath.Join(root, "package.pxp"))
	mustWriteFile(t, filepath.Join(root, "ignored.txt"))
	if err := os.MkdirAll(filepath.Join(root, "empty-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := discoverArtifactPaths(root, entries)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)

	want := []string{
		filepath.Join(root, "com.powerx.plugins.mediax-studio", "0.1.0"),
		filepath.Join(root, "com.powerx.plugins.scrm", "0.1.0"),
		filepath.Join(root, "flat-plugin"),
		filepath.Join(root, "package.pxp"),
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact paths mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("id: test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
