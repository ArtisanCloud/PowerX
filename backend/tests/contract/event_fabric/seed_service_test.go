package eventfabric_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

func TestSeedServiceCreatesTopicsAndACL(t *testing.T) {
	dirStub := newDirectoryStub()
	aclStub := &aclStub{}
	auditStub := &auditStub{}
	bindingStore := newBindingStoreStub()
	service := manifest.NewSeedService(manifest.SeedServiceOptions{
		Directory: dirStub,
		ACL:       aclStub,
		Audit:     auditStub,
		Bindings:  bindingStore,
		Clock:     func() time.Time { return time.Unix(0, 0).UTC() },
	})

	m, err := manifest.Parse(strings.NewReader(sampleManifest))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	ctx := manifest.SeedContext{
		TenantUUID:    "tenant-demo",
		PluginID:      "demo-plugin",
		PluginVersion: "1.0.0",
		Operator:      "tester",
		Variables: map[string]string{
			"cluster": "east",
		},
	}
	result, err := service.ApplyManifest(context.Background(), m, ctx)
	if err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	if result == nil || len(result.Topics) != 2 {
		t.Fatalf("expected 2 topics result, got %#v", result)
	}
	if len(dirStub.topics) != 2 {
		t.Fatalf("expected 2 topics created, got %d", len(dirStub.topics))
	}
	if len(aclStub.requests) != 3 {
		t.Fatalf("expected 3 acl grants, got %d", len(aclStub.requests))
	}
	if len(auditStub.records) == 0 {
		t.Fatalf("expected audit records to be written")
	}
	if len(bindingStore.topics) != 2 {
		t.Fatalf("expected topic bindings recorded, got %d", len(bindingStore.topics))
	}
	if len(bindingStore.acls) != 3 {
		t.Fatalf("expected acl bindings recorded, got %d", len(bindingStore.acls))
	}
}

func TestSeedServiceSkipsExistingTopic(t *testing.T) {
	dirStub := newDirectoryStub()
	aclStub := &aclStub{}
	auditStub := &auditStub{}
	bindingStore := newBindingStoreStub()

	// Preload existing topic
	dirStub.topics["tenant-demo.orders.created"] = &eventfabricmodel.TopicDefinition{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
		TenantKey:      "tenant-demo",
		Namespace:      "orders",
		Name:           "created",
		FullTopic:      "tenant-demo.orders.created",
	}

	service := manifest.NewSeedService(manifest.SeedServiceOptions{
		Directory: dirStub,
		ACL:       aclStub,
		Audit:     auditStub,
		Bindings:  bindingStore,
		Clock:     func() time.Time { return time.Unix(0, 0).UTC() },
	})

	manifestContent := `
version: 1
topics:
  - namespace: orders
    name: created
    acl:
      - principal_type: service
        principal_id: foo
        actions: [publish]
`
	m, err := manifest.Parse(strings.NewReader(manifestContent))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	ctx := manifest.SeedContext{TenantUUID: "tenant-demo"}
	if _, err := service.ApplyManifest(context.Background(), m, ctx); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	if dirStub.createCount != 0 {
		t.Fatalf("expected no new topics created, count=%d", dirStub.createCount)
	}
	if len(aclStub.requests) != 1 {
		t.Fatalf("expected one acl request, got %d", len(aclStub.requests))
	}
}

func TestSeedServiceSkipsACLWhenBindingMatches(t *testing.T) {
	dirStub := newDirectoryStub()
	aclStub := &aclStub{}
	auditStub := &auditStub{}
	bindingStore := newBindingStoreStub()
	bindingStore.existingACL[bindingStore.aclKey("tenant-demo", "demo-plugin", "orders.created", "service", "demo-plugin-writer")] = manifest.ACLBindingRecord{
		TenantUUID:    "tenant-demo",
		PluginID:      "demo-plugin",
		TopicKey:      "orders.created",
		PrincipalType: "service",
		PrincipalID:   "demo-plugin-writer",
		ActionsHash:   "publish",
	}

	service := manifest.NewSeedService(manifest.SeedServiceOptions{
		Directory: dirStub,
		ACL:       aclStub,
		Audit:     auditStub,
		Bindings:  bindingStore,
		Clock:     func() time.Time { return time.Unix(0, 0).UTC() },
	})

	m, err := manifest.Parse(strings.NewReader(sampleManifest))
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	ctx := manifest.SeedContext{
		TenantUUID:    "tenant-demo",
		PluginID:      "demo-plugin",
		PluginVersion: "1.0.0",
		Variables: map[string]string{
			"cluster": "east",
		},
	}
	if _, err := service.ApplyManifest(context.Background(), m, ctx); err != nil {
		t.Fatalf("apply manifest: %v", err)
	}
	if len(aclStub.requests) != 2 {
		t.Fatalf("expected duplicate ACL to be skipped, got %d grants", len(aclStub.requests))
	}
}

