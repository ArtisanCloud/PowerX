package customer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestMiniAppPhoneChangedPhoneMustLoseVerification(t *testing.T) {
	service, _ := miniAppPhoneTestService(t)
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
	_, membership, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/review-change")
	require.NoError(t, err)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, membership.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
	require.NoError(t, err)
	var account modelcustomer.Account
	require.NoError(t, service.db.Where("uuid = ?", membership.CustomerUUID).First(&account).Error)
	version := customerrepo.AccountProfileVersion(membership.CustomerUUID, account.DisplayName, account.Nickname, account.PrimaryEmail, account.PrimaryPhone)
	require.NoError(t, service.db.Transaction(func(tx *gorm.DB) error {
		return customerrepo.NewAccountRepository(tx).UpdateAdminProfile(ctx, miniAppTestTenant, membership.CustomerUUID, version, map[string]any{"primary_phone": "+8613900005678"})
	}))
	require.NoError(t, service.db.Where("uuid = ?", membership.CustomerUUID).First(&account).Error)
	require.Equal(t, "+8613900005678", account.PrimaryPhone)
	verified, err := service.PhoneVerified(ctx, membership.CustomerUUID)
	require.NoError(t, err)
	require.False(t, verified, "未经认证的新号码不能继承旧号码的认证")
}

func TestMiniAppPhoneInactiveCustomerCannotVerifyPhone(t *testing.T) {
	for _, kind := range []string{"membership_suspended", "account_suspended", "membership_expired"} {
		t.Run(kind, func(t *testing.T) {
			service, _ := miniAppPhoneTestService(t)
			ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
			pair, membership, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/review-inactive")
			require.NoError(t, err)
			switch kind {
			case "membership_suspended":
				require.NoError(t, service.db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", membership.MembershipUUID).Update("status", "suspended").Error)
			case "account_suspended":
				require.NoError(t, service.db.Model(&modelcustomer.Account{}).Where("uuid = ?", membership.CustomerUUID).Update("status", "suspended").Error)
			case "membership_expired":
				require.NoError(t, service.db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", membership.MembershipUUID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			}
			_, err = service.tokens.Validate(ctx, pair.AccessToken)
			require.NoError(t, err, "旧令牌仍通过会话校验")
			_, err = service.memberships.ResolveCurrent(ctx, miniAppTestTenant, membership.CustomerUUID)
			require.ErrorIs(t, err, ErrCustomerMembershipInactive, "正常成员校验应拒绝该客户")
			_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, membership.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
			require.ErrorIs(t, err, ErrCustomerMembershipInactive, "手机号认证必须在写入前拒绝无效客户")
			var account modelcustomer.Account
			require.NoError(t, service.db.Where("uuid = ?", membership.CustomerUUID).First(&account).Error)
			require.Empty(t, account.PrimaryPhone)
		})
	}
}

func TestMiniAppPhoneProfileUpdatesInvalidateEvidence(t *testing.T) {
	for _, admin := range []bool{false, true} {
		name := "service_profile"
		if admin {
			name = "admin_profile"
		}
		t.Run(name, func(t *testing.T) {
			service, _ := miniAppPhoneTestService(t)
			ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
			_, m, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/profile")
			require.NoError(t, err)
			require.NoError(t, service.db.Model(&modelcustomer.Account{}).Where("uuid = ?", m.CustomerUUID).Update("metadata", datatypes.JSON(`{"custom":"preserved"}`)).Error)
			_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
			require.NoError(t, err)
			update := func(fields map[string]any) {
				t.Helper()
				require.NoError(t, service.db.Transaction(func(tx *gorm.DB) error {
					repo := customerrepo.NewAccountRepository(tx)
					if !admin {
						return repo.UpdateProfile(ctx, miniAppTestTenant, m.CustomerUUID, fields)
					}
					var a modelcustomer.Account
					if err := tx.Where("uuid = ?", m.CustomerUUID).First(&a).Error; err != nil {
						return err
					}
					return repo.UpdateAdminProfile(ctx, miniAppTestTenant, m.CustomerUUID, customerrepo.AccountProfileVersion(m.CustomerUUID, a.DisplayName, a.Nickname, a.PrimaryEmail, a.PrimaryPhone), fields)
				}))
			}
			update(map[string]any{"nickname": "updated"})
			update(map[string]any{"primary_phone": "+8613800001234"})
			verified, err := service.PhoneVerified(ctx, m.CustomerUUID)
			require.NoError(t, err)
			require.True(t, verified, "无改号的资料编辑保留认证")
			for _, phone := range []string{"+8613900005678", "+8613800001234", ""} {
				update(map[string]any{"primary_phone": phone})
				verified, err := service.PhoneVerified(ctx, m.CustomerUUID)
				require.NoError(t, err)
				require.False(t, verified, "改号、改回旧号和清空都需要重新认证")
			}
			var a modelcustomer.Account
			require.NoError(t, service.db.Where("uuid = ?", m.CustomerUUID).First(&a).Error)
			var metadata map[string]any
			require.NoError(t, json.Unmarshal(a.Metadata, &metadata))
			require.Equal(t, "preserved", metadata["custom"])
			_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
			require.NoError(t, err)
			verified, err = service.PhoneVerified(ctx, m.CustomerUUID)
			require.NoError(t, err)
			require.True(t, verified, "重新认证恢复有效证据")
		})
	}
}

type miniAppPhoneVerifierFunc func(context.Context, ClientApp, string) (PhoneInfo, error)

func (f miniAppPhoneVerifierFunc) VerifyPhone(ctx context.Context, app ClientApp, code string) (PhoneInfo, error) {
	return f(ctx, app, code)
}

func TestMiniAppPhoneRechecksMembershipAfterProvider(t *testing.T) {
	service, _ := miniAppPhoneTestService(t)
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
	_, m, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/revoked-during-request")
	require.NoError(t, err)
	service.phoneVerifiers[WeChatMiniAppChannel] = miniAppPhoneVerifierFunc(func(context.Context, ClientApp, string) (PhoneInfo, error) {
		require.NoError(t, service.db.Model(&modelcustomer.TenantMembership{}).Where("uuid = ?", m.MembershipUUID).Update("status", "suspended").Error)
		return PhoneInfo{PurePhone: "13800001234", CountryCode: "86"}, nil
	})
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "code")
	require.ErrorIs(t, err, ErrCustomerMembershipInactive)
	var account modelcustomer.Account
	require.NoError(t, service.db.Where("uuid = ?", m.CustomerUUID).First(&account).Error)
	require.Empty(t, account.PrimaryPhone)
}
