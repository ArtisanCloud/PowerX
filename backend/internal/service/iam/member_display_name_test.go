package iam

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureActiveDisplayNameAvailableRejectsNormalizedActiveDuplicate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:member-display-name?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	table := model.TableIAMMember
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+table+" (id integer primary key, tenant_uuid text not null, display_name text, status integer not null, deleted_at datetime)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+table+" (id, tenant_uuid, display_name, status) VALUES (1, 'tenant-a', 'Tom', 1)").Error)

	svc := NewMemberService(db)
	err = svc.ensureActiveDisplayNameAvailable(context.Background(), db, "tenant-a", " tom ", 0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMemberDisplayNameConflict))
	require.Equal(t, CodeMemberDisplayNameConflict, "IAM_MEMBER_DISPLAY_NAME_CONFLICT")

	require.NoError(t, svc.ensureActiveDisplayNameAvailable(context.Background(), db, "tenant-a", "tom", 1))
	require.NoError(t, svc.ensureActiveDisplayNameAvailable(context.Background(), db, "tom", "tom", 0))
}

func TestEnsureActiveDisplayNameAvailableIgnoresInactiveAndDeletedMembers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:member-display-name-inactive?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	table := model.TableIAMMember
	require.NoError(t, db.Exec("DROP TABLE IF EXISTS "+table).Error)
	require.NoError(t, db.Exec("CREATE TABLE "+table+" (id integer primary key, tenant_uuid text not null, display_name text, status integer not null, deleted_at datetime)").Error)
	require.NoError(t, db.Exec("INSERT INTO "+table+" (id, tenant_uuid, display_name, status) VALUES (1, 'tenant-a', 'Tom', 0)").Error)

	svc := NewMemberService(db)
	require.NoError(t, svc.ensureActiveDisplayNameAvailable(context.Background(), db, "tenant-a", "Tom", 0))
}
