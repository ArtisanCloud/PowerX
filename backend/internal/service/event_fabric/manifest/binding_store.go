package manifest

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"gorm.io/datatypes"
)

// BindingStore 提供 Topic/ACL 播种记录的读写能力。
type BindingStore interface {
	GetTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) (*TopicBindingRecord, error)
	ListTopics(ctx context.Context, tenantUUID, pluginID string) ([]TopicBindingRecord, error)
	UpsertTopic(ctx context.Context, record TopicBindingRecord) error
	DeleteTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) error
	GetACL(ctx context.Context, tenantUUID, pluginID, topicKey, principalType, principalID string) (*ACLBindingRecord, error)
	UpsertACL(ctx context.Context, record ACLBindingRecord) error
	DeleteACLByTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) error
}

type bindingStore struct {
	repo *eventfabricrepo.ManifestBindingRepository
}

type TopicBindingRecord struct {
	TenantUUID string
	PluginID   string
	TopicKey   string
	Namespace  string
	Name       string
	FullTopic  string
	TopicID    string
}

type ACLBindingRecord struct {
	TenantUUID    string
	PluginID      string
	TopicKey      string
	PrincipalType string
	PrincipalID   string
	ActionsHash   string
	Actions       []acl.PrincipalAction
}

func NewBindingStore(repo *eventfabricrepo.ManifestBindingRepository) BindingStore {
	if repo == nil {
		return nil
	}
	return &bindingStore{repo: repo}
}

func (s *bindingStore) GetTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) (*TopicBindingRecord, error) {
	model, err := s.repo.GetTopicBinding(ctx, tenantUUID, pluginID, topicKey)
	if err != nil || model == nil {
		return nil, err
	}
	return &TopicBindingRecord{
		TenantUUID: model.TenantKey,
		PluginID:   model.PluginID,
		TopicKey:   model.TopicKey,
		Namespace:  model.Namespace,
		Name:       model.Name,
		FullTopic:  model.FullTopic,
		TopicID:    model.TopicUUID,
	}, nil
}

func (s *bindingStore) UpsertTopic(ctx context.Context, record TopicBindingRecord) error {
	return s.repo.UpsertTopicBinding(ctx, &eventfabricmodel.TopicManifestBinding{
		TenantKey:     record.TenantUUID,
		PluginID:      record.PluginID,
		TopicKey:      record.TopicKey,
		Namespace:     record.Namespace,
		Name:          record.Name,
		FullTopic:     record.FullTopic,
		TopicUUID:     record.TopicID,
		LastAppliedAt: time.Now().UTC(),
	})
}

func (s *bindingStore) ListTopics(ctx context.Context, tenantUUID, pluginID string) ([]TopicBindingRecord, error) {
	models, err := s.repo.ListTopicBindings(ctx, tenantUUID, pluginID)
	if err != nil {
		return nil, err
	}
	records := make([]TopicBindingRecord, 0, len(models))
	for _, model := range models {
		if model == nil {
			continue
		}
		records = append(records, TopicBindingRecord{
			TenantUUID: model.TenantKey,
			PluginID:   model.PluginID,
			TopicKey:   model.TopicKey,
			Namespace:  model.Namespace,
			Name:       model.Name,
			FullTopic:  model.FullTopic,
			TopicID:    model.TopicUUID,
		})
	}
	return records, nil
}

func (s *bindingStore) DeleteTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) error {
	return s.repo.DeleteTopicBinding(ctx, tenantUUID, pluginID, topicKey)
}

func (s *bindingStore) GetACL(ctx context.Context, tenantUUID, pluginID, topicKey, principalType, principalID string) (*ACLBindingRecord, error) {
	model, err := s.repo.GetAclBinding(ctx, tenantUUID, pluginID, topicKey, principalType, principalID)
	if err != nil || model == nil {
		return nil, err
	}
	var actions []acl.PrincipalAction
	if len(model.Actions) > 0 {
		var values []string
		if err := json.Unmarshal(model.Actions, &values); err == nil {
			for _, v := range values {
				actions = append(actions, acl.PrincipalAction(v))
			}
		}
	}
	return &ACLBindingRecord{
		TenantUUID:    model.TenantKey,
		PluginID:      model.PluginID,
		TopicKey:      model.TopicKey,
		PrincipalType: model.PrincipalType,
		PrincipalID:   model.PrincipalID,
		ActionsHash:   model.ActionsHash,
		Actions:       actions,
	}, nil
}

func (s *bindingStore) UpsertACL(ctx context.Context, record ACLBindingRecord) error {
	actionsJSON, err := aclActionsToJSON(record.Actions)
	if err != nil {
		return err
	}
	return s.repo.UpsertAclBinding(ctx, &eventfabricmodel.AclManifestBinding{
		TenantKey:     record.TenantUUID,
		PluginID:      record.PluginID,
		TopicKey:      record.TopicKey,
		PrincipalType: record.PrincipalType,
		PrincipalID:   record.PrincipalID,
		Actions:       actionsJSON,
		ActionsHash:   record.ActionsHash,
		LastAppliedAt: time.Now().UTC(),
	})
}

func (s *bindingStore) DeleteACLByTopic(ctx context.Context, tenantUUID, pluginID, topicKey string) error {
	return s.repo.DeleteAclBindingsByTopic(ctx, tenantUUID, pluginID, topicKey)
}

func aclActionsToJSON(actions []acl.PrincipalAction) (datatypes.JSON, error) {
	values := make([]string, len(actions))
	for i, act := range actions {
		values[i] = string(act)
	}
	if len(values) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	bytes, err := json.Marshal(values)
	if err != nil {
		return datatypes.JSON([]byte("[]")), err
	}
	return datatypes.JSON(bytes), nil
}
