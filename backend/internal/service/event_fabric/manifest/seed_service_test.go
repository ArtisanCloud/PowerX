package manifest

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestApplyPlanRetiresStaleManifestTopics(t *testing.T) {
	ctx := context.Background()
	tenantUUID := "tenant-seed"
	pluginID := "com.powerx.plugins.ai-craft"
	activeID := uuid.New()
	staleID := uuid.New()
	dir := &seedDirectoryStub{
		existing: map[string]*eventfabricmodel.TopicDefinition{
			"tenant-seed|ai_craft.current|progress": {
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: activeID},
				TenantKey:      tenantUUID,
				Namespace:      "ai_craft.current",
				Name:           "progress",
				FullTopic:      "tenant-seed.ai_craft.current.progress",
			},
		},
	}
	bindings := &seedBindingStoreStub{
		topics: map[string]TopicBindingRecord{
			"ai_craft.old.progress": {
				TenantUUID: tenantUUID,
				PluginID:   pluginID,
				TopicKey:   "ai_craft.old.progress",
				Namespace:  "ai_craft.old",
				Name:       "progress",
				FullTopic:  "tenant-seed.ai_craft.old.progress",
				TopicID:    staleID.String(),
			},
		},
		acls: map[string]struct{}{
			"ai_craft.old.progress": {},
		},
	}
	svc := NewSeedService(SeedServiceOptions{
		Directory: dir,
		ACL:       seedACLStub{},
		Bindings:  bindings,
	})

	_, err := svc.ApplyPlan(ctx, &SeedPlan{Topics: []TopicPlan{
		{
			Key: "ai_craft.current.progress",
			Topic: directory.CreateTopicInput{
				TenantUUID:    tenantUUID,
				Namespace:     "ai_craft.current",
				Name:          "progress",
				PayloadFormat: "json",
			},
		},
	}}, SeedContext{TenantUUID: tenantUUID, PluginID: pluginID, PluginVersion: "0.1.0"})
	require.NoError(t, err)
	require.Contains(t, dir.retired, staleID.String())
	require.NotContains(t, bindings.topics, "ai_craft.old.progress")
	require.NotContains(t, bindings.acls, "ai_craft.old.progress")
	require.Contains(t, bindings.topics, "ai_craft.current.progress")
}

func TestApplyPlanUpsertsExistingManifestTopic(t *testing.T) {
	ctx := context.Background()
	tenantUUID := "tenant-seed"
	pluginID := "com.powerx.plugins.ai-craft"
	topicID := uuid.New()
	dir := &seedDirectoryStub{
		existing: map[string]*eventfabricmodel.TopicDefinition{
			"tenant-seed|ai_craft.current|progress": {
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: topicID},
				TenantKey:      tenantUUID,
				Namespace:      "ai_craft.current",
				Name:           "progress",
				FullTopic:      "tenant-seed.ai_craft.current.progress",
				PayloadFormat:  "json",
				VersioningMode: "strict",
				MaxRetry:       1,
				AckTimeoutSec:  10,
			},
		},
	}
	svc := NewSeedService(SeedServiceOptions{
		Directory: dir,
		ACL:       seedACLStub{},
		Bindings:  &seedBindingStoreStub{},
	})

	_, err := svc.ApplyPlan(ctx, &SeedPlan{Topics: []TopicPlan{
		{
			Key: "ai_craft.current.progress",
			Topic: directory.CreateTopicInput{
				TenantUUID:     tenantUUID,
				Namespace:      "ai_craft.current",
				Name:           "progress",
				PayloadFormat:  "cloudevents",
				VersioningMode: "loose",
				MaxRetry:       9,
				AckTimeoutSec:  90,
			},
		},
	}}, SeedContext{TenantUUID: tenantUUID, PluginID: pluginID, PluginVersion: "0.1.0"})
	require.NoError(t, err)
	updated := dir.existing["tenant-seed|ai_craft.current|progress"]
	require.Equal(t, "cloudevents", updated.PayloadFormat)
	require.Equal(t, "loose", updated.VersioningMode)
	require.Equal(t, 9, updated.MaxRetry)
	require.Equal(t, 90, updated.AckTimeoutSec)
}

type seedDirectoryStub struct {
	existing map[string]*eventfabricmodel.TopicDefinition
	retired  map[string]directory.UpdateLifecycleInput
}

func (s *seedDirectoryStub) FindTopicByFullName(_ context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	if s.existing == nil {
		return nil, nil
	}
	return s.existing[tenantKey+"|"+namespace+"|"+name], nil
}

