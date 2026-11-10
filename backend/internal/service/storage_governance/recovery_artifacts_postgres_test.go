package storagegov

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

// opt-in：正式校验脚本另建整个临时集群，绝不使用日常配置或现有桶。
func requireNativeArtifacts(t *testing.T) {
	t.Helper()
	if os.Getenv("POWERX_STORAGE_NATIVE_ARTIFACTS") != "1" {
		t.Skip("requires owned PostgreSQL and MinIO artifact acceptance runner")
	}
}

func TestStorageNativeRealBackupRestore(t *testing.T) {
	requireNativeArtifacts(t)
	s, ctrl, queue, db := monitoringNative(t)
	ctx := controlRoot()
	require.NoError(t, database.MigrateBackupPolicyControlModels(db))
	require.NoError(t, db.AutoMigrate(&model.BackupAlert{}, &model.RestoreDrillRecord{}))
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scripts, err := filepath.Abs("../../../scripts/ops")
	require.NoError(t, err)
	t.Setenv("POWERX_OPS_SCRIPT_DIR", scripts)
	t.Setenv("POWERX_OPS_BACKUP_ARTIFACT_DIR", filepath.Join(root, "artifacts"))
	t.Setenv("POWERX_OPS_RESTORE_KEEP_DB", "1")
	policies, jobs := backup.NewPolicyService(db), backup.NewJobService(db)
	require.NoError(t, s.BindDatabaseBackupControl(policies, jobs, "test"))
	adoptBackup(t, policies)
	applyMonitoring(t, ctrl, queue)
	settings := backupSettings()
	settings.RetentionCount = 3
	created := backupChange(t, policies, "save", "", &settings, nil)
	applyMonitoring(t, ctrl, queue)
	var policy model.BackupPolicy
	require.NoError(t, db.Where("uuid = ?", created.PolicyUUID).First(&policy).Error)
	_, err = jobs.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: policy.ID, ForceSync: true})
	require.Error(t, err, "saving a plan must not activate a real capture")
	yes := true
	enabled := backupChange(t, policies, "status", created.PolicyUUID, nil, &yes)
	applyMonitoring(t, ctrl, queue)
	table := pgx.Identifier{coremodel.PowerXSchema, "artifact_acceptance_orders"}.Sanitize()
	require.NoError(t, db.Exec("CREATE TABLE "+table+" (id int PRIMARY KEY, label text, quantity bigint)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+table+" VALUES (1, '样本订单', 3), (2, 'second', 5)").Error)
	job, err := jobs.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: policy.ID, ForceSync: true, Operator: "owned.artifact.acceptance"})
	require.NoError(t, err)
	require.Equal(t, model.BackupJobStatusSuccess, job.Status)
	require.Equal(t, enabled.Configuration.Policies[0].Revision, job.PolicyRevision)
	file, release, err := jobs.OpenDownload(ctx, job.ID, "owned.artifact.acceptance")
	require.NoError(t, err)
	content, readErr := io.ReadAll(file)
	release()
	require.NoError(t, readErr)
	require.True(t, bytes.HasPrefix(content, []byte("PGDMP")), "must capture a real PostgreSQL custom archive")
	// 源库继续产生新值，恢复后必须取得备份时的旧值，不能只证明目标可连接。
	require.NoError(t, db.Exec("UPDATE "+table+" SET quantity=99 WHERE id=1").Error)
	drill, err := backup.NewRestoreDrillService(db).Trigger(ctx, backup.TriggerRestoreDrillRequest{SourceJobID: job.ID, Operator: "owned.artifact.acceptance"})
	require.NoError(t, err)
	var restoredName string
	for _, field := range strings.Fields(drill.ReportURI) {
		if strings.HasPrefix(field, "db=") {
			restoredName = strings.TrimPrefix(field, "db=")
		}
	}
	require.True(t, strings.HasPrefix(restoredName, "powerx_restore_probe_"))
	cfg, err := pgx.ParseConfig(os.Getenv("POWERX_STORAGE_CONTROL_TEST_DSN"))
	require.NoError(t, err)
	require.NotEqual(t, cfg.Database, restoredName)
	admin, err := pgx.ConnectConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, cleanupErr := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{restoredName}.Sanitize()+" WITH (FORCE)")
		require.NoError(t, cleanupErr)
		require.NoError(t, admin.Close(context.Background()))
	})
	require.Equal(t, model.RestoreDrillStatusSuccess, drill.Status, drill.ReportURI)
	copyConfig := cfg.Copy()
	copyConfig.Database = restoredName
	restored, err := pgx.ConnectConfig(ctx, copyConfig)
	require.NoError(t, err)
	defer restored.Close(ctx)
	var label string
	var quantity int64
	require.NoError(t, restored.QueryRow(ctx, "SELECT label,quantity FROM "+table+" WHERE id=1").Scan(&label, &quantity))
	require.Equal(t, "样本订单", label)
	require.EqualValues(t, 3, quantity)
	var total int64
	require.NoError(t, restored.QueryRow(ctx, "SELECT sum(quantity) FROM "+table).Scan(&total))
	require.EqualValues(t, 8, total)
	require.NoError(t, db.Raw("SELECT quantity FROM "+table+" WHERE id=1").Scan(&quantity).Error)
	require.EqualValues(t, 99, quantity, "restore must not overwrite the live source")
	no := false
	backupChange(t, policies, "status", created.PolicyUUID, nil, &no)
	_, err = jobs.TriggerJob(ctx, backup.TriggerJobRequest{PolicyID: policy.ID, ForceSync: true})
	require.Error(t, err, "pause must stop real capture before a stale publication can be used")
	t.Logf("real pg_dump/pg_restore: published plan revision=%d bytes=%d; historical rows and unchanged source verified", job.PolicyRevision, len(content))
}

