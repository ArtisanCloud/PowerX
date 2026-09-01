package iamintegration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInviteCodeConsumeTransaction(t *testing.T) {
	t.Run("success consumes code while preserving root-visible plaintext", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		code := "PX-INTERNAL-001"
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		createInviteCode(t, db, batch.UUID.String(), code)

		consumed, err := authsvc.NewInviteCodeService(db).Consume(context.Background(), nil, authsvc.InviteCodeConsumeInput{
			Code:       code,
			Email:      "owner@example.com",
			Channel:    "internal_beta",
			Plan:       "free",
			TenantUUID: uuid.NewString(),
		})
		require.NoError(t, err)
		require.Equal(t, modeliam.RegistrationInviteCodeStatusConsumed, consumed.Status)
		require.Equal(t, 1, consumed.UseCount)
		require.Equal(t, code, consumed.PlainCode)
		require.NotContains(t, consumed.CodeHash, code)
	})

	t.Run("generate stores plaintext for root batch code list", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		svc := authsvc.NewInviteCodeService(db)

		plain, err := svc.GenerateCodes(context.Background(), batch.UUID.String(), 3)
		require.NoError(t, err)
		require.Len(t, plain, 3)

		items, err := svc.ListCodes(context.Background(), batch.UUID.String(), 100)
		require.NoError(t, err)
		require.Len(t, items, 3)
		require.ElementsMatch(t, plain, []string{items[0].PlainCode, items[1].PlainCode, items[2].PlainCode})
		for _, item := range items {
			require.NotEmpty(t, item.PlainCode)
			require.NotContains(t, item.CodeHash, item.PlainCode)
		}
	})

	t.Run("reset missing plaintext only regenerates unused active codes", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		missing := createInviteCode(t, db, batch.UUID.String(), "PX-OLD-HASH-ONLY")
		missing.PlainCode = ""
		require.NoError(t, db.Save(missing).Error)
		used := createInviteCode(t, db, batch.UUID.String(), "PX-USED-HASH-ONLY")
		used.PlainCode = ""
		used.UseCount = 1
		require.NoError(t, db.Save(used).Error)

		items, err := authsvc.NewInviteCodeService(db).ResetMissingPlainCodes(context.Background(), batch.UUID.String())
		require.NoError(t, err)

		var reset modeliam.RegistrationInviteCode
		require.NoError(t, db.Where("uuid = ?", missing.UUID).First(&reset).Error)
		require.NotEmpty(t, reset.PlainCode)
		require.Equal(t, authsvc.HashInviteCode(reset.PlainCode), reset.CodeHash)

		var unchanged modeliam.RegistrationInviteCode
		require.NoError(t, db.Where("uuid = ?", used.UUID).First(&unchanged).Error)
		require.Empty(t, unchanged.PlainCode)
		require.Equal(t, 1, unchanged.UseCount)
		require.Len(t, items, 2)
	})

	t.Run("delete batches removes unused codes and rejects used codes", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		unusedBatch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		createInviteCode(t, db, unusedBatch.UUID.String(), "PX-DELETE-UNUSED")
		svc := authsvc.NewInviteCodeService(db)

		deleted, err := svc.DeleteBatches(context.Background(), []string{unusedBatch.UUID.String()})
		require.NoError(t, err)
		require.Equal(t, int64(1), deleted)
		var batchCount int64
		require.NoError(t, db.Model(&modeliam.RegistrationInviteBatch{}).Where("uuid = ?", unusedBatch.UUID).Count(&batchCount).Error)
		require.Equal(t, int64(0), batchCount)
		var codeCount int64
		require.NoError(t, db.Model(&modeliam.RegistrationInviteCode{}).Where("batch_uuid = ?", unusedBatch.UUID.String()).Count(&codeCount).Error)
		require.Equal(t, int64(0), codeCount)

		usedBatch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		usedCode := createInviteCode(t, db, usedBatch.UUID.String(), "PX-DELETE-USED")
		usedCode.UseCount = 1
		require.NoError(t, db.Save(usedCode).Error)

		_, err = svc.DeleteBatches(context.Background(), []string{usedBatch.UUID.String()})
		require.ErrorIs(t, err, authsvc.ErrRegistrationInviteInvalid)
		require.NoError(t, db.Model(&modeliam.RegistrationInviteBatch{}).Where("uuid = ?", usedBatch.UUID).Count(&batchCount).Error)
		require.Equal(t, int64(1), batchCount)
	})

	t.Run("duplicate submit is rejected after max uses", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		code := "PX-INTERNAL-002"
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		createInviteCode(t, db, batch.UUID.String(), code)
		svc := authsvc.NewInviteCodeService(db)

		_, err := svc.Consume(context.Background(), nil, authsvc.InviteCodeConsumeInput{Code: code, Email: "owner@example.com", Channel: "internal_beta"})
		require.NoError(t, err)
		_, err = svc.Consume(context.Background(), nil, authsvc.InviteCodeConsumeInput{Code: code, Email: "owner@example.com", Channel: "internal_beta"})
		require.ErrorIs(t, err, authsvc.ErrRegistrationInviteUnavailable)
	})

	t.Run("paused batch rejects consumption", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		code := "PX-INTERNAL-003"
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusPaused)
		createInviteCode(t, db, batch.UUID.String(), code)

		_, err := authsvc.NewInviteCodeService(db).Consume(context.Background(), nil, authsvc.InviteCodeConsumeInput{Code: code, Email: "owner@example.com", Channel: "internal_beta"})
		require.ErrorIs(t, err, authsvc.ErrRegistrationInviteUnavailable)
	})

	t.Run("rollback does not consume code", func(t *testing.T) {
		db := setupRegistrationInviteDB(t)
		code := "PX-INTERNAL-004"
		batch := createInviteBatch(t, db, modeliam.RegistrationInviteBatchStatusActive)
		createInviteCode(t, db, batch.UUID.String(), code)
		svc := authsvc.NewInviteCodeService(db)
		rollbackErr := errors.New("force rollback")

		err := db.Transaction(func(tx *gorm.DB) error {
			_, err := svc.Consume(context.Background(), tx, authsvc.InviteCodeConsumeInput{Code: code, Email: "owner@example.com", Channel: "internal_beta"})
			require.NoError(t, err)
			return rollbackErr
		})
		require.ErrorIs(t, err, rollbackErr)

		var got modeliam.RegistrationInviteCode
		require.NoError(t, db.Where("code_hash = ?", authsvc.HashInviteCode(code)).First(&got).Error)
		require.Equal(t, modeliam.RegistrationInviteCodeStatusActive, got.Status)
		require.Equal(t, 0, got.UseCount)
	})
}

func setupRegistrationInviteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&modeliam.RegistrationInviteBatch{}, &modeliam.RegistrationInviteCode{}))
	return db
}

func createInviteBatch(t *testing.T, db *gorm.DB, status string) *modeliam.RegistrationInviteBatch {
	t.Helper()
	now := time.Now().UTC()
	batch := &modeliam.RegistrationInviteBatch{
		PowerUUIDModel:      modelbase.PowerUUIDModel{UUID: uuid.New()},
		Name:                "Internal Beta",
		Status:              status,
		MaxCodes:            10,
		MaxUsesPerCode:      1,
		AllowedEmailDomains: datatypes.JSON([]byte(`["example.com"]`)),
		AllowedChannels:     datatypes.JSON([]byte(`["internal_beta"]`)),
		CreatedByUserUUID:   uuid.NewString(),
		UpdatedByUserUUID:   uuid.NewString(),
		StartsAt:            &now,
	}
	require.NoError(t, db.Create(batch).Error)
	return batch
}

func createInviteCode(t *testing.T, db *gorm.DB, batchUUID string, code string) *modeliam.RegistrationInviteCode {
	t.Helper()
	record := &modeliam.RegistrationInviteCode{
		PowerUUIDModel: modelbase.PowerUUIDModel{UUID: uuid.New()},
		BatchUUID:      batchUUID,
		PlainCode:      code,
		CodeHash:       authsvc.HashInviteCode(code),
		Status:         modeliam.RegistrationInviteCodeStatusActive,
		MaxUses:        1,
	}
	require.NoError(t, db.Create(record).Error)
	return record
}
