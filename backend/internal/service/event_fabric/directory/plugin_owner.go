package directory

import (
	"context"
	"encoding/json"
	"fmt"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
)

const PluginOwnerMetadataKey = "core.plugin_owner"
const PluginTopicKeysMetadataKey = "core.plugin_topic_keys"

// PluginOwner 返回 Core 写入的主题归属；旧主题没有此标记时不能自动认领。
func PluginOwner(topic *model.TopicDefinition) string {
	if topic == nil {
		return ""
	}
	var metadata map[string]any
	if json.Unmarshal(topic.Metadata, &metadata) != nil {
		return ""
	}
	owner, _ := metadata[PluginOwnerMetadataKey].(string)
	return owner
}

func GeneratedPluginBinding(topic *model.TopicDefinition, plugin, key string) bool {
	if PluginOwner(topic) != plugin {
		return false
	}
	var metadata map[string]any
	if json.Unmarshal(topic.Metadata, &metadata) != nil {
		return false
	}
	keys, _ := metadata[PluginTopicKeysMetadataKey].([]any)
	for _, value := range keys {
		if value == key {
			return true
		}
	}
	return false
}

// MarkPluginTopic 仅供已经核验权威绑定的 Core 安装服务使用，保留生成映射的历史 key，便于失败升级后重试。
func (s *DirectoryService) MarkPluginTopic(ctx context.Context, topic *model.TopicDefinition, plugin, key string) error {
	if topic == nil || plugin == "" || key == "" {
		return fmt.Errorf("topic owner required")
	}
	if owner := PluginOwner(topic); owner != "" && owner != plugin {
		return fmt.Errorf("topic owner conflict")
	}
	if GeneratedPluginBinding(topic, plugin, key) {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(topic.Metadata, &metadata); err != nil && len(topic.Metadata) > 0 {
		return err
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	keys, _ := metadata[PluginTopicKeysMetadataKey].([]any)
	metadata[PluginOwnerMetadataKey] = plugin
	metadata[PluginTopicKeysMetadataKey] = append(keys, key)
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	topic.Metadata = raw
	_, err = s.store.Update(ctx, topic)
	return err
}