type directoryStub struct {
	topics      map[string]*eventfabricmodel.TopicDefinition
	createCount int
}

func newDirectoryStub() *directoryStub {
	return &directoryStub{
		topics: make(map[string]*eventfabricmodel.TopicDefinition),
	}
}

func (s *directoryStub) FindTopicByFullName(_ context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	key := fmt.Sprintf("%s.%s.%s", tenantKey, namespace, name)
	if topic, ok := s.topics[key]; ok {
		return topic, nil
	}
	return nil, nil
}

func (s *directoryStub) CreateTopic(_ context.Context, input directory.CreateTopicInput) (*directory.Topic, error) {
	key := fmt.Sprintf("%s.%s.%s", input.TenantUUID, input.Namespace, input.Name)
	if _, exists := s.topics[key]; exists {
		return nil, fmt.Errorf("topic already exists")
	}
	id := uuid.New()
	s.topics[key] = &eventfabricmodel.TopicDefinition{
		PowerUUIDModel: coremodel.PowerUUIDModel{UUID: id},
		TenantKey:      input.TenantUUID,
		Namespace:      input.Namespace,
		Name:           input.Name,
		FullTopic:      key,
	}
	s.createCount++
	return &directory.Topic{
		ID:         id.String(),
		TenantUUID: input.TenantUUID,
		TenantKey:  input.TenantUUID,
		Namespace:  input.Namespace,
		Name:       input.Name,
		FullTopic:  key,
	}, nil
}

type aclStub struct {
	requests []acl.GrantRequest
}

func (s *aclStub) Grant(_ context.Context, req acl.GrantRequest) ([]*acl.Binding, error) {
	s.requests = append(s.requests, req)
	resp := make([]*acl.Binding, 0, len(req.Actions))
	for _, action := range req.Actions {
		resp = append(resp, &acl.Binding{
			ID:            uuid.NewString(),
			TenantUUID:    req.TenantUUID,
			TopicUUID:     req.TopicUUID,
			PrincipalType: req.PrincipalType,
			PrincipalID:   req.PrincipalID,
			Action:        action,
		})
	}
	return resp, nil
}

type auditStub struct {
	records []eventaudit.Record
}

func (a *auditStub) Write(_ context.Context, record eventaudit.Record) error {
	a.records = append(a.records, record)
	return nil
}

type bindingStoreStub struct {
	topics      map[string]manifest.TopicBindingRecord
	acls        map[string]manifest.ACLBindingRecord
	existingACL map[string]manifest.ACLBindingRecord
}

func newBindingStoreStub() *bindingStoreStub {
	return &bindingStoreStub{
		topics:      map[string]manifest.TopicBindingRecord{},
		acls:        map[string]manifest.ACLBindingRecord{},
		existingACL: map[string]manifest.ACLBindingRecord{},
	}
}

func (s *bindingStoreStub) topicKey(tenantUUID, pluginID, topicKey string) string {
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(strings.ToLower(tenantUUID)), strings.TrimSpace(pluginID), strings.TrimSpace(topicKey))
}

func (s *bindingStoreStub) aclKey(tenantUUID, pluginID, topicKey, principalType, principalID string) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s",
		strings.TrimSpace(strings.ToLower(tenantUUID)),
		strings.TrimSpace(pluginID),
		strings.TrimSpace(topicKey),
		strings.ToLower(strings.TrimSpace(principalType)),
		strings.TrimSpace(principalID),
	)
}

func (s *bindingStoreStub) GetTopic(_ context.Context, tenantUUID, pluginID, topicKey string) (*manifest.TopicBindingRecord, error) {
	if rec, ok := s.topics[s.topicKey(tenantUUID, pluginID, topicKey)]; ok {
		result := rec
		return &result, nil
	}
	return nil, nil
}

func (s *bindingStoreStub) UpsertTopic(_ context.Context, record manifest.TopicBindingRecord) error {
	s.topics[s.topicKey(record.TenantUUID, record.PluginID, record.TopicKey)] = record
	return nil
}

func (s *bindingStoreStub) GetACL(_ context.Context, tenantUUID, pluginID, topicKey, principalType, principalID string) (*manifest.ACLBindingRecord, error) {
	key := s.aclKey(tenantUUID, pluginID, topicKey, principalType, principalID)
	if rec, ok := s.existingACL[key]; ok {
		result := rec
		return &result, nil
	}
	if rec, ok := s.acls[key]; ok {
		result := rec
		return &result, nil
	}
	return nil, nil
}

func (s *bindingStoreStub) UpsertACL(_ context.Context, record manifest.ACLBindingRecord) error {
	key := s.aclKey(record.TenantUUID, record.PluginID, record.TopicKey, record.PrincipalType, record.PrincipalID)
	s.acls[key] = record
	return nil
}
