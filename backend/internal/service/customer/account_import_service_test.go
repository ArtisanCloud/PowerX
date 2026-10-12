package customer

import (
	"context"
	"strings"
	"testing"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func importAdminContext(tenant string) context.Context {
	ctx := reqctx.WithTenantUUID(context.Background(), tenant)
	ctx = reqctx.WithIsRoot(ctx, true)
	ctx = context.WithValue(ctx, reqctx.KeyUserID, uint64(42))
	return reqctx.WithTraceID(ctx, "customer-import-test-trace")
}

func TestCustomerImportPreviewCommitAndReplay(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&modelcustomer.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewAccountImportService(db)
	csv := "type,display_name,primary_email,primary_phone,primary_contact_name,primary_contact_email,primary_contact_phone\n" +
		"person,Ada,ada@example.test,,,,\n" +
		"company,Acme,company@example.test,,Jane,jane@example.test,\n" +
		"person,Duplicate,ADA@example.test,,,,\n" +
		"company,Missing contact,missing@example.test,,,,\n" +
		"person,Invalid,bad-address,,,,\n"
	var before int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Count(&before).Error)
	preview, err := s.Preview(ctx, "customers.csv", csv)
	require.NoError(t, err)
	require.Equal(t, 2, preview.Ready)
	require.Equal(t, 3, preview.Skipped)
	require.Equal(t, "CUSTOMER_IMPORT_DUPLICATE_IN_FILE", preview.Rows[2].ErrorCode)
	require.Equal(t, "CUSTOMER_IMPORT_PRIMARY_CONTACT_REQUIRED", preview.Rows[3].ErrorCode)
	var after int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Count(&after).Error)
	require.Equal(t, before, after, "preview must not create Customer data")
	_, err = s.Commit(ctx, preview.UUID, strings.Repeat("a", 64), true)
	require.Equal(t, "CUSTOMER_IMPORT_PREVIEW_CHANGED", dto.CodeOf(err))
	_, err = s.Commit(ctx, preview.UUID, preview.Checksum, false)
	require.Equal(t, "CUSTOMER_IMPORT_CONFIRM_REQUIRED", dto.CodeOf(err))
	result, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)
	require.Equal(t, 2, result.Created)
	require.Equal(t, 3, result.Skipped)
	require.Zero(t, result.Failed)
	for _, row := range result.Rows[:2] {
		require.NotEmpty(t, row.CustomerUUID)
		require.NotEmpty(t, row.PrimaryContactUUID)
		contact, err := NewContactService(db).Get(ctx, tenant, row.CustomerUUID, row.PrimaryContactUUID)
		require.NoError(t, err)
		if row.Input.Type == "company" {
			require.Equal(t, "Jane", contact.DisplayName)
		}
	}
	replayed, err := NewAccountImportService(db).Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, result, replayed)
	read, err := NewAccountImportService(db).Get(ctx, result.UUID)
	require.NoError(t, err)
	require.Equal(t, result, read)
	require.NoError(t, db.Model(&modelcustomer.Account{}).Count(&after).Error)
	require.Equal(t, before+2, after)
	second, err := s.Preview(ctx, "customers.csv", csv)
	require.NoError(t, err)
	require.Zero(t, second.Ready)
	require.Equal(t, "CUSTOMER_IMPORT_DUPLICATE_EXISTING", second.Rows[0].ErrorCode)
	_, err = s.Get(importAdminContext(uuid.NewString()), preview.UUID)
	require.Equal(t, 404, dto.StatusCode(err))
	otherActor := context.WithValue(ctx, reqctx.KeyUserID, uint64(43))
	_, err = s.Get(otherActor, preview.UUID)
	require.Equal(t, 404, dto.StatusCode(err))
}

func TestCustomerImportExpiredAndChangedExistingData(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&modelcustomer.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewAccountImportService(db)
	csv := "display_name,primary_email\nAda,ada@example.test\n"
	expired, err := s.Preview(ctx, "customers.csv", csv)
	require.NoError(t, err)
	require.NoError(t, db.Model(&modelcustomer.AccountImport{}).Where("uuid = ?", expired.UUID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	_, err = s.Commit(ctx, expired.UUID, expired.Checksum, true)
	require.Equal(t, "CUSTOMER_IMPORT_EXPIRED", dto.CodeOf(err))
	preview, err := s.Preview(ctx, "customers.csv", csv)
	require.NoError(t, err)
	_, err = NewAccountService(db).Create(ctx, CreateAccountInput{TenantUUID: tenant, DisplayName: "Created meanwhile", PrimaryEmail: "ada@example.test"})
	require.NoError(t, err)
	result, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Zero(t, result.Created)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, "CUSTOMER_IMPORT_DUPLICATE_EXISTING", result.Rows[0].ErrorCode)
}

func TestCustomerImportRowFailureRollsBackWholeCustomer(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&modelcustomer.AccountImport{}))
	ctx := importAdminContext(tenant)
	s := NewAccountImportService(db)
	preview, err := s.Preview(ctx, "customers.csv", "display_name,primary_email\nGood,good@example.test\nFail,fail@example.test\n")
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER customer_import_contact_failure BEFORE INSERT ON customer_contacts WHEN NEW.display_name='Fail' BEGIN SELECT RAISE(ABORT,'injected'); END").Error)
	result, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Failed)
	var accounts int64
	require.NoError(t, db.Model(&modelcustomer.Account{}).Where("primary_email = ?", "fail@example.test").Count(&accounts).Error)
	require.Zero(t, accounts)
	require.NoError(t, db.Exec("DROP TRIGGER customer_import_contact_failure").Error)
	retry, err := s.Commit(ctx, preview.UUID, preview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, result, retry, "completed partial results must remain idempotent")
	newPreview, err := s.Preview(ctx, "retry.csv", "display_name,primary_email\nFail,fail@example.test\n")
	require.NoError(t, err)
	retry, err = s.Commit(ctx, newPreview.UUID, newPreview.Checksum, true)
	require.NoError(t, err)
	require.Equal(t, 1, retry.Created)
}

func TestCustomerImportRejectsCredentialPlanesAndUnsupportedCSV(t *testing.T) {
	db, tenant, _ := newContactServiceTestDB(t)
	s := NewAccountImportService(db)
	for _, plane := range []string{"api_key", "powerx-sts"} {
		claims := &reqctx.CoreXClaims{}
		if plane == "api_key" {
			claims.Platforms = []string{"api_key"}
		} else {
			claims.Issuer = plane
		}
		_, err := s.Preview(reqctx.WithClaims(importAdminContext(tenant), claims), "customers.csv", "display_name,primary_email\nAda,ada@example.test\n")
		require.Equal(t, 403, dto.StatusCode(err))
	}
	for _, csv := range []string{"display_name,tenant_uuid\nAda,tenant\n", "display_name,primary_email,password\nAda,a@example.test,pwd\n", "display_name,display_name,primary_email\nAda,Ada,a@example.test\n", "display_name,primary_email\n\xff,a@example.test\n", "display_name,primary_email\n\"broken,a@example.test"} {
		_, err := parseCustomerImportCSV(csv)
		require.Equal(t, 400, dto.StatusCode(err))
	}
	_, err := parseCustomerImportCSV("display_name,primary_email\n" + strings.Repeat("Ada,a@example.test\n", 201))
	require.Equal(t, 413, dto.StatusCode(err))
	_, err = parseCustomerImportCSV(strings.Repeat("a", CustomerImportMaxBytes+1))
	require.Equal(t, 413, dto.StatusCode(err))
}
