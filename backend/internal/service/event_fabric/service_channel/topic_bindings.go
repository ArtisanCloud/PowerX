package servicechannel

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"gorm.io/gorm"
)

// ResolveTopics 只读返回当前真实服务身份、租户和方向下的正式映射；不创建主题或授权。
func (s *Service) ResolveTopics(ctx context.Context, in Request, capability string) (result map[string]any, err error) {
	start := time.Now()
	defer func() { s.record(ctx, in, result, err, start) }()
	if capability != PublishCapability && capability != ConsumeCapability {
		return nil, failure(403, "EVENT_FABRIC_OPERATION_FORBIDDEN")
	}
	if _, err := s.Check(ctx, in, capability); err != nil {
		return nil, err
	}
	if in.Operation != "resolve_topics" || len(in.TopicKeys) == 0 || len(in.TopicKeys) > 1000 || in.Event != nil || len(in.Topics) > 0 || in.DeliveryID != "" || in.Reason != "" || in.BatchSize != 0 || len(in.SupportedVersions) > 0 || in.CompatibilityMode != "" {
		return nil, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	var credential setting.PluginInstanceConfig
	if err := s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", in.TenantUUID, in.PluginID, settingrepo.KeyClientCredentials, true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, failure(409, "EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED")
		}
		return nil, failure(503, "EVENT_FABRIC_UNAVAILABLE")
	}
	var value struct {
		Config map[string]any `json:"config"`
	}
	if json.Unmarshal(credential.ValueJSON, &value) != nil {
		return nil, failure(503, "EVENT_FABRIC_BINDINGS_INVALID")
	}
	bridge := pluginbinding.Bridge(value.Config)
	if bridge["tenant_uuid"] != in.TenantUUID {
		return nil, failure(409, "EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED")
	}
	bindings, _ := bridge["topic_bindings"].(map[string]any)
	selected := map[string]string{}
	action := "publish"
	if capability == ConsumeCapability {
		action = "subscribe"
	}
	for _, key := range in.TopicKeys {
		if !valid(key, 255) || selected[key] != "" {
			return nil, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
		}
		full, ok := bindings[key].(string)
		if !ok || !valid(full, 255) {
			return nil, failure(409, "EVENT_FABRIC_TOPIC_BINDING_REQUIRED")
		}
		topic, err := s.topic(ctx, in.TenantUUID, full)
		if err != nil {
			return nil, err
		}
		if err := s.can(ctx, in.TenantUUID, topic.UUID, in.PluginID, action); err != nil {
			return nil, err
		}
		selected[key] = topic.FullTopic
	}
	return map[string]any{"contract_version": Contract, "tenant_uuid": in.TenantUUID, "plugin_id": in.PluginID, "topic_bindings": selected}, nil
}
