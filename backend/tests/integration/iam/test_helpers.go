package iamintegration

import (
	"fmt"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeligw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	modelsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type iamFixture struct {
	DB *gorm.DB
	Me *authsvc.MeService
}

func setupIAMFixture(t *testing.T) *iamFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() {
		modelbase.PowerXSchema = prevSchema
	})

	require.NoError(t, db.AutoMigrate(
		&modeltenant.Tenant{},
		&modeliam.User{},
		&modeliam.Member{},
		&modeliam.Role{},
		&modeliam.RoleBinding{},
		&modeliam.Credential{},
		&modeliam.RefreshToken{},
		&modeliam.Permission{},
		&modeliam.APIKeyProfile{},
		&modeliam.APIKeyProfilePermission{},
		&modeliam.RootSupportSession{},
		&modeliam.RegistrationPolicy{},
		&modeliam.RegistrationInviteBatch{},
		&modeliam.RegistrationInviteCode{},
		&modeliam.RegistrationRequest{},
		&modeliam.RegistrationPolicyAuditEvent{},
		&modeligw.IntegrationGatewayAPIKey{},
		&modeligw.IntegrationGatewayAPIKeyPermission{},
		&modelaudit.AuditEvent{},
		&modelsetting.PluginInstanceConfig{},
	))
	seedActiveRegistrationPolicy(t, db, false)
	require.NoError(t, db.Exec(`CREATE TABLE iam_tenant_key_pairs (
		id integer primary key autoincrement,
		created_at datetime,
		updated_at datetime,
		deleted_at datetime,
		env text,
		tenant_uuid text,
		k_id text,
		alg text,
		public_pem text,
		enc_private JSON,
		active numeric
	)`).Error)
	require.NoError(t, crypto.SetGlobalKeyB64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="))

	return &iamFixture{
		DB: db,
		Me: authsvc.NewMeService(db),
	}
}

func seedActiveRegistrationPolicy(t *testing.T, db *gorm.DB, requiresVerification bool) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, db.Create(&modeliam.RegistrationPolicy{
		PowerUUIDModel:       modelbase.PowerUUIDModel{UUID: uuid.New()},
		Version:              1,
		Mode:                 modeliam.RegistrationPolicyModeOpen,
		Status:               modeliam.RegistrationPolicyStatusActive,
		RequiresVerification: requiresVerification,
		Rules:                datatypes.JSON([]byte(`[]`)),
		ActivatedAt:          &now,
		CreatedByUserUUID:    uuid.NewString(),
		UpdatedByUserUUID:    uuid.NewString(),
	}).Error)
}

func setActiveRegistrationPolicyVerification(t *testing.T, db *gorm.DB, requiresVerification bool) {
	t.Helper()
	require.NoError(t, db.Model(&modeliam.RegistrationPolicy{}).
		Where("status = ?", modeliam.RegistrationPolicyStatusActive).
		Update("requires_verification", requiresVerification).Error)
}

func setActiveRegistrationPolicyInviteOnly(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Model(&modeliam.RegistrationPolicy{}).
		Where("status = ?", modeliam.RegistrationPolicyStatusActive).
		Updates(map[string]any{
			"mode":                 modeliam.RegistrationPolicyModeInviteOnly,
			"requires_invite_code": true,
		}).Error)
}

func mustUUID(v string) uuid.UUID {
	return uuid.MustParse(v)
}
