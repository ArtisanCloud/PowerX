package storagegov

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDatasetContractRejectsInvalidBoundsCountsAndLease(t *testing.T) {
	_, err := parseDatasetUUID(uuid.Nil.String())
	require.Error(t, err)
	require.Error(t, (DatasetRetentionBounds{MinimumDays: 30, SuggestedDays: 20, MaximumDays: 7}).Validate())
	require.NoError(t, (DatasetRetentionBounds{MinimumDays: 7, SuggestedDays: 30, MaximumDays: 3650}).Validate())
	result := DatasetPreviewCounts{
		Coverage: "bounded", ExaminedCount: 10, CandidateCount: 3,
		ProtectedByReason: []DatasetProtectionCount{{"referenced", 1}, {"latest_effective", 1}, {"incomplete_record", 2}, {"unknown_eligibility", 2}},
	}
	require.NoError(t, result.Validate(10))
	require.Error(t, result.Validate(9))
	result.ProtectedByReason = append(result.ProtectedByReason, DatasetProtectionCount{"referenced", 1})
	require.Error(t, result.Validate(10))
	result.ProtectedByReason[4] = DatasetProtectionCount{"active", 2}
	require.Error(t, result.Validate(10))

	task := DatasetPreviewTask{uuid.New(), "core-task", "fixture-deployment", uuid.New(), "com.powerx.plugins.storage-fixture", "rev-1", "managed-subscriber"}
	now := time.Now()
	lease := DatasetPreviewLease{task, uuid.New(), 3, now.Add(time.Minute)}
	require.NoError(t, ValidateDatasetPreviewLease(task, lease, lease.ReceiptUUID, 3, now))
	require.Error(t, ValidateDatasetPreviewLease(task, lease, lease.ReceiptUUID, 2, now))
	require.Error(t, ValidateDatasetPreviewLease(task, lease, lease.ReceiptUUID, 3, lease.ExpiresAt))
	imitation := lease
	imitation.Task.TaskID = "ordinary-task-with-identical-notice"
	require.Error(t, ValidateDatasetPreviewLease(task, imitation, lease.ReceiptUUID, 3, now))
	imitation = lease
	imitation.Task.TenantUUID = uuid.New()
	require.Error(t, ValidateDatasetPreviewLease(task, imitation, lease.ReceiptUUID, 3, now))
}

