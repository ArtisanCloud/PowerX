package acl

import (
	"context"
	"testing"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

func TestBuildACLResultCacheKey(t *testing.T) {
	topicID := uuid.MustParse("8f4aa96b-5fb7-4e7a-bdd8-f87fdf8e26d1")
	key := BuildACLResultCacheKey(" TENANT-COREX ", topicID, " Service.Replay ", " Subscribe ")
	expected := "event:acl:tenant-corex:8f4aa96b-5fb7-4e7a-bdd8-f87fdf8e26d1:service.replay:subscribe"
	if key != expected {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestLayeredACLResultCache_LocalThenRemote(t *testing.T) {
	ctx := context.Background()
	local := NewLocalACLResultCache(10*time.Minute, time.Now)
	remote := NewLocalACLResultCache(10*time.Minute, time.Now)
	cache := NewLayeredACLResultCache(local, remote)

	key := "event:acl:tenant:topic:principal:publish"
	if err := remote.Set(ctx, key, true); err != nil {
		t.Fatalf("seed remote cache failed: %v", err)
	}

	allowed, hit, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("get from layered cache failed: %v", err)
	}
	if !hit || !allowed {
		t.Fatalf("expected remote hit with allowed=true, got hit=%v allowed=%v", hit, allowed)
	}

	if err := remote.Delete(ctx, key); err != nil {
		t.Fatalf("delete remote failed: %v", err)
	}

	allowed, hit, err = cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("get from local cache failed: %v", err)
	}
	if !hit || !allowed {
		t.Fatalf("expected local hit after warmup, got hit=%v allowed=%v", hit, allowed)
	}
}

func TestACLServiceCan_UseCache(t *testing.T) {
	store := newCountAclStore(true)
	svc := NewACLService(Options{
		Store: store,
		Cache: NewLayeredACLResultCache(
			NewLocalACLResultCache(10*time.Minute, time.Now),
			nil,
		),
		Clock: time.Now,
	})

	ctx := context.Background()
	tenant := "tenant-corex"
	topic := uuid.MustParse("52937ef6-d650-42d2-9f41-4b6cf5f787b3")
	principal := "core.worker"

	allowed, err := svc.Can(ctx, tenant, topic, principal, PrincipalActionPublish)
	if err != nil {
		t.Fatalf("first can failed: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allowed=true")
	}
	if store.calls != 1 {
		t.Fatalf("expected first call hits store once, got %d", store.calls)
	}

	allowed, err = svc.Can(ctx, tenant, topic, principal, PrincipalActionPublish)
	if err != nil {
		t.Fatalf("second can failed: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allowed=true on cached path")
	}
	if store.calls != 1 {
		t.Fatalf("expected second call served from cache, store calls=%d", store.calls)
	}
}

type countAclStore struct {
	allowed bool
	calls   int
}

func newCountAclStore(allowed bool) *countAclStore {
	return &countAclStore{allowed: allowed}
}

func (s *countAclStore) UpsertBindings(_ context.Context, _ []*model.AclBinding) ([]*model.AclBinding, error) {
	panic("unexpected call")
}

func (s *countAclStore) RemoveBindings(_ context.Context, _ string, _ uuid.UUID, _ string, _ []string) (int64, error) {
	panic("unexpected call")
}

func (s *countAclStore) ListByTopic(_ context.Context, _ string, _ uuid.UUID) ([]*model.AclBinding, error) {
	panic("unexpected call")
}

func (s *countAclStore) HasPermission(_ context.Context, _ string, _ uuid.UUID, _ string, _ string, _ time.Time) (bool, error) {
	s.calls++
	return s.allowed, nil
}
