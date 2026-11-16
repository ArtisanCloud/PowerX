package archive_ops

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func recoveryFixture(t *testing.T) (Source, string, map[string]any) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "powerx_dev")
	dir := filepath.Join(root, "apply-123", "batch-00000001")
	require.NoError(t, os.MkdirAll(dir, 0700))
	uid := uint32(os.Geteuid())
	source := Source{ID: "dev-audit-recovery", Kind: "database_files", Directory: root, FileOwnerUID: &uid, AuditRecoveryDatabase: "powerx_dev"}
	contract := map[string]any{"automatic_backup_deletion": false, "backup_review_after": time.Now().Add(7 * 24 * time.Hour), "scope": "historical verified idle dequeue only"}
	writeFixtureJSON(t, filepath.Join(root, "apply-123", "run-contract.json"), contract)
	archives := []map[string]any{}
	for _, name := range []string{"traces.copy.gz", "events.copy.gz"} {
		raw := append([]byte("PGCOPY\n\xff\r\n\x00"), make([]byte, 8)...)
		raw = append(raw, 0, 1, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 1, 0xff, 0xff)
		var out bytes.Buffer
		g := gzip.NewWriter(&out)
		_, e := g.Write(raw)
		require.NoError(t, e)
		require.NoError(t, g.Close())
		data := out.Bytes()
		h := sha256.Sum256(data)
		rh := sha256.Sum256(raw)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0600))
		archives = append(archives, map[string]any{"file": name, "rows": int64(1), "compressed_bytes": int64(len(data)), "sha256": hex.EncodeToString(h[:]), "binary_copy_sha256": hex.EncodeToString(rh[:])})
	}
	r := map[string]any{"state": "committed", "at": time.Now().Add(-time.Hour), "cutoff": time.Now().Add(-48 * time.Hour), "rows": 1, "first_trace_id": 1, "last_trace_id": 1, "restore_verified": true, "schema_signatures": map[string]string{"public.capability_registry_invocation_traces": "trace-schema", "public.capability_registry_event_publications": "event-schema"}, "archives": archives}
	writeFixtureJSON(t, filepath.Join(dir, "receipt.json"), r)
	return source, dir, r
}
func writeFixtureJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, e := json.Marshal(v)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func TestAuditRecoveryCapturesOnlyVerifiedCommittedBatchesAndDeduplicates(t *testing.T) {
	source, dir, _ := recoveryFixture(t)
	before, e := os.ReadFile(filepath.Join(dir, "traces.copy.gz"))
	require.NoError(t, e)
	plan, e := auditRecoveryPlan(context.Background(), source)
	require.NoError(t, e)
	require.Equal(t, 1, plan.Batches)
	require.Len(t, plan.Files, 4)
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "owned", Sources: []Source{source}}, Options{})
	t.Cleanup(s.Close)
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "恢复归档", SourceID: source.ID, IntervalMinutes: 5, RetentionDays: 30, RetentionCount: 1, RemoteRetentionDays: 30, RemoteRetentionCount: 100, RemoteMaxBytes: 4 << 30})
	require.NoError(t, e)
	j, e := s.start(archiveRoot(), p.ID, uuidFixture(0), "test.root", "", true)
	require.NoError(t, e)
	require.Equal(t, "success", j.Status)
	require.True(t, j.Protected)
	require.True(t, j.ProvenanceVerified)
	require.Equal(t, 1, j.AuditRecoveryBatches)
	require.Len(t, j.Files, 4)
	stored, e := s.readJob(j.ID)
	require.NoError(t, e)
	unchanged, e := s.sealedUnchanged(context.Background(), source, stored, p)
	require.NoError(t, e)
	require.True(t, unchanged, "frozen inputs must not create perpetual new archive jobs")
	after, e := os.ReadFile(filepath.Join(dir, "traces.copy.gz"))
	require.NoError(t, e)
	require.Equal(t, before, after, "source recovery copy must remain intact")
	// 快速检查只比较已验证输入身份；内容被改写后必须重新全量核验，不能继续复用成功回执。
	require.NoError(t, os.WriteFile(filepath.Join(dir, "traces.copy.gz"), append([]byte(nil), before...), 0600))
	unchanged, e = s.sealedUnchanged(context.Background(), source, stored, p)
	require.NoError(t, e)
	require.False(t, unchanged)
}

