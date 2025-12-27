package manifest

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBindingStoreTopicPersistence(t *testing.T) {
	store := newTestBindingStore(t)
	ctx := context.Background()

	record := TopicBindingRecord{
		TenantUUID: "AEFFC79F-E72A-4FD9-B908-5C150BCE3741",
		PluginID:   "plugin.demo",
		TopicKey:   "orders.created",
		Namespace:  "orders",
		Name:       "created",
		FullTopic:  "aeffc79f-e72a-4fd9-b908-5c150bce3741.orders.created",
		TopicID:    "topic-1",
	}
	require.NoError(t, store.UpsertTopic(ctx, record))

	fetched, err := store.GetTopic(ctx, "aeffc79f-e72a-4fd9-b908-5c150bce3741", "plugin.demo", "orders.created")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, "aeffc79f-e72a-4fd9-b908-5c150bce3741", fetched.TenantUUID)
	require.Equal(t, "orders", fetched.Namespace)
	require.Equal(t, "topic-1", fetched.TopicID)

	record.FullTopic = "aeffc79f-e72a-4fd9-b908-5c150bce3741.orders.created.v2"
	record.TopicID = "topic-2"
	require.NoError(t, store.UpsertTopic(ctx, record))

	fetched, err = store.GetTopic(ctx, "AEFFC79F-E72A-4FD9-B908-5C150BCE3741", "plugin.demo", "orders.created")
	require.NoError(t, err)
	require.Equal(t, "topic-2", fetched.TopicID)
	require.Equal(t, record.FullTopic, fetched.FullTopic)
}

func TestBindingStoreACLPersistence(t *testing.T) {
	store := newTestBindingStore(t)
	ctx := context.Background()

	actions := []acl.PrincipalAction{acl.PrincipalActionPublish, acl.PrincipalActionSubscribe}
	record := ACLBindingRecord{
		TenantUUID:    "AEFFC79F-E72A-4FD9-B908-5C150BCE3741",
		PluginID:      "plugin.demo",
		TopicKey:      "orders.created",
		PrincipalType: "Service",
		PrincipalID:   " demo-writer ",
		ActionsHash:   "publish,subscribe",
		Actions:       actions,
	}
	require.NoError(t, store.UpsertACL(ctx, record))

	fetched, err := store.GetACL(ctx, "aeffc79f-e72a-4fd9-b908-5c150bce3741", "plugin.demo", "orders.created", "SERVICE", "demo-writer")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	require.Equal(t, "service", fetched.PrincipalType)
	require.Equal(t, "demo-writer", fetched.PrincipalID)
	require.Equal(t, "publish,subscribe", fetched.ActionsHash)
	require.Equal(t, actions, fetched.Actions)

	record.Actions = []acl.PrincipalAction{acl.PrincipalActionReplay}
	record.ActionsHash = "replay"
	require.NoError(t, store.UpsertACL(ctx, record))

	fetched, err = store.GetACL(ctx, "aeffc79f-e72a-4fd9-b908-5c150bce3741", "plugin.demo", "orders.created", "service", "demo-writer")
	require.NoError(t, err)
	require.Equal(t, []acl.PrincipalAction{acl.PrincipalActionReplay}, fetched.Actions)
	require.Equal(t, "replay", fetched.ActionsHash)
}

func newTestBindingStore(t *testing.T) BindingStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	origSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = origSchema
	})
	require.NoError(t, db.AutoMigrate(&eventfabricmodel.TopicManifestBinding{}, &eventfabricmodel.AclManifestBinding{}))
	repo := eventfabricrepo.NewManifestBindingRepository(db)
	return NewBindingStore(repo)
}
