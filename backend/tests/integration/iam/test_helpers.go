package iamintegration

import (
	"fmt"
	"testing"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
	))

	return &iamFixture{
		DB: db,
		Me: authsvc.NewMeService(db),
	}
}

func mustUUID(v string) uuid.UUID {
	return uuid.MustParse(v)
}
