package iamintegration

import (
	"context"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/stretchr/testify/require"
)

func TestSaaSSignupRejectsExistingUserWrongPasswordWithoutTenantResidue(t *testing.T) {
	fx := setupIAMFixture(t)
	verifier := authsvc.NewSignupVerificationService(authsvc.LocalSignupVerificationDriver{}, time.Minute)
	require.NoError(t, verifier.IssueForTest("rollback@example.com", "111111", time.Minute))
	svc := authsvc.NewSaaSSignupService(fx.DB, authsvc.NewAuthService(fx.DB, authsvc.AuthOptions{
		JWTSecret:  []byte("0123456789abcdef0123456789abcdef"),
		Issuer:     "powerx-test",
		Audience:   "powerx-user",
		Platforms:  []string{"web"},
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
		DefaultEnv: "test",
	}), authsvc.SaaSSignupOptions{Verifier: verifier})
	ctx := context.Background()

	_, err := svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:       "First",
		OwnerEmail:       "rollback@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "111111",
	})
	require.NoError(t, err)
	require.NoError(t, verifier.IssueForTest("rollback@example.com", "222222", time.Minute))

	_, err = svc.Signup(ctx, authsvc.SaaSSignupInput{
		TenantName:       "Second",
		OwnerEmail:       "rollback@example.com",
		OwnerPassword:    "wrong-password",
		VerificationCode: "222222",
	})
	require.ErrorIs(t, err, authsvc.ErrSaaSSignupInvalidCredentials)

	var tenants int64
	require.NoError(t, fx.DB.Model(&modeltenant.Tenant{}).Where("key = ?", "second").Count(&tenants).Error)
	require.Zero(t, tenants)
}

func TestSaaSSignupDuplicateTenantKeyDoesNotCreateExtraMember(t *testing.T) {
	fx := setupIAMFixture(t)
	verifier := authsvc.NewSignupVerificationService(authsvc.LocalSignupVerificationDriver{}, time.Minute)
	require.NoError(t, verifier.IssueForTest("broken@example.com", "111111", time.Minute))
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
		TenantKey:        "broken",
		TenantName:       "Broken",
		OwnerEmail:       "broken@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "111111",
	})
	require.NoError(t, err)
	require.NoError(t, verifier.IssueForTest("broken@example.com", "222222", time.Minute))

	_, err = svc.Signup(context.Background(), authsvc.SaaSSignupInput{
		TenantKey:        "broken",
		TenantName:       "Broken Duplicate",
		OwnerEmail:       "broken@example.com",
		OwnerPassword:    "secret123",
		VerificationCode: "222222",
	})
	require.ErrorIs(t, err, authsvc.ErrSaaSSignupTenantKeyExists)

	var tenants int64
	require.NoError(t, fx.DB.Model(&modeltenant.Tenant{}).Where("key = ?", "broken").Count(&tenants).Error)
	require.EqualValues(t, 1, tenants)

	var members int64
	require.NoError(t, fx.DB.Model(&modeliam.Member{}).Count(&members).Error)
	require.EqualValues(t, 1, members)
}
