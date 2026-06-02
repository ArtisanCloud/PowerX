package shared

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

func TestCachedTopicLookup_FindByComposite_CacheHit(t *testing.T) {
	ctx := context.Background()
	base := &mockTopicLookup{
		result: &eventfabricmodel.TopicDefinition{
			PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
			TenantKey:      "tenant-a",
			Namespace:      "knowledge.space",
			Name:           "reprocess",
		},
	}
	lookup := NewCachedTopicLookup(base, CachedTopicLookupOptions{Cache: cache.NewMemoryCache()})

	first, err := lookup.FindByComposite(ctx, "tenant-a", "knowledge.space", "reprocess")
	if err != nil || first == nil {
		t.Fatalf("first lookup failed: err=%v topic_nil=%v", err, first == nil)
	}
	second, err := lookup.FindByComposite(ctx, "tenant-a", "knowledge.space", "reprocess")
	if err != nil || second == nil {
		t.Fatalf("second lookup failed: err=%v topic_nil=%v", err, second == nil)
	}
	if base.calls != 1 {
		t.Fatalf("expected base lookup called once, got %d", base.calls)
	}
}

func TestCachedTopicLookup_FindByComposite_MissCached(t *testing.T) {
	ctx := context.Background()
	base := &mockTopicLookup{result: nil}
	lookup := NewCachedTopicLookup(base, CachedTopicLookupOptions{Cache: cache.NewMemoryCache()})

	first, err := lookup.FindByComposite(ctx, "tenant-a", "knowledge.space", "missing")
	if err != nil {
		t.Fatalf("first lookup error: %v", err)
	}
	if first != nil {
		t.Fatalf("expected nil on first miss")
	}
	second, err := lookup.FindByComposite(ctx, "tenant-a", "knowledge.space", "missing")
	if err != nil {
		t.Fatalf("second lookup error: %v", err)
	}
	if second != nil {
		t.Fatalf("expected nil on second miss")
	}
	if base.calls != 1 {
		t.Fatalf("expected base lookup called once for miss cache, got %d", base.calls)
	}
}

func TestCachedTopicLookup_FindTemplateMatchDelegatesToBase(t *testing.T) {
	ctx := context.Background()
	base := &mockTopicLookup{
		templateResult: &eventfabricmodel.TopicDefinition{
			PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
			TenantKey:      "tenant-a",
			Namespace:      "ai_craft.progress.tenant_{{tenant_uuid}}",
			Name:           "member_{{member_uuid}}",
		},
	}
	lookup := NewCachedTopicLookup(base, CachedTopicLookupOptions{Cache: cache.NewMemoryCache()})

	topic, err := lookup.FindTemplateMatch(ctx, "tenant-a", "ai_craft.progress.tenant_tenant-a", "member_abc")
	if err != nil {
		t.Fatalf("template lookup error: %v", err)
	}
	if topic == nil {
		t.Fatalf("expected template topic")
	}
	if base.templateCalls != 1 {
		t.Fatalf("expected base template lookup called once, got %d", base.templateCalls)
	}
}

func TestCachedTopicLookup_FindTemplateMatchReturnsNilWhenBaseUnsupported(t *testing.T) {
	ctx := context.Background()
	lookup := NewCachedTopicLookup(&plainTopicLookup{}, CachedTopicLookupOptions{Cache: cache.NewMemoryCache()})

	topic, err := lookup.FindTemplateMatch(ctx, "tenant-a", "ai_craft.progress.tenant_tenant-a", "member_abc")
	if err != nil {
		t.Fatalf("template lookup error: %v", err)
	}
	if topic != nil {
		t.Fatalf("expected nil template topic")
	}
}

type mockTopicLookup struct {
	result         *eventfabricmodel.TopicDefinition
	templateResult *eventfabricmodel.TopicDefinition
	calls          int
	templateCalls  int
}

func (m *mockTopicLookup) FindByComposite(_ context.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	m.calls++
	if m.result == nil {
		return nil, nil
	}
	clone := *m.result
	return &clone, nil
}

func (m *mockTopicLookup) FindByUUID(_ context.Context, _ uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	if m.result == nil {
		return nil, nil
	}
	clone := *m.result
	return &clone, nil
}

func (m *mockTopicLookup) FindTemplateMatch(_ context.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	m.templateCalls++
	if m.templateResult == nil {
		return nil, nil
	}
	clone := *m.templateResult
	return &clone, nil
}

type plainTopicLookup struct{}

func (p *plainTopicLookup) FindByComposite(context.Context, string, string, string) (*eventfabricmodel.TopicDefinition, error) {
	return nil, nil
}

func (p *plainTopicLookup) FindByUUID(context.Context, uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	return nil, nil
}
