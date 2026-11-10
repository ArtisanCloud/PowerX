package storagelifecycle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/stretchr/testify/require"
)

func nativePostgresFixture(t *testing.T) (*Engine, Source, []string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "log")
	require.NoError(t, os.Mkdir(dir, 0700))
	names := []string{"postgresql-2026-01-04_010000.log", "postgresql-2026-01-03_010000.log", "postgresql-2026-01-02_010000.log", "postgresql-2026-01-01_010000.log"}
	for i, name := range names {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte("owned native log fixture"), 0600))
		at := time.Now().Add(-time.Duration(10+i) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	manifest := filepath.Join(root, "current_logfiles")
	require.NoError(t, os.WriteFile(manifest, []byte("stderr log/"+names[0]+"\n"), 0600))
	s := Source{ID: "pg", Kind: "postgresql_logs", Scope: "shared", Directory: dir, Format: "postgresql", ActiveFiles: []string{"postgresql-"}, CurrentLogfiles: manifest, OwnerUID: uint32(os.Geteuid())}
	c, err := budget.New(budget.Config{Directory: filepath.Join(root, "state"), FilesystemPath: root, HostID: "host", FilesystemID: "disk", InstanceID: "dev", MinFreeBytes: 1 << 20, CriticalFreeBytes: 1})
	require.NoError(t, err)
	e, err := New(Options{Coordinator: c, InstanceID: "dev", AllowEnforce: true, Sources: []Source{s}})
	require.NoError(t, err)
	_, err = e.Save(context.Background(), 1, "pg", 0, Policy{Enabled: true, CleanupMode: "expire_delete", RetentionDays: 7, MaxBytes: 1 << 20})
	require.NoError(t, err)
	return e, e.sources["pg"], names
}

