package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetentionLimitsAndPreservation(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	policy := modelops.BackupPolicy{RetentionMode: "age_and_count", RetentionDays: 7, RetentionCount: 3}
	job := modelops.BackupJob{EndedAt: &old}
	if backupExpired(job, 0, policy, now) {
		t.Fatal("latest backup must survive age limit")
	}
	if !backupExpired(job, 1, policy, now) {
		t.Fatal("old backup must expire")
	}
	job.Protected = true
	if backupExpired(job, 20, policy, now) {
		t.Fatal("protected backup must survive all limits")
	}
	job.Protected = false
	job.EndedAt = &now
	if !backupExpired(job, 3, policy, now) {
		t.Fatal("count limit not applied")
	}
	if backupExpired(job, 2, policy, now) {
		t.Fatal("fresh retained copy expired")
	}
	policy.RetentionMode = "count"
	job.EndedAt = &old
	if backupExpired(job, 1, policy, now) {
		t.Fatal("count-only policy unexpectedly applied days")
	}
	for _, tc := range []struct {
		mode string
		days int
	}{{"unknown", 7}, {"age_and_count", -1}, {"count", 3651}} {
		if _, _, err := normalizeRetention(tc.mode, tc.days); err == nil {
			t.Fatal("invalid retention accepted")
		}
	}
}
func testArtifact(t *testing.T, root, name string) *modelops.BackupArtifact {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	content := []byte("registered backup data")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return &modelops.BackupArtifact{StorageURI: "file://" + path, SizeBytes: int64(len(content)), Checksum: hex.EncodeToString(sum[:])}
}
func TestArtifactSafety(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	artifact := testArtifact(t, root, "tenant/policy_1/a.dump")
	path, err := validatedArtifactPath(root, artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyArtifactFile(path, artifact); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(strings.Repeat("x", int(artifact.SizeBytes))), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyArtifactFile(path, artifact) == nil {
		t.Fatal("tampered checksum accepted")
	}
	outside := testArtifact(t, filepath.Dir(root), filepath.Base(root)+"-outside.dump")
	defer os.Remove(strings.TrimPrefix(outside.StorageURI, "file://"))
	if _, err = validatedArtifactPath(root, outside); err == nil {
		t.Fatal("outside root accepted")
	}
	link := filepath.Join(root, "link")
	if err = os.Symlink(filepath.Dir(path), link); err != nil {
		t.Fatal(err)
	}
	artifact.StorageURI = "file://" + filepath.Join(link, "a.dump")
	if _, err = validatedArtifactPath(root, artifact); err == nil {
		t.Fatal("symlink traversal accepted")
	}
	for _, uri := range []string{"https://example.test/a.dump", "file://host/a.dump", "file:///a.dump?secret=x", "a.dump"} {
		if _, err := parseFileStorageURI(uri); err == nil {
			t.Fatalf("URI accepted: %s", uri)
		}
	}
}
func TestSourceEnvironmentOmitsPasswordFromArguments(t *testing.T) {
	env, err := postgresEnvironment("postgresql://operator:pass%40word@127.0.0.1:5432/powerx_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, value := range []string{"PGDATABASE=powerx_test", "PGPASSWORD=pass@word", "PGSSLMODE=disable"} {
		if !strings.Contains(joined, value) {
			t.Fatalf("missing %s", value)
		}
	}
	if _, err := postgresEnvironment(""); err == nil {
		t.Fatal("missing source accepted")
	}
}
func TestRestoreScriptCannotDropExistingDatabase(t *testing.T) {
	script, err := filepath.Abs("../../../scripts/ops/restore-drill.sh")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "backup.dump")
	if err = os.WriteFile(artifact, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"powerx_pro", "postgres", "powerx_restore_probe_1"} {
		cmd := exec.Command("bash", script, "1", artifact)
		cmd.Env = append(os.Environ(), "POWERX_OPS_RESTORE_PROBE_DB="+target)
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "custom target databases are forbidden") {
			t.Fatalf("unsafe target was not rejected: %s %s", target, out)
		}
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "commands.log")
	for _, tool := range []string{"pg_restore", "createdb", "dropdb", "psql"} {
		body := "#!/bin/sh\necho " + tool + " >> \"$PROBE_LOG\"\n"
		if tool == "createdb" {
			body += "exit 1\n"
		}
		if err = os.WriteFile(filepath.Join(dir, tool), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", script, "1", artifact)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "PGDATABASE=powerx_pro", "PGHOST=127.0.0.1", "PGUSER=test", "PROBE_LOG="+log, "POWERX_OPS_RESTORE_PROBE_DB=", "POWERX_OPS_RESTORE_PROBE_DB_PREFIX=")
	if err = cmd.Run(); err == nil {
		t.Fatal("existing target creation must fail")
	}
	content, _ := os.ReadFile(log)
	if strings.Contains(string(content), "dropdb") {
		t.Fatal("failed creation attempted to delete existing database")
	}
}
func TestMissingRestoreScriptFails(t *testing.T) {
	t.Setenv("POWERX_OPS_SCRIPT_DIR", t.TempDir())
	svc := NewRestoreDrillService(nil)
	if _, err := svc.runRestoreScript(context.Background(), 1, "missing.dump"); err == nil {
		t.Fatal("missing restore script reported success")
	}
}
