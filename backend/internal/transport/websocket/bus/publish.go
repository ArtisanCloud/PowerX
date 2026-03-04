package bus

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var publishAllowedTopics = map[string]struct{}{}

const envWSDynamicTopicCompat = "POWERX_WS_DYNAMIC_TOPIC_COMPAT"

var dynamicTopicCompat atomic.Bool

func init() {
	dynamicTopicCompat.Store(parseBoolEnv(envWSDynamicTopicCompat))
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

	if !isDynamicTopicCompatEnabled() {
		registered := make([]string, 0, len(unique))
		for topic := range unique {
			registered = append(registered, topic)
		}
		return registered
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
	if !isDynamicTopicCompatEnabled() {
		return false
	}
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

func isDynamicTopicCompatEnabled() bool {
	if parseBoolEnv(envWSDynamicTopicCompat) {
		dynamicTopicCompat.Store(true)
		return true
	}
	return dynamicTopicCompat.Load()
}

func parseBoolEnv(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// SetDynamicTopicCompatEnabledForTest 仅用于测试控制兼容开关。
func SetDynamicTopicCompatEnabledForTest(enabled bool) {
	dynamicTopicCompat.Store(enabled)
}
