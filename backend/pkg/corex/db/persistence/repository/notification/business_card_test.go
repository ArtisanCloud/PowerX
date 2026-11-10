package notification

import (
	"context"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPostgresConcurrentCardUpdatesAndLegacyColumns(t *testing.T) {
	if os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN") == "" {
		t.Skip("isolated Postgres DSN required")
	}
	db := host.Database(t)
	require.NoError(t, db.AutoMigrate(&nm.Notification{}))
	tenant, member := uuid.NewString(), uuid.NewString()
	r := NewNotificationRepository(db)
	row, err := r.Create(context.Background(), &nm.Notification{TenantUUID: tenant, MemberUUID: member, ServiceOwner: "plugin:test", SourceRevision: 1, Revision: 1, Title: "fixture", Type: "info", Category: "order"})
	require.NoError(t, err)
	var wg sync.WaitGroup
	changes := make(chan bool, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, changed, err := r.InvalidateCard(context.Background(), tenant, "plugin:test", member, row.UUID.String(), 2, time.Now(), true, nil)
			changes <- changed
			errs <- err
		}()
	}
	wg.Wait()
	close(changes)
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	count := 0
	for changed := range changes {
		if changed {
			count++
		}
	}
	require.Equal(t, 1, count)
	current, err := r.FindByUUID(context.Background(), tenant, member, row.UUID.String())
	require.NoError(t, err)
	require.EqualValues(t, 2, current.Revision)
	require.EqualValues(t, 2, current.SourceRevision)
	require.ErrorIs(t, r.MarkReadVersion(context.Background(), tenant, member, row.UUID.String(), 1, time.Now()), ErrRevisionConflict)
	require.NoError(t, r.MarkReadVersion(context.Background(), tenant, member, row.UUID.String(), 2, time.Now()))
	// Upgrade old ordinary notification rows with additive default columns.
	require.NoError(t, db.Migrator().DropColumn(&nm.Notification{}, "revision"))
	require.NoError(t, db.Migrator().DropColumn(&nm.Notification{}, "source_revision"))
	require.NoError(t, db.AutoMigrate(&nm.Notification{}))
	var backfilled nm.Notification
	require.NoError(t, db.Where("uuid = ?", row.UUID).First(&backfilled).Error)
	require.EqualValues(t, 1, backfilled.Revision)
	require.Zero(t, backfilled.SourceRevision)
	require.True(t, backfilled.IsRead)
	require.Equal(t, row.UUID, backfilled.UUID)
}
