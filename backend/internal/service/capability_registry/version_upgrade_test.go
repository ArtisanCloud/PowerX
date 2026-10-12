package capability_registry

import (
	"context"
	"errors"
	"strings"
	"testing"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type upgradeTestStore struct {
	hash  string
	apply func() error
}

func (s *upgradeTestStore) CurrentHash(context.Context, string, string) (string, bool) {
	return s.hash, s.hash != ""
}
func (s *upgradeTestStore) ConfirmUpgrade(_ context.Context, _, _, hash string) error {
	if s.apply != nil {
		if err := s.apply(); err != nil {
			return err
		}
	}
	s.hash = hash
	return nil
}

func TestVersionUpgradeRequiresAdminCurrentPublicationAndDurableAudit(t *testing.T) {
	db := newGrantStatusTestDB(t)
	require.NoError(t, db.AutoMigrate(&audit.AuditEvent{}))
	capID := "com.corex.customer.accounts.service_manage"
	seedGrantStatusCapability(t, db, capID, true)
	hash := strings.Repeat("a", 64)
	require.NoError(t, db.Model(&capm.CapabilityRecord{}).Where("capability_id = ?", capID).Update("capabilities_hash", hash).Error)
	store := &upgradeTestStore{hash: strings.Repeat("b", 64)}
	svc := NewVersionUpgradeService(db, store)
	input := VersionUpgradeInput{TenantUUID: grantStatusTestTenant, CapabilityID: capID, CapabilitiesHash: hash, Reason: "customer contract update"}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, UserUUID: uuid.NewString(), TenantUUID: grantStatusTestTenant, Roles: []string{"role_admin"}})
	for _, claims := range []*reqctx.CoreXClaims{nil, {UserID: 1, TenantUUID: grantStatusTestTenant}, {UserID: 1, TenantUUID: uuid.NewString(), Roles: []string{"role_admin"}}, {UserID: 1, IsRoot: true, PluginID: "plugin"}} {
		_, err := svc.Confirm(reqctx.WithClaims(context.Background(), claims), input)
		require.ErrorIs(t, err, ErrVersionUpgradeForbidden)
	}
	wrong := input
	wrong.CapabilitiesHash = strings.Repeat("c", 64)
	_, err := svc.Confirm(ctx, wrong)
	require.ErrorIs(t, err, ErrVersionUpgradeHashMismatch)
	var count int64
	require.NoError(t, db.Model(&audit.AuditEvent{}).Count(&count).Error)
	require.Zero(t, count)
	store.apply = func() error {
		var row audit.AuditEvent
		require.NoError(t, db.Order("id DESC").First(&row).Error)
		require.Equal(t, "APPROVED", row.Outcome)
		return nil
	}
	out, err := svc.Confirm(ctx, input)
	require.NoError(t, err)
	require.Equal(t, hash, store.hash)
	require.Equal(t, strings.Repeat("b", 64), out.PreviousHash)
	var event audit.AuditEvent
	require.NoError(t, db.First(&event, out.AuditID).Error)
	require.Equal(t, "SUCCESS", event.Outcome)
	require.EqualValues(t, 1, *event.ActorUserID)
	require.Contains(t, string(event.ChangesAfter), hash)
	_, err = svc.Confirm(ctx, input)
	require.NoError(t, err)
	require.Equal(t, hash, store.hash)
	// Latest disabled registration must win over historical published registration.
	require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: capID, TenantUUID: grantStatusTestTenant, ContractRef: "test", Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	_, err = svc.Confirm(ctx, input)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestVersionUpgradeCannotApplyWithoutAudit(t *testing.T) {
	db := newGrantStatusTestDB(t)
	capID := "com.corex.test.upgrade"
	seedGrantStatusCapability(t, db, capID, true)
	hash := strings.Repeat("a", 64)
	require.NoError(t, db.Model(&capm.CapabilityRecord{}).Where("capability_id = ?", capID).Update("capabilities_hash", hash).Error)
	called := false
	store := &upgradeTestStore{hash: "old", apply: func() error { called = true; return errors.New("must not apply") }}
	ctx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true})
	_, err := NewVersionUpgradeService(db, store).Confirm(ctx, VersionUpgradeInput{TenantUUID: grantStatusTestTenant, CapabilityID: capID, CapabilitiesHash: hash, Reason: "test"})
	require.Error(t, err)
	require.False(t, called)
	require.Equal(t, "old", store.hash)
}