func (s *seedDirectoryStub) CreateTopic(_ context.Context, input directory.CreateTopicInput) (*directory.Topic, error) {
	id := uuid.New().String()
	return &directory.Topic{
		ID:        id,
		TenantKey: input.TenantUUID,
		Namespace: input.Namespace,
		Name:      input.Name,
		FullTopic: input.TenantUUID + "." + input.Namespace + "." + input.Name,
		MaxRetry:  input.MaxRetry,
		Lifecycle: eventfabricmodel.TopicLifecycleActive,
	}, nil
}

func (s *seedDirectoryStub) UpsertTopic(ctx context.Context, input directory.UpsertTopicInput) (*directory.Topic, bool, error) {
	key := input.TenantUUID + "|" + input.Namespace + "|" + input.Name
	if s.existing == nil {
		s.existing = map[string]*eventfabricmodel.TopicDefinition{}
	}
	if existing := s.existing[key]; existing != nil {
		existing.PayloadFormat = input.PayloadFormat
		existing.VersioningMode = input.VersioningMode
		existing.MaxRetry = int(input.MaxRetry)
		existing.AckTimeoutSec = int(input.AckTimeoutSec)
		return &directory.Topic{
			ID:             existing.UUID.String(),
			TenantKey:      existing.TenantKey,
			Namespace:      existing.Namespace,
			Name:           existing.Name,
			FullTopic:      existing.FullTopic,
			PayloadFormat:  existing.PayloadFormat,
			VersioningMode: existing.VersioningMode,
			MaxRetry:       int32(existing.MaxRetry),
			AckTimeoutSec:  int32(existing.AckTimeoutSec),
			Lifecycle:      existing.Lifecycle,
		}, false, nil
	}
	topic, err := s.CreateTopic(ctx, directory.CreateTopicInput(input))
	if err != nil {
		return nil, false, err
	}
	s.existing[key] = &eventfabricmodel.TopicDefinition{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.MustParse(topic.ID)},
		TenantKey:      topic.TenantKey,
		Namespace:      topic.Namespace,
		Name:           topic.Name,
		FullTopic:      topic.FullTopic,
		PayloadFormat:  topic.PayloadFormat,
		VersioningMode: topic.VersioningMode,
		MaxRetry:       int(topic.MaxRetry),
		AckTimeoutSec:  int(topic.AckTimeoutSec),
		Lifecycle:      topic.Lifecycle,
	}
	return topic, true, nil
}

func (s *seedDirectoryStub) UpdateLifecycle(_ context.Context, input directory.UpdateLifecycleInput) (*directory.Topic, error) {
	if s.retired == nil {
		s.retired = map[string]directory.UpdateLifecycleInput{}
	}
	s.retired[input.TopicID] = input
	return &directory.Topic{ID: input.TopicID, Lifecycle: input.TargetState}, nil
}

type seedACLStub struct{}

func (seedACLStub) Grant(context.Context, acl.GrantRequest) ([]*acl.Binding, error) {
	return nil, nil
}

type seedBindingStoreStub struct {
	topics map[string]TopicBindingRecord
	acls   map[string]struct{}
}

func (s *seedBindingStoreStub) GetTopic(_ context.Context, _, _, topicKey string) (*TopicBindingRecord, error) {
	record, ok := s.topics[topicKey]
	if !ok {
		return nil, nil
	}
	return &record, nil
}

func (s *seedBindingStoreStub) ListTopics(context.Context, string, string) ([]TopicBindingRecord, error) {
	out := make([]TopicBindingRecord, 0, len(s.topics))
	for _, record := range s.topics {
		out = append(out, record)
	}
	return out, nil
}

func (s *seedBindingStoreStub) UpsertTopic(_ context.Context, record TopicBindingRecord) error {
	if s.topics == nil {
		s.topics = map[string]TopicBindingRecord{}
	}
	s.topics[record.TopicKey] = record
	return nil
}

func (s *seedBindingStoreStub) DeleteTopic(_ context.Context, _, _, topicKey string) error {
	delete(s.topics, topicKey)
	return nil
}

func (s *seedBindingStoreStub) GetACL(context.Context, string, string, string, string, string) (*ACLBindingRecord, error) {
	return nil, nil
}

func (s *seedBindingStoreStub) UpsertACL(context.Context, ACLBindingRecord) error {
	return nil
}

func (s *seedBindingStoreStub) DeleteACLByTopic(_ context.Context, _, _, topicKey string) error {
	delete(s.acls, topicKey)
	return nil
}
