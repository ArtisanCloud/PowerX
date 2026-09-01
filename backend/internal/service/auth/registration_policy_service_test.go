package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegistrationPolicyServiceEvaluateModes(t *testing.T) {
	cases := []struct {
		name             string
		mode             string
		input            RegistrationPolicyEvaluateInput
		rules            []registrationPolicyRule
		canSignup        bool
		canSubmitRequest bool
		reasonCode       string
	}{
		{name: "closed", mode: modeliam.RegistrationPolicyModeClosed, canSignup: false, reasonCode: "registration_closed"},
		{name: "open", mode: modeliam.RegistrationPolicyModeOpen, canSignup: true},
		{name: "invite only rejects missing code", mode: modeliam.RegistrationPolicyModeInviteOnly, canSignup: false, reasonCode: "invite_code_required"},
		{name: "invite only accepts present code", mode: modeliam.RegistrationPolicyModeInviteOnly, input: RegistrationPolicyEvaluateInput{InviteCode: "PX-123"}, canSignup: true},
		{name: "waitlist", mode: modeliam.RegistrationPolicyModeWaitlist, canSubmitRequest: true, reasonCode: "waitlist_required"},
		{name: "approval required", mode: modeliam.RegistrationPolicyModeApprovalRequired, canSubmitRequest: true, reasonCode: "approval_required"},
		{
			name:      "allowlist accepts matching domain",
			mode:      modeliam.RegistrationPolicyModeAllowlist,
			input:     RegistrationPolicyEvaluateInput{Email: "ALICE@Example.com"},
			rules:     []registrationPolicyRule{{Type: modeliam.RegistrationPolicyRuleEmailDomainAllowlist, Values: []string{"example.com"}}},
			canSignup: true,
		},
		{
			name:       "allowlist rejects miss",
			mode:       modeliam.RegistrationPolicyModeAllowlist,
			input:      RegistrationPolicyEvaluateInput{Email: "alice@other.test"},
			rules:      []registrationPolicyRule{{Type: modeliam.RegistrationPolicyRuleEmailDomainAllowlist, Values: []string{"example.com"}}},
			reasonCode: "allowlist_miss",
		},
		{
			name:      "progressive rollout accepts 100 percent",
			mode:      modeliam.RegistrationPolicyModeProgressiveRollout,
			input:     RegistrationPolicyEvaluateInput{Email: "alice@example.com"},
			rules:     []registrationPolicyRule{{Type: modeliam.RegistrationPolicyRulePercentage, Value: floatPtr(100), Seed: "contact"}},
			canSignup: true,
		},
		{
			name:       "progressive rollout rejects 0 percent",
			mode:       modeliam.RegistrationPolicyModeProgressiveRollout,
			input:      RegistrationPolicyEvaluateInput{Email: "alice@example.com"},
			rules:      []registrationPolicyRule{{Type: modeliam.RegistrationPolicyRulePercentage, Value: floatPtr(0), Seed: "contact"}},
			reasonCode: "rollout_percentage_miss",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupRegistrationPolicyServiceTestDB(t)
			svc := NewRegistrationPolicyService(db, WithRegistrationPolicyClock(func() time.Time {
				return time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
			}))
			createActiveRegistrationPolicy(t, db, tc.mode, tc.rules)

			got, err := svc.Evaluate(context.Background(), tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.mode, got.Mode)
			require.Equal(t, tc.canSignup, got.CanSignup)
			require.Equal(t, tc.canSubmitRequest, got.CanSubmitRequest)
			require.Equal(t, tc.reasonCode, got.ReasonCode)
		})
	}
}

func TestRegistrationPolicyServiceEvaluateFailsFast(t *testing.T) {
	t.Run("missing active policy", func(t *testing.T) {
		db := setupRegistrationPolicyServiceTestDB(t)
		_, err := NewRegistrationPolicyService(db).Evaluate(context.Background(), RegistrationPolicyEvaluateInput{})
		require.ErrorIs(t, err, ErrRegistrationPolicyActiveMissing)
	})

	t.Run("unknown mode", func(t *testing.T) {
		db := setupRegistrationPolicyServiceTestDB(t)
		createActiveRegistrationPolicy(t, db, "legacy_enabled", nil)
		_, err := NewRegistrationPolicyService(db).Evaluate(context.Background(), RegistrationPolicyEvaluateInput{})
		require.ErrorIs(t, err, ErrRegistrationPolicyInvalid)
	})

	t.Run("unknown rule type", func(t *testing.T) {
		db := setupRegistrationPolicyServiceTestDB(t)
		createActiveRegistrationPolicy(t, db, modeliam.RegistrationPolicyModeOpen, []registrationPolicyRule{{Type: "free_text_rule"}})
		_, err := NewRegistrationPolicyService(db).Evaluate(context.Background(), RegistrationPolicyEvaluateInput{})
		require.ErrorIs(t, err, ErrRegistrationPolicyInvalid)
	})
}

