package bus

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
)

func TestAuthorizeAllowsRegisteredTopicWithACL(t *testing.T) {
	topicID := uuid.New()
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{
			topic: &eventfabricmodel.TopicDefinition{
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: topicID},
				TenantKey:      "tenant-authorizer",
				Namespace:      "custom",
				Name:           "progress",
			},
		},
		ACLStore: &mockACLStore{allowed: true},
		Clock:    time.Now,
	})
	client := &Client{TenantUUID: "tenant-authorizer", MemberID: 1}
	if err := authorizer.Authorize(context.Background(), client, "custom.progress"); err != nil {
		t.Fatalf("expected registered topic allowed, got err=%v", err)
	}
}

func TestAuthorizeRejectsTopicWithoutRegistry(t *testing.T) {
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{topic: nil},
		ACLStore:   &mockACLStore{allowed: true},
		Clock:      time.Now,
	})
	client := &Client{TenantUUID: "tenant-authorizer", MemberID: 1}
	err := authorizer.Authorize(context.Background(), client, "custom.progress")
	if !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("expected ErrTopicNotAllowed, got err=%v", err)
	}
}

func TestAuthorizePublishRejectsTemplateOnlyTopic(t *testing.T) {
	topicID := uuid.New()
	acl := &mockACLStore{allowed: true}
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{
			templateTopic: &eventfabricmodel.TopicDefinition{
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: topicID},
				TenantKey:      "tenant-authorizer",
				Namespace:      "ai_craft.shopify.product.sync.progress.member.tenant_{{tenant_uuid}}",
				Name:           "member_{{member_uuid}}",
			},
		},
		ACLStore: acl,
		Clock:    time.Now,
	})
	err := authorizer.AuthorizePublish(context.Background(), PublishAuthorizeInput{
		TenantUUID: "tenant-authorizer",
		Topic:      "ai_craft.shopify.product.sync.progress.member.tenant_tenant-authorizer.member_fda3589b-d30b-41b0-a859-c061c179fb58",
		Principal:  "plugin:com.powerx.plugins.ai-craft",
	})
	if err != nil {
		if !errors.Is(err, ErrTopicNotAllowed) {
			t.Fatalf("expected ErrTopicNotAllowed, got err=%v", err)
		}
		return
	}
	t.Fatalf("expected ErrTopicNotAllowed")
}

func TestAuthorizePublishRejectsUnregisteredTemplateTopic(t *testing.T) {
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{
			templateTopic: &eventfabricmodel.TopicDefinition{
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
				TenantKey:      "tenant-authorizer",
				Namespace:      "ai_craft.shopify.product.sync.progress.member.tenant_{{tenant_uuid}}",
				Name:           "member_{{member_uuid}}",
			},
		},
		ACLStore: &mockACLStore{allowed: true},
		Clock:    time.Now,
	})
	err := authorizer.AuthorizePublish(context.Background(), PublishAuthorizeInput{
		TenantUUID: "tenant-authorizer",
		Topic:      "ai_craft.shopify.product.sync.progress.member.tenant_other-tenant.member_fda3589b-d30b-41b0-a859-c061c179fb58",
		Principal:  "plugin:com.powerx.plugins.ai-craft",
	})
	if !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("expected ErrTopicNotAllowed, got err=%v", err)
	}
}

func TestAuthorizeRejectsCrossTenantTopic(t *testing.T) {
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{topic: &eventfabricmodel.TopicDefinition{
			PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()},
			TenantKey:      "tenant-authorizer",
			Namespace:      "custom",
			Name:           "progress",
		}},
		ACLStore: &mockACLStore{allowed: true},
		Clock:    time.Now,
	})
	client := &Client{TenantUUID: "tenant-authorizer", MemberID: 1}
	err := authorizer.Authorize(context.Background(), client, "00000000-0000-0000-0000-000000000999.custom.progress")
	if !errors.Is(err, ErrTopicNotAllowed) {
		t.Fatalf("expected ErrTopicNotAllowed for cross-tenant topic, got err=%v", err)
	}
}

func TestAuthorizePublishUsesExplicitPrincipal(t *testing.T) {
	topicID := uuid.New()
	acl := &mockACLStore{allowed: true}
	authorizer := NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		TopicStore: &mockTopicLookup{
			topic: &eventfabricmodel.TopicDefinition{
				PowerUUIDModel: coremodel.PowerUUIDModel{UUID: topicID},
				TenantKey:      "tenant-authorizer",
				Namespace:      "custom",
				Name:           "progress",
			},
		},
		ACLStore: acl,
		Clock:    time.Now,
	})
	err := authorizer.AuthorizePublish(context.Background(), PublishAuthorizeInput{
		TenantUUID: "tenant-authorizer",
		Topic:      "custom.progress",
		Principal:  "plugin:com.powerx.plugins.ai-craft",
	})
	if err != nil {
		t.Fatalf("AuthorizePublish() err = %v", err)
	}
	if acl.principal != "plugin:com.powerx.plugins.ai-craft" {
		t.Fatalf("principal=%q, want plugin principal", acl.principal)
	}
	if acl.action != "publish" {
		t.Fatalf("action=%q, want publish", acl.action)
	}
}

type mockTopicLookup struct {
	topic         *eventfabricmodel.TopicDefinition
	templateTopic *eventfabricmodel.TopicDefinition
}

func (m *mockTopicLookup) FindByComposite(_ context.Context, _, _, _ string) (*eventfabricmodel.TopicDefinition, error) {
	if m.topic == nil {
		return nil, nil
	}
	clone := *m.topic
	return &clone, nil
}

func (m *mockTopicLookup) FindTemplateMatch(_ context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	if m.templateTopic == nil {
		return nil, nil
	}
	expectedNamespace := strings.ReplaceAll(m.templateTopic.Namespace, "{{tenant_uuid}}", tenantKey)
	if expectedNamespace != namespace {
		return nil, nil
	}
	if !strings.HasPrefix(name, "member_") {
		return nil, nil
	}
	clone := *m.templateTopic
	return &clone, nil
}

type mockACLStore struct {
	allowed   bool
	principal string
	action    string
}

func (m *mockACLStore) HasPermission(_ context.Context, _ string, _ uuid.UUID, principal string, action string, _ time.Time) (bool, error) {
	m.principal = principal
	m.action = action
	return m.allowed, nil
}