func TestPostgresqlNativeManifestProtectsActiveAndLatestPerDestination(t *testing.T) {
	e, s, names := nativePostgresFixture(t)
	for i, name := range names {
		path := filepath.Join(s.Directory, strings.TrimSuffix(name, ".log")+".csv")
		require.NoError(t, os.WriteFile(path, []byte("owned csv fixture"), 0600))
		at := time.Now().Add(-time.Duration(10+i) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	require.NoError(t, os.WriteFile(s.CurrentLogfiles, []byte("stderr log/"+names[0]+"\ncsvlog log/"+strings.TrimSuffix(names[0], ".log")+".csv\n"), 0600))
	for _, name := range []string{"business.txt", "postgresql-2026-02-30_010000.log", "postgresql-Mon.log"} {
		require.NoError(t, os.WriteFile(filepath.Join(s.Directory, name), []byte("protected"), 0600))
	}
	p, err := e.Preview(context.Background(), "pg")
	require.NoError(t, err)
	require.True(t, p.Complete)
	require.Len(t, p.Candidates, 4)
	require.Equal(t, 2, p.Protected["latest_rotation"])
	require.Equal(t, 5, p.Protected["active_or_unknown"])
	archive, err := InspectSealedSource(context.Background(), s)
	require.NoError(t, err)
	require.Equal(t, p.CandidateNames, archive.CandidateNames, "archive and cleanup must use the same active-file boundary")
	before, err := postgresActivity(s)
	require.NoError(t, err)
	f, err := os.OpenFile(filepath.Join(s.Directory, names[0]), os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.WriteString("append during observation")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.True(t, before.unchanged(s), "normal writes to active logs must not invalidate a stable rotation manifest")
}

func TestPostgresqlMalformedOrUntrustedManifestNeverMakesDeletionCandidates(t *testing.T) {
	for _, input := range []string{"", "stderr ../log/postgresql-2026-01-04_010000.log\n", "stderr /outside/postgresql-2026-01-04_010000.log\n", "stderr log/missing.log\n", "unknown log/postgresql-2026-01-04_010000.log\n", "stderr log/postgresql-2026-01-04_010000.log\nstderr log/postgresql-2026-01-04_010000.log\n", strings.Repeat("x", 4097)} {
		t.Run(fmt.Sprintf("bytes-%d-%x", len(input), sha256.Sum256([]byte(input))), func(t *testing.T) {
			e, s, _ := nativePostgresFixture(t)
			require.NoError(t, os.WriteFile(s.CurrentLogfiles, []byte(input), 0600))
			p, err := e.Preview(context.Background(), "pg")
			require.NoError(t, err)
			require.False(t, p.Complete)
			require.Empty(t, p.Candidates)
			require.Equal(t, "postgresql_activity_unavailable", p.Problem)
		})
	}
	for _, mutation := range []string{"missing", "symlink", "hardlink", "writable", "directory-writable"} {
		t.Run(mutation, func(t *testing.T) {
			e, s, _ := nativePostgresFixture(t)
			switch mutation {
			case "missing":
				require.NoError(t, os.Remove(s.CurrentLogfiles))
			case "symlink":
				require.NoError(t, os.Rename(s.CurrentLogfiles, s.CurrentLogfiles+".real"))
				require.NoError(t, os.Symlink(s.CurrentLogfiles+".real", s.CurrentLogfiles))
			case "hardlink":
				require.NoError(t, os.Link(s.CurrentLogfiles, s.CurrentLogfiles+".linked"))
			case "writable":
				require.NoError(t, os.Chmod(s.CurrentLogfiles, 0666))
			case "directory-writable":
				require.NoError(t, os.Chmod(s.Directory, 0770))
			}
			p, err := e.Preview(context.Background(), "pg")
			require.NoError(t, err)
			require.False(t, p.Complete)
			require.Empty(t, p.Candidates)
		})
	}
}

func TestPostgresqlRotationDuringDeleteRestoresCapturedFile(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-hold-%t", after), func(t *testing.T) {
			e, s, names := nativePostgresFixture(t)
			p, err := e.Preview(context.Background(), "pg")
			require.NoError(t, err)
			mutate := func() { require.NoError(t, os.WriteFile(s.CurrentLogfiles, []byte("stderr log/"+names[2]+"\n"), 0600)) }
			if after {
				e.afterHold = mutate
			} else {
				e.beforeHold = mutate
			}
			removed, problem, held := e.remove(context.Background(), s, p.Candidates[0], "owned-intent")
			require.False(t, removed)
			require.False(t, held)
			require.Equal(t, "postgresql_activity_changed", problem)
			for _, name := range names {
				require.FileExists(t, filepath.Join(s.Directory, name))
			}
		})
	}
}

func TestPostgresqlOpenHandleAndUnavailableProofPreventUnlink(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprintf("unavailable-%t", unavailable), func(t *testing.T) {
			e, s, _ := nativePostgresFixture(t)
			p, err := e.Preview(context.Background(), "pg")
			require.NoError(t, err)
			c := p.Candidates[0]
			if unavailable {
				t.Setenv("PATH", t.TempDir())
			} else {
				if _, err := exec.LookPath("lsof"); err != nil {
					t.Skip("lsof not installed; fail-closed path is tested separately")
				}
				f, err := os.Open(filepath.Join(s.Directory, c.Name))
				require.NoError(t, err)
				defer f.Close()
			}
			removed, problem, held := e.remove(context.Background(), s, c, "owned-intent")
			require.False(t, removed)
			require.False(t, held)
			require.Equal(t, "postgresql_file_open_or_unknown", problem)
			require.FileExists(t, filepath.Join(s.Directory, c.Name))
		})
	}
}

func TestPostgresqlClosedExpiredLogsDeleteWithoutTouchingActiveOrLatest(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof not installed")
	}
	e, s, names := nativePostgresFixture(t)
	run, err := e.RunWithKey(context.Background(), "pg", "native-closed")
	require.NoError(t, err)
	require.Equal(t, "completed", run.State, run.Problem)
	require.Equal(t, 2, run.DeletedFiles)
	require.FileExists(t, filepath.Join(s.Directory, names[0]))
	require.FileExists(t, filepath.Join(s.Directory, names[1]))
	require.NoFileExists(t, filepath.Join(s.Directory, names[2]))
	require.NoFileExists(t, filepath.Join(s.Directory, names[3]))
}
