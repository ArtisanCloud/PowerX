package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/cache"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

const topicResolveCachePrefix = "event:topic:resolve"

type TopicLookup interface {
	FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
	FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error)
}

type TopicTemplateLookup interface {
	FindTemplateMatch(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
}

type CachedTopicLookupOptions struct {
	Cache   cache.ICache
	TTL     time.Duration
	MissTTL time.Duration
}

type cachedTopicLookupPayload struct {
	NotFound bool                              `json:"not_found"`
	Topic    *eventfabricmodel.TopicDefinition `json:"topic,omitempty"`
}

type CachedTopicLookup struct {
	base    TopicLookup
	cache   cache.ICache
	ttl     time.Duration
	missTTL time.Duration
}

func NewCachedTopicLookup(base TopicLookup, opts CachedTopicLookupOptions) *CachedTopicLookup {
	if base == nil {
		return nil
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 180 * time.Second
	}
	missTTL := opts.MissTTL
	if missTTL <= 0 {
		missTTL = 30 * time.Second
	}
	return &CachedTopicLookup{
		base:    base,
		cache:   opts.Cache,
		ttl:     ttl,
		missTTL: missTTL,
	}
}

func (c *CachedTopicLookup) FindByComposite(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	if c == nil || c.base == nil {
		return nil, nil
	}
	tenant := strings.TrimSpace(strings.ToLower(tenantKey))
	ns := strings.TrimSpace(strings.ToLower(namespace))
	topicName := strings.TrimSpace(strings.ToLower(name))
	if ns == "" || topicName == "" {
		return c.base.FindByComposite(ctx, tenantKey, namespace, name)
	}
	cacheKey := fmt.Sprintf("%s:%s:%s.%s", topicResolveCachePrefix, tenant, ns, topicName)
	if topic, hit, err := c.getCachedComposite(ctx, cacheKey); err != nil {
		return nil, err
	} else if hit {
		return topic, nil
	}

	topic, err := c.base.FindByComposite(ctx, tenantKey, namespace, name)
	if err != nil {
		return nil, err
	}
	if topic == nil {
		_ = c.setCachedComposite(ctx, cacheKey, &cachedTopicLookupPayload{NotFound: true}, c.missTTL)
		return nil, nil
	}
	_ = c.setCachedComposite(ctx, cacheKey, &cachedTopicLookupPayload{NotFound: false, Topic: topic}, c.ttl)
	return topic, nil
}

func (c *CachedTopicLookup) FindByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.TopicDefinition, error) {
	if c == nil || c.base == nil {
		return nil, nil
	}
	return c.base.FindByUUID(ctx, id)
}

func (c *CachedTopicLookup) FindTemplateMatch(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	if c == nil || c.base == nil {
		return nil, nil
	}
	templateLookup, ok := c.base.(TopicTemplateLookup)
	if !ok {
		return nil, nil
	}
	return templateLookup.FindTemplateMatch(ctx, tenantKey, namespace, name)
}

func (c *CachedTopicLookup) getCachedComposite(ctx context.Context, key string) (*eventfabricmodel.TopicDefinition, bool, error) {
	if c == nil || c.cache == nil {
		return nil, false, nil
	}
	raw, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, false, nil
	}
	if len(raw) == 0 {
		return nil, false, nil
	}
	var payload cachedTopicLookupPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false, nil
	}
	if payload.NotFound {
		return nil, true, nil
	}
	return payload.Topic, true, nil
}

func (c *CachedTopicLookup) setCachedComposite(ctx context.Context, key string, payload *cachedTopicLookupPayload, ttl time.Duration) error {
	if c == nil || c.cache == nil || payload == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.cache.Set(ctx, key, raw, ttl)
}