func TestDatasetCatalogSyntheticPluginNativePostgres(t *testing.T) {
	db, _ := ownedControlPostgres(t)
	scope := "dataset-fixture-deployment"
	s := &Service{opts: Options{DB: db, Identity: Identity{InstanceID: scope}}}
	ctx := context.Background()
	pluginID := "com.powerx.plugins.storage-fixture"
	in := VerifiedDatasetAdmission{
		RequestUUID: uuid.NewString(), TenantUUID: uuid.New(), CallerPluginID: pluginID,
		OwnerID: pluginID, Installation: "fixture-v1", BindingUUID: uuid.New(), Actor: "fixture-sts",
		DefinitionDigest: strings.Repeat("a", 64), SourceFingerprint: strings.Repeat("b", 64),
		Declaration: DatasetDeclaration{
			DatasetKey: "fixture_report", DisplayName: map[string]string{"zh-CN": "合成执行报告", "en": "Synthetic reports"},
			ContractVersion: 1, SchemaContractVersion: 1, SourceBindingRef: "database", TenantScope: "tenant",
			AdapterKey: "fixture_report", AdapterVersion: 1, AllowedActions: []string{"observe", "preview"},
			RetentionBounds: &DatasetRetentionBounds{MinimumDays: 7, SuggestedDays: 30, MaximumDays: 3650},
		},
	}
	var wg sync.WaitGroup
	var receipts [8]DatasetAdmissionReceipt
	var failures [8]error
	for index := range receipts {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			receipts[index], failures[index] = s.AdmitVerifiedDataset(ctx, in)
		}(index)
	}
	wg.Wait()
	first := receipts[0]
	for index, receipt := range receipts {
		require.NoError(t, failures[index])
		require.Equal(t, first, receipt)
	}
	require.Equal(t, "accepted", first.RegistrationState)
	var count int64
	require.NoError(t, db.Model(&m.DatasetRegistration{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	duplicate := in
	duplicate.RequestUUID = uuid.NewString()
	second, err := s.AdmitVerifiedDataset(ctx, duplicate)
	require.NoError(t, err)
	require.Equal(t, first.DatasetUUID, second.DatasetUUID)
	require.Equal(t, first.AcceptanceRevision, second.AcceptanceRevision)
	r := repo.NewDatasetRepository(db, scope)
	head, err := r.Head(ctx, in.OwnerID, in.BindingUUID, in.Declaration.DatasetKey)
	require.NoError(t, err)
	require.Equal(t, first.RequestUUID, *head.RequestUUID)
	require.Equal(t, in.TenantUUID, head.ServiceTenantUUID)

	unrelated := in
	unrelated.CallerPluginID = "com.other.plugin"
	_, err = s.AdmitVerifiedDataset(ctx, unrelated)
	require.Equal(t, "DATASET_OWNER_REQUIRED", dto.CodeOf(err))
	unrelated = in
	unrelated.TenantUUID = uuid.New()
	_, err = s.AdmitVerifiedDataset(ctx, unrelated)
	require.Equal(t, "DATASET_REQUEST_CONFLICT", dto.CodeOf(err))
	unrelated = in
	unrelated.RequestUUID = uuid.NewString()
	unrelated.DefinitionDigest = strings.Repeat("d", 64)
	_, err = s.AdmitVerifiedDataset(ctx, unrelated)
	require.Equal(t, "DATASET_DEFINITION_CONFLICT", dto.CodeOf(err))

	drift := in
	drift.RequestUUID = uuid.NewString()
	drift.SourceFingerprint = strings.Repeat("c", 64)
	changed, err := s.AdmitVerifiedDataset(ctx, drift)
	require.NoError(t, err)
	require.Equal(t, first.DatasetUUID, changed.DatasetUUID)
	require.Equal(t, first.AcceptanceRevision+1, changed.AcceptanceRevision)
	require.Equal(t, "needs_revalidation", changed.RegistrationState)
	replay, err := s.AdmitVerifiedDataset(ctx, in)
	require.NoError(t, err)
	require.Equal(t, first, replay)

	_, err = s.DatasetCatalog(ctx)
	require.ErrorIs(t, err, ErrRootRequired)
	entries, err := s.DatasetCatalog(controlRoot())
	require.NoError(t, err)
	probe, fixture := 0, 0
	for _, entry := range entries {
		switch entry.DatasetKey {
		case repo.ProbeRetentionID:
			probe++
			require.Equal(t, "system_table_retention", entry.ExistingPolicy)
		case "fixture_report":
			fixture++
			require.Equal(t, changed.DatasetUUID, entry.DatasetUUID)
			require.Equal(t, changed.RegistrationState, entry.RegistrationState)
		}
	}
	require.Equal(t, 1, probe)
	require.Equal(t, 1, fixture)

	require.NoError(t, db.Exec("CREATE TABLE "+coremodel.PowerXSchema+".fixture_reports (id integer)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+coremodel.PowerXSchema+".fixture_reports VALUES (1)").Error)
	metrics, err := repo.ObserveDatasetRelations(ctx, db, "owned-fixture", coremodel.PowerXSchema, []string{"fixture_reports"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	deduplicated, err := repo.SumDatasetPhysicalBytes(append(metrics, metrics...))
	require.NoError(t, err)
	require.Positive(t, deduplicated)
	require.Equal(t, metrics[0].Bytes, deduplicated)
	_, err = repo.ObserveDatasetRelations(ctx, db, "owned-fixture", coremodel.PowerXSchema, []string{"fixture_missing"})
	require.Error(t, err)
	require.NoError(t, db.Table(coremodel.PowerXSchema+".fixture_reports").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
