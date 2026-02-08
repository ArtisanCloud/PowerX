package bus

import (
	"strings"
	"sync"
)

var publishAllowedTopics = map[string]struct{}{
	TopicOrgSyncProgress:   {},
	TopicOrgSyncProgressV1: {},
}

var publishDynamicTopics = struct {
	mu       sync.RWMutex
	byTenant map[string]map[string]struct{}
}{
	byTenant: make(map[string]map[string]struct{}),
}

func RegisterPublishTopics(tenantUUID string, topics []string) []string {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil
	}

	unique := make(map[string]struct{})
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		unique[topic] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}

	publishDynamicTopics.mu.Lock()
	if _, ok := publishDynamicTopics.byTenant[tenantUUID]; !ok {
		publishDynamicTopics.byTenant[tenantUUID] = make(map[string]struct{})
	}
	for topic := range unique {
		publishDynamicTopics.byTenant[tenantUUID][topic] = struct{}{}
	}
	publishDynamicTopics.mu.Unlock()

	registered := make([]string, 0, len(unique))
	for topic := range unique {
		registered = append(registered, topic)
	}
	return registered
}

func IsDynamicTopicRegistered(tenantUUID, topic string) bool {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return false
	}
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return false
	}
	publishDynamicTopics.mu.RLock()
	topics := publishDynamicTopics.byTenant[tenantUUID]
	_, ok := topics[topic]
	publishDynamicTopics.mu.RUnlock()
	return ok
}

func IsPublishTopicAllowed(tenantUUID, topic string) bool {
	allowed, _, _ := PublishTopicCheck(tenantUUID, topic)
	return allowed
}

// PublishTopicCheck returns whether a topic is allowed and which rule matched.
func PublishTopicCheck(tenantUUID, topic string) (allowed, whitelistHit, dynamicHit bool) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return false, false, false
	}
	_, whitelistHit = publishAllowedTopics[topic]
	dynamicHit = IsDynamicTopicRegistered(tenantUUID, topic)
	allowed = whitelistHit || dynamicHit
	return allowed, whitelistHit, dynamicHit
}
