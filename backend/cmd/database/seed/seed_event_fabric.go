package seed

import (
	"fmt"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type eventFabricTopicSeed struct {
	TenantKey     string
	Namespace     string
	Name          string
	PayloadFormat string
	CreatedBy     string
}

func SeedEventFabricTopics(db *gorm.DB) error {
	repo := eventfabricrepo.NewTopicRepository(db)

	builtinSystemTopics := []string{
		eventbus.TopicKnowledgeIngestionJob,
		eventbus.TopicKnowledgeCorpusCheckJob,
		eventbus.TopicSystemNotification,
		eventbus.TopicIntegrationGatewayRouteCreated,
		eventbus.TopicIntegrationGatewayRouteUpdated,
		eventbus.TopicIntegrationGatewayInvocationSucceeded,
		eventbus.TopicIntegrationGatewayInvocationFailed,
		eventbus.TopicIntegrationGatewayInvocationFallback,
		eventbus.TopicCapabilityCatalogSyncStarted,
		eventbus.TopicCapabilityCatalogSyncSucceeded,
		eventbus.TopicCapabilityCatalogSyncFailed,
		eventbus.TopicCapabilityPolicyDegraded,
		eventbus.TopicKnowledgeFeedbackReprocess,
	}

	seeds := make([]eventFabricTopicSeed, 0, len(builtinSystemTopics))
	for _, topic := range builtinSystemTopics {
		namespace, name, ok := splitTopic(topic)
		if !ok {
			continue
		}
		seeds = append(seeds, eventFabricTopicSeed{
			TenantKey:     "global",
			Namespace:     namespace,
			Name:          name,
			PayloadFormat: "json",
			CreatedBy:     "seed",
		})
	}

	for i := range seeds {
		seed := seeds[i]
		existing, err := repo.FindByComposite(seedCtx(), seed.TenantKey, seed.Namespace, seed.Name)
		if err != nil {
			return fmt.Errorf("query topic %s.%s.%s failed: %w", seed.TenantKey, seed.Namespace, seed.Name, err)
		}
		if existing != nil {
			continue
		}

		row := &eventfabricmodel.TopicDefinition{
			ScopeType:       eventfabricmodel.TopicScopeSystem,
			ScopeID:         seed.TenantKey,
			TenantKey:       seed.TenantKey,
			Namespace:       seed.Namespace,
			Name:            seed.Name,
			Lifecycle:       eventfabricmodel.TopicLifecycleActive,
			PayloadFormat:   seed.PayloadFormat,
			VersioningMode:  "strict",
			MaxRetry:        5,
			AckTimeoutSec:   30,
			RetentionPolicy: datatypes.JSON([]byte(`{"mode":"standard"}`)),
			Metadata:        datatypes.JSON([]byte(`{"seed":"event_fabric"}`)),
			CreatedBy:       seed.CreatedBy,
			Status:          1,
		}
		if _, err := repo.Create(seedCtx(), row); err != nil {
			return fmt.Errorf("create topic %s.%s.%s failed: %w", seed.TenantKey, seed.Namespace, seed.Name, err)
		}
	}

	fmt.Printf("[seed] event fabric topics ready: %d\n", len(seeds))
	return nil
}

func splitTopic(topic string) (namespace string, name string, ok bool) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	namespace = strings.Join(parts[:len(parts)-1], ".")
	name = strings.TrimSpace(parts[len(parts)-1])
	if namespace == "" || name == "" {
		return "", "", false
	}
	return namespace, name, true
}

func SeedEventFabricDefaultACL(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}
	var topics []*eventfabricmodel.TopicDefinition
	if err := db.WithContext(seedCtx()).
		Where("deleted_at IS NULL").
		Find(&topics).Error; err != nil {
		return fmt.Errorf("list event topics failed: %w", err)
	}
	if len(topics) == 0 {
		fmt.Println("[seed] event fabric default acl skipped: no topics")
		return nil
	}

	now := time.Now().UTC()
	bindings := make([]*eventfabricmodel.AclBinding, 0, len(topics)*3)
	for _, topic := range topics {
		if topic == nil {
			continue
		}
		for _, action := range []string{"publish", "subscribe", "replay"} {
			bindings = append(bindings, &eventfabricmodel.AclBinding{
				TenantKey:     topic.TenantKey,
				TopicUUID:     topic.UUID,
				PrincipalType: "role",
				PrincipalID:   "role:role_admin",
				Action:        action,
				GrantedBy:     "seed",
				Justification: "seed default admin access",
				Status:        1,
				PowerUUIDModel: coremodel.PowerUUIDModel{
					CreatedAt: now,
					UpdatedAt: now,
				},
			})
		}
	}
	if len(bindings) == 0 {
		return nil
	}

	repo := eventfabricrepo.NewAclRepository(db)
	if _, err := repo.UpsertBindings(seedCtx(), bindings); err != nil {
		return fmt.Errorf("upsert event fabric default acl failed: %w", err)
	}
	fmt.Printf("[seed] event fabric default acl ready: topics=%d bindings=%d\n", len(topics), len(bindings))
	return nil
}