func TestAuditRecoveryUnknownOrMalformedProofNeverBecomesManagedCopy(t *testing.T) {
	for _, kind := range []string{"delete_intent", "restore_unverified", "foreign_file", "missing_pair", "bad_compressed_hash", "bad_raw_hash", "missing_schema", "future_time", "symlink", "hardlink", "owner", "contract", "gzip_tail"} {
		t.Run(kind, func(t *testing.T) {
			source, dir, r := recoveryFixture(t)
			a := r["archives"].([]map[string]any)
			switch kind {
			case "delete_intent":
				r["state"] = "delete_intent"
			case "restore_unverified":
				r["restore_verified"] = false
			case "foreign_file":
				a[0]["file"] = "../../secret"
			case "missing_pair":
				r["archives"] = a[:1]
			case "bad_compressed_hash":
				a[0]["sha256"] = string(bytes.Repeat([]byte("a"), 64))
			case "bad_raw_hash":
				a[0]["binary_copy_sha256"] = string(bytes.Repeat([]byte("a"), 64))
			case "missing_schema":
				r["schema_signatures"] = map[string]string{}
			case "future_time":
				r["at"] = time.Now().Add(24 * time.Hour)
			case "symlink":
				path := filepath.Join(dir, "traces.copy.gz")
				require.NoError(t, os.Rename(path, path+".saved"))
				require.NoError(t, os.Symlink(path+".saved", path))
			case "hardlink":
				require.NoError(t, os.Link(filepath.Join(dir, "traces.copy.gz"), filepath.Join(dir, "extra")))
			case "owner":
				uid := uint32(os.Geteuid() + 1)
				source.FileOwnerUID = &uid
			case "contract":
				writeFixtureJSON(t, filepath.Join(source.Directory, "apply-123", "run-contract.json"), map[string]any{"automatic_backup_deletion": true, "backup_review_after": time.Now()})
			case "gzip_tail":
				p := filepath.Join(dir, "traces.copy.gz")
				b, e := os.ReadFile(p)
				require.NoError(t, e)
				b = append(b, 1)
				require.NoError(t, os.WriteFile(p, b, 0600))
				h := sha256.Sum256(b)
				a[0]["sha256"] = hex.EncodeToString(h[:])
				a[0]["compressed_bytes"] = int64(len(b))
			}
			writeFixtureJSON(t, filepath.Join(dir, "receipt.json"), r)
			plan, e := auditRecoveryPlan(context.Background(), source)
			if kind == "delete_intent" {
				require.NoError(t, e)
				require.Zero(t, plan.Batches)
				require.Equal(t, 1, plan.Protected)
			} else {
				require.Error(t, e)
			}
		})
	}
}

func TestAuditRecoveryRejectsInvalidCOPYFramingAndCancellation(t *testing.T) {
	header := "PGCOPY\n\xff\r\n\x00" + strings.Repeat("\x00", 8)
	for _, raw := range []string{
		"invalid", header + "\xff\xff", // 行数不足。
		header + "\x00\x01\xff\xff\xff\xfe\xff\xff",      // 非法负字段长度。
		header + "\x00\x01\x00\x00\x00\x08\x01",          // 字段截断。
		header + "\x00\x01\xff\xff\xff\xff\xff\xffextra", // COPY 尾部多余内容。
	} {
		require.Error(t, verifyRecoveryCOPY(strings.NewReader(raw), 1))
	}
	source, _, _ := recoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := auditRecoveryPlan(ctx, source)
	require.ErrorIs(t, e, context.Canceled)
}

func TestAuditRecoveryRequiresExplicitFixedSourceContract(t *testing.T) {
	source, _, _ := recoveryFixture(t)
	for _, mutate := range []func(*Source){
		func(s *Source) { s.Kind = "media" },
		func(s *Source) { s.FileOwnerUID = nil },
		func(s *Source) { s.AuditRecoveryDatabase = "another_database" },
	} {
		candidate := source
		mutate(&candidate)
		c := Config{Directory: t.TempDir(), Instance: "owned", Sources: []Source{candidate}}
		require.Error(t, validateConfig(&c))
	}
	c := Config{Directory: t.TempDir(), Instance: "owned", Sources: []Source{source}}
	require.NoError(t, validateConfig(&c))
}

func TestAuditRecoveryAcceptsActualOwnedPostgresBinaryCOPY(t *testing.T) {
	dsn := os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicitly provided isolated PostgreSQL fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer conn.Close(context.Background())
	var raw bytes.Buffer
	_, e = conn.PgConn().CopyTo(ctx, &raw, "COPY (SELECT 1::bigint, 'owned-recovery-fixture'::text) TO STDOUT (FORMAT binary)")
	require.NoError(t, e)
	require.NoError(t, verifyRecoveryCOPY(bytes.NewReader(raw.Bytes()), 1))
	require.Error(t, verifyRecoveryCOPY(bytes.NewReader(raw.Bytes()), 2))
}

func TestAuditRecoveryThirtyDayMinimumSurvivesCountPressureAndProtection(t *testing.T) {
	source, _, _ := recoveryFixture(t)
	now := time.Now()
	s := NewService(context.Background(), Config{Directory: t.TempDir(), Instance: "owned", Sources: []Source{source}}, Options{RetentionNow: func() time.Time { return now }})
	t.Cleanup(s.Close)
	p, e := s.SavePolicy(archiveRoot(), Policy{Name: "恢复归档", SourceID: source.ID, IntervalMinutes: 5, RetentionDays: 30, RetentionCount: 1, RemoteRetentionDays: 30, RemoteRetentionCount: 100})
	require.NoError(t, e)
	oldID := ""
	for i := 0; i < 3; i++ {
		j := jobRecord{Job: Job{ID: uuidFixture(i), PolicyID: p.ID, SourceID: source.ID, Status: "success", Phase: "completed", CreatedAt: now.Add(-time.Duration(i+1) * time.Hour), Protected: i == 2, ProvenanceVerified: true, AuditRecoveryFingerprint: string(bytes.Repeat([]byte("b"), 64))}}
		require.NoError(t, s.saveJob(&j))
		if i == 1 {
			oldID = j.ID
		}
	}
	n, e := s.cleanupApproved(context.Background(), p.ID)
	require.NoError(t, e)
	require.Zero(t, n, "count pressure cannot shorten the 30 day recovery window")
	now = now.Add(31 * 24 * time.Hour)
	n, e = s.cleanupApproved(context.Background(), p.ID)
	require.NoError(t, e)
	require.Equal(t, 1, n)
	j, e := s.readJob(oldID)
	require.NoError(t, e)
	require.Equal(t, "pruned", j.Status)
	protected, e := s.readJob(uuidFixture(2))
	require.NoError(t, e)
	require.True(t, protected.Protected)
	require.Equal(t, "success", protected.Status)
}
func uuidFixture(i int) string {
	return []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", "00000000-0000-4000-8000-000000000003"}[i]
}