func TestStorageNativeRealS3ArchiveRecovery(t *testing.T) {
	requireNativeArtifacts(t)
	s, ctrl, queue, _ := monitoringNative(t)
	ctx := controlRoot()
	var config struct {
		Endpoint string `json:"endpoint"`
		Bucket   string `json:"bucket"`
		Region   string `json:"region"`
		Access   string `json:"access_key"`
		Secret   string `json:"secret_key"`
	}
	raw, err := os.ReadFile(os.Getenv("POWERX_STORAGE_VALIDATION_CONFIG"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &config))
	require.True(t, strings.HasPrefix(config.Endpoint, "http://127.0.0.1:"))
	require.Equal(t, "powerx-storage-validation", config.Bucket)
	client, err := minio.New(config.Endpoint, &minio.Options{AccessKey: config.Access, SecretKey: config.Secret, Region: config.Region, ForcePathStyle: true})
	require.NoError(t, err)
	versioned, err := client.BucketVersioningEnabled(ctx, config.Bucket)
	if err != nil {
		response := minio.ToErrorResponse(err)
		message := strings.NewReplacer(config.Access, "[redacted]", config.Secret, "[redacted]").Replace(response.Message)
		t.Fatalf("owned S3 versioning request failed: code=%s message=%s", response.Code, message)
	}
	require.True(t, versioned)
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	source, runtimeDir := filepath.Join(root, "source"), filepath.Join(root, "archive")
	require.NoError(t, os.Mkdir(source, 0700))
	require.NoError(t, os.Mkdir(filepath.Join(root, "vault"), 0700))
	want := []byte("owned sealed audit 测试\n")
	for i, name := range []string{"app.log", "app.log.1", "app.log.2"} {
		path := filepath.Join(source, name)
		payload := want
		if i != 2 {
			payload = []byte(name + " preserved\n")
		}
		require.NoError(t, os.WriteFile(path, payload, 0600))
		old := time.Now().Add(-time.Duration([]int{0, 1, 10}[i]) * 24 * time.Hour)
		require.NoError(t, os.Chtimes(path, old, old))
	}
	key := make([]byte, 32)
	_, err = rand.Read(key)
	require.NoError(t, err)
	keyring := filepath.Join(root, "external-keyring.json")
	raw, err = json.Marshal(vault.Keyring{Version: 1, StoreID: "owned-real-artifacts", ActiveKeyID: "key1", Keys: map[string]string{"key1": base64.StdEncoding.EncodeToString(key)}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyring, raw, 0600))
	a := archive.NewService(context.Background(), archive.Config{Directory: runtimeDir, Instance: "owned-install", ExecutionNodeID: "owned-install", CredentialVault: &vault.Config{Directory: filepath.Join(root, "vault"), KeyringFile: keyring, StoreID: "owned-real-artifacts"}, Sources: []archive.Source{{ID: "logs", Kind: "application_logs", Directory: source, RotationFormat: "numeric", ActiveFiles: []string{"app.log"}}}}, archive.Options{})
	t.Cleanup(a.Close)
	credential, err := a.SaveS3Credential(ctx, archive.SaveS3CredentialInput{RequestID: uuid.NewString(), Ref: "owned-live", AccessKey: config.Access, SecretKey: config.Secret})
	require.NoError(t, err)
	target, err := a.SaveTarget(ctx, archive.Target{Driver: "s3", Name: "owned MinIO", Endpoint: config.Endpoint, Bucket: config.Bucket, Region: config.Region, Prefix: "governed/" + uuid.NewString(), CredentialRef: "owned-live", ForcePathStyle: true})
	require.NoError(t, err)
	require.NoError(t, s.BindArchiveControl(a))
	maintenance := filepath.Join(root, "maintenance")
	require.NoError(t, os.Mkdir(maintenance, 0700))
	anchor, err := storagerecovery.Initialize(maintenance, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, s.BindRecoveryAnchor(anchor))
	targetConfig, err := a.TargetConfiguration(ctx)
	require.NoError(t, err)
	_, err = a.AdoptTargetConfiguration(ctx, archive.TargetAdoption{RequestID: uuid.NewString(), LegacyDigest: targetConfig.LegacyDigest})
	require.NoError(t, err)
	applyMonitoring(t, ctrl, queue)
	require.NoError(t, a.TestTarget(ctx, target.ID))
	policyConfig, err := a.PolicyConfiguration(ctx)
	require.NoError(t, err)
	_, err = a.AdoptPolicyConfiguration(ctx, archive.PolicyAdoption{RequestID: uuid.NewString(), LegacyDigest: policyConfig.LegacyDigest})
	require.NoError(t, err)
	applyMonitoring(t, ctrl, queue)
	created, err := a.SavePolicyConfiguration(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Action: "save", Settings: &archive.PolicySettings{Name: "owned archive", SourceID: "logs", TargetID: target.ID, IntervalMinutes: 5, RetentionDays: 7, RetentionCount: 10, RemoteRetentionDays: 30, RemoteRetentionCount: 100, RemoteMaxBytes: 16 << 20}})
	require.NoError(t, err)
	applyMonitoring(t, ctrl, queue)
	_, err = a.Start(ctx, created.PolicyID, uuid.NewString(), "owned", "")
	require.Error(t, err, "a saved archive policy remains paused")
	view, err := a.Overview(ctx)
	require.NoError(t, err)
	require.Len(t, view.Targets, 1)
	activeTarget := view.Targets[0]
	require.True(t, activeTarget.VerificationValid)
	yes, no := true, false
	_, err = a.SavePolicyConfiguration(ctx, archive.PolicyChange{RequestID: uuid.NewString(), ExpectedRevision: created.OperationRevision, ExpectedPolicyRevision: created.Configuration.Policies[0].Revision, PolicyID: created.PolicyID, Action: "status", Enabled: &yes, RemoteCleanupEnabled: &no, Confirm: true, ExpectedTargetRevision: activeTarget.Revision, ExpectedTargetFingerprint: activeTarget.VerifiedFingerprint})
	require.NoError(t, err)
	applyMonitoring(t, ctrl, queue)
	job, err := a.Start(ctx, created.PolicyID, uuid.NewString(), "owned", "")
	require.NoError(t, err)
	jobID := job.ID
	require.Eventually(t, func() bool {
		current, e := a.Job(ctx, jobID)
		if e == nil {
			job = current
		}
		return e == nil && (current.Status == "failed" || current.RemoteState == "success" || current.RemoteState == "failed" || current.RemoteState == "unconfirmed")
	}, 15*time.Second, 20*time.Millisecond)
	require.Equal(t, "success", job.Status, job.Error)
	require.Equal(t, "success", job.RemoteState, job.RemoteError)
	require.NotEmpty(t, job.RemoteManifestVersion)
	require.Len(t, job.Files, 1)
	entry := job.Files[0]
	require.Equal(t, "app.log.2", filepath.Base(entry.Path))
	verified, err := a.VerifyLifecycleFile(ctx, source, filepath.Base(entry.Path), entry.Size, entry.ModifiedAt)
	require.NoError(t, err)
	require.True(t, verified)
	// 最新版本被替换时，归档必须继续读取记录中的确切旧版本。
	replacement := []byte("replacement latest version must not become historical content\n")
	newVersion, err := client.PutObject(ctx, config.Bucket, entry.ArchiveKey, bytes.NewReader(replacement), int64(len(replacement)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	require.NoError(t, err)
	require.NotEmpty(t, newVersion.VersionID)
	require.NotEqual(t, entry.ArchiveVersion, newVersion.VersionID)
	require.NoError(t, os.Remove(filepath.Join(source, filepath.Base(entry.Path))))
	cache := filepath.Join(runtimeDir, "objects", entry.SHA256[:2], entry.SHA256)
	require.NoError(t, os.Remove(cache))
	_, err = anchor.Begin(ctx, uuid.NewString(), uuid.NewString())
	require.NoError(t, err)
	var downloaded bytes.Buffer
	require.Error(t, a.Download(ctx, job.ID, &downloaded), "old credential must not be usable after restore")
	fresh, err := a.SaveS3Credential(ctx, archive.SaveS3CredentialInput{RequestID: uuid.NewString(), ExpectedRevision: credential.Receipt.Revision, Ref: target.CredentialRef, AccessKey: config.Access, SecretKey: config.Secret})
	require.NoError(t, err)
	require.NotEmpty(t, fresh.Receipt.RecoveryEpoch)
	require.Error(t, a.CheckRecoveryTarget(ctx, activeTarget), "fresh credentials cannot revive an old target proof")
	require.NoError(t, a.TestTarget(ctx, target.ID))
	downloaded.Reset()
	require.NoError(t, a.Download(ctx, job.ID, &downloaded))
	gz, err := gzip.NewReader(bytes.NewReader(downloaded.Bytes()))
	require.NoError(t, err)
	defer gz.Close()
	reader := tar.NewReader(gz)
	found := false
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		if header.Name == entry.Path || strings.HasSuffix(header.Name, "/"+entry.Path) {
			actual, readErr := io.ReadAll(reader)
			require.NoError(t, readErr)
			require.Equal(t, want, actual)
			found = true
		}
	}
	require.True(t, found, "download must contain the original sealed log bytes")
	require.FileExists(t, filepath.Join(source, "app.log"))
	require.FileExists(t, filepath.Join(source, "app.log.1"))
	t.Logf("governed real S3: exact-version archive=%s latest replaced and source/cache removed; new epoch credential+target confirmation restored historical bytes", job.ID)
}
