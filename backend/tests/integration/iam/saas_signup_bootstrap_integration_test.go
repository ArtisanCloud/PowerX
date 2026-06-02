package iamintegration

import (
	"context"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
)

func TestSaaSSignupBootstrapCreatesTenantOwnerAndTokens(t *testing.T) {
	fx := setupIAMFixture(t)
	svc := authsvc.NewSaaSSignupService(fx.DB, authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	}))

	result, err := svc.Signup(context.Background(), authsvc.SaaSSignupInput{
		TenantName:       "Acme Inc",
		OwnerEmail:       "owner@example.com",
		OwnerPassword:    "secret123",
		OwnerDisplayName: "Owner",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
	require.Equal(t, "acme-inc", result.Tenant.Key)
	require.Equal(t, "acme-inc.tenant.powerx.local", result.Tenant.Domain)
	require.Equal(t, "owner@example.com", result.User.Email)
	require.Equal(t, result.Tenant.UUID.String(), result.Member.TenantUUID)

	var roles []modeliam.Role
	require.NoError(t, fx.DB.Where("tenant_uuid = ?", result.Tenant.UUID.String()).Find(&roles).Error)
	roleCodes := map[string]bool{}
	for _, role := range roles {
		roleCodes[string(role.Code)] = true
	}
	require.True(t, roleCodes["role_owner"])
	require.True(t, roleCodes["role_admin"])
	require.True(t, roleCodes["role_user"])

	var bindings []modeliam.RoleBinding
	require.NoError(t, fx.DB.Where("tenant_uuid = ? AND subject_id = ?", result.Tenant.UUID.String(), result.Member.ID).Find(&bindings).Error)
	require.Len(t, bindings, 3)

	var profile modeliam.APIKeyProfile
	require.NoError(t, fx.DB.Where("tenant_uuid = ?", result.Tenant.UUID.String()).First(&profile).Error)
	require.NotNil(t, profile.OwnerMemberID)
	require.Equal(t, result.Member.ID, *profile.OwnerMemberID)

	var keyPair modeltenant.TenantKeyPair
	require.NoError(t, fx.DB.Where("tenant_uuid = ? AND active = ?", result.Tenant.UUID.String(), true).First(&keyPair).Error)
	require.NotEmpty(t, keyPair.PublicPEM)
}

func TestSaaSSignupWithVerifierRequiresCode(t *testing.T) {
	fx := setupIAMFixture(t)
	verifier := authsvc.NewSignupVerificationService(authsvc.LocalSignupVerificationDriver{}, time.Minute)
	require.NoError(t, verifier.IssueForTest("verified@example.com", "123456", time.Minute))
	svc := authsvc.NewSaaSSignupService(fx.DB, authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	}), authsvc.SaaSSignupOptions{Verifier: verifier})

	_, err := svc.Signup(context.Background(), authsvc.SaaSSignupInput{
		TenantName:       "Verified Inc",
		OwnerEmail:       "verified@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "000000",
	})
	require.ErrorIs(t, err, authsvc.ErrSignupVerificationCodeInvalid)

	result, err := svc.Signup(context.Background(), authsvc.SaaSSignupInput{
		TenantName:       "Verified Inc",
		OwnerEmail:       "verified@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "123456",
	})
	require.NoError(t, err)
	require.Equal(t, "verified-inc", result.Tenant.Key)
}

func TestSaaSSignupExistingUserWithValidPasswordCreatesSecondTenant(t *testing.T) {
	fx := setupIAMFixture(t)
	auth := authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	})
	verifier := authsvc.NewSignupVerificationService(authsvc.LocalSignupVerificationDriver{}, time.Minute)
	require.NoError(t, verifier.IssueForTest("owner2@example.com", "111111", time.Minute))
	svc := authsvc.NewSaaSSignupService(fx.DB, auth, authsvc.SaaSSignupOptions{Verifier: verifier})
	ctx := context.Background()

	first, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:       "Tenant One",
		OwnerEmail:       "owner2@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "111111",
	})
	require.NoError(t, err)
	require.NoError(t, verifier.IssueForTest("owner2@example.com", "222222", time.Minute))
	second, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:       "Tenant Two",
		OwnerEmail:       "owner2@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "222222",
	})
	require.NoError(t, err)
	require.Equal(t, first.User.ID, second.User.ID)
	require.NotEqual(t, first.Member.ID, second.Member.ID)

	var memberCount int64
	require.NoError(t, fx.DB.Model(&modeliam.Member{}).Where("user_id = ?", first.User.ID).Count(&memberCount).Error)
	require.EqualValues(t, 2, memberCount)
}

func TestLoginWithMultipleTenantsUsesLastTenant(t *testing.T) {
	fx := setupIAMFixture(t)
	auth := authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	})
	svc := authsvc.NewSaaSSignupService(fx.DB, auth)
	ctx := context.Background()

	first, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:    "Login Tenant One",
		OwnerEmail:    "multi-login@example.com",
		OwnerPassword: "secret123",
	})
	require.NoError(t, err)
	second, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:    "Login Tenant Two",
		OwnerEmail:    "multi-login@example.com",
		OwnerPassword: "secret123",
	})
	require.NoError(t, err)
	require.Equal(t, first.User.ID, second.User.ID)

	require.NoError(t, auth.UserRepo.UpdateLastTenantUUID(ctx, first.User.ID, second.Tenant.UUID.String()))
	access, _, err := auth.Login(ctx, "", "multi-login@example.com", "secret123")
	require.NoError(t, err)
	claims, err := pkgauth.ParseAndValidate(access, []byte("0123456789abcdef0123456789abcdef"), "powerx-test", "powerx-user")
	require.NoError(t, err)
	require.Equal(t, second.Tenant.UUID.String(), claims.TenantUUID)
	require.Equal(t, second.Member.ID, claims.MemberID)
}

func TestSaaSSignupDuplicateTenantNameAllocatesUniqueKeyAndDomain(t *testing.T) {
	fx := setupIAMFixture(t)
	svc := authsvc.NewSaaSSignupService(fx.DB, authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	}))
	ctx := context.Background()

	first, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:    "Duplicate Name",
		OwnerEmail:    "first-duplicate@example.com",
		OwnerPassword: "secret123",
	})
	require.NoError(t, err)
	second, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:    "Duplicate Name",
		OwnerEmail:    "second-duplicate@example.com",
		OwnerPassword: "secret123",
	})
	require.NoError(t, err)

	require.Equal(t, "duplicate-name", first.Tenant.Key)
	require.Equal(t, "duplicate-name-2", second.Tenant.Key)
	require.Equal(t, "duplicate-name.tenant.powerx.local", first.Tenant.Domain)
	require.Equal(t, "duplicate-name-2.tenant.powerx.local", second.Tenant.Domain)
}