func TestRegistrationPolicyServiceEvaluateUsesStablePercentageBucket(t *testing.T) {
	db := setupRegistrationPolicyServiceTestDB(t)
	svc := NewRegistrationPolicyService(db)
	createActiveRegistrationPolicy(t, db, modeliam.RegistrationPolicyModeProgressiveRollout, []registrationPolicyRule{
		{Type: modeliam.RegistrationPolicyRulePercentage, Value: floatPtr(50), Seed: "contact"},
	})

	first, err := svc.Evaluate(context.Background(), RegistrationPolicyEvaluateInput{Email: "stable@example.com"})
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		got, err := svc.Evaluate(context.Background(), RegistrationPolicyEvaluateInput{Email: "stable@example.com"})
		require.NoError(t, err)
		require.Equal(t, first.CanSignup, got.CanSignup)
		require.Equal(t, first.ReasonCode, got.ReasonCode)
	}
}

func TestRegistrationPolicyServiceEvaluateQuota(t *testing.T) {
	db := setupRegistrationPolicyServiceTestDB(t)
	quota := 1
	createActiveRegistrationPolicyWithQuota(t, db, modeliam.RegistrationPolicyModeOpen, nil, &quota, nil)
	require.NoError(t, db.Create(&modeltenant.Tenant{Key: "used", Name: "Used", Domain: "used.test"}).Error)

	got, err := NewRegistrationPolicyService(db).Evaluate(context.Background(), RegistrationPolicyEvaluateInput{})
	require.NoError(t, err)
	require.False(t, got.CanSignup)
	require.Equal(t, "total_quota_exceeded", got.ReasonCode)
}

func TestRegistrationPolicyServiceListHistory(t *testing.T) {
	db := setupRegistrationPolicyServiceTestDB(t)
	createRegistrationPolicyVersion(t, db, 1, modeliam.RegistrationPolicyModeClosed, modeliam.RegistrationPolicyStatusArchived)
	createRegistrationPolicyVersion(t, db, 2, modeliam.RegistrationPolicyModeInviteOnly, modeliam.RegistrationPolicyStatusActive)
	createRegistrationPolicyVersion(t, db, 3, modeliam.RegistrationPolicyModeOpen, modeliam.RegistrationPolicyStatusDraft)

	items, err := NewRegistrationPolicyService(db).ListHistory(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 3, items[0].Version)
	require.Equal(t, 2, items[1].Version)
	require.Equal(t, modeliam.RegistrationPolicyStatusDraft, items[0].Status)
	require.Equal(t, modeliam.RegistrationPolicyStatusActive, items[1].Status)
}

func setupRegistrationPolicyServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&modeliam.RegistrationPolicy{}, &modeltenant.Tenant{}))
	return db
}

func createActiveRegistrationPolicy(t *testing.T, db *gorm.DB, mode string, rules []registrationPolicyRule) {
	t.Helper()
	createActiveRegistrationPolicyWithQuota(t, db, mode, rules, nil, nil)
}

func createActiveRegistrationPolicyWithQuota(t *testing.T, db *gorm.DB, mode string, rules []registrationPolicyRule, totalQuota *int, dailyQuota *int) {
	t.Helper()
	raw, err := json.Marshal(rules)
	require.NoError(t, err)
	now := time.Now().UTC()
	err = db.Create(&modeliam.RegistrationPolicy{
		PowerUUIDModel:       modelbase.PowerUUIDModel{UUID: uuid.New()},
		Version:              1,
		Mode:                 mode,
		Status:               modeliam.RegistrationPolicyStatusActive,
		RequiresVerification: true,
		RequiresInviteCode:   mode == modeliam.RegistrationPolicyModeInviteOnly,
		TotalTenantQuota:     totalQuota,
		DailyTenantQuota:     dailyQuota,
		Rules:                datatypes.JSON(raw),
		ActivatedAt:          &now,
		CreatedByUserUUID:    uuid.NewString(),
		UpdatedByUserUUID:    uuid.NewString(),
	}).Error
	require.NoError(t, err)
}

func createRegistrationPolicyVersion(t *testing.T, db *gorm.DB, version int, mode string, status string) {
	t.Helper()
	now := time.Now().UTC()
	policy := &modeliam.RegistrationPolicy{
		PowerUUIDModel:       modelbase.PowerUUIDModel{UUID: uuid.New()},
		Version:              version,
		Mode:                 mode,
		Status:               status,
		RequiresVerification: true,
		RequiresInviteCode:   mode == modeliam.RegistrationPolicyModeInviteOnly,
		Rules:                datatypes.JSON([]byte(`[]`)),
		CreatedByUserUUID:    uuid.NewString(),
		UpdatedByUserUUID:    uuid.NewString(),
	}
	if status == modeliam.RegistrationPolicyStatusActive {
		policy.ActivatedAt = &now
	}
	require.NoError(t, db.Create(policy).Error)
}

func floatPtr(v float64) *float64 {
	return &v
}

func TestRegistrationPolicyServiceErrorsAreComparable(t *testing.T) {
	require.True(t, errors.Is(fmt.Errorf("%w: bad", ErrRegistrationPolicyInvalid), ErrRegistrationPolicyInvalid))
}
