package bus

import "strings"

var publishAllowedTopics = map[string]struct{}{
	TopicOrgSyncProgress: {},
}

func IsPublishTopicAllowed(topic string) bool {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return false
	}
	_, ok := publishAllowedTopics[topic]
	return ok
}
