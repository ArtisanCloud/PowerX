package pluginbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Service struct {
	db              *gorm.DB
	namespaceOwners map[string]string
}

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) WithNamespaceOwners(owners map[string]string) *Service {
	s.namespaceOwners = map[string]string{}
	for namespace, plugin := range owners {
		s.namespaceOwners[namespace] = plugin
	}
	return s
}

func (s *Service) namespaceAllowed(plugin, namespace string) bool {
	matched := ""
	for root, owner := range s.namespaceOwners {
		if root == "" || owner == "" || root != strings.TrimSpace(root) {
			return false
		}
		if namespace == root || strings.HasPrefix(namespace, root+".") {
			if matched != "" && matched != owner {
				return false
			}
			matched = owner
		}
	}
	if matched != "" {
		return matched == plugin
	}
	return manifest.PluginNamespaceAllowed(plugin, namespace)
}

// Prepare 仅受控安装/租户启用使用 provision=true。persist 与主题、ACL、绑定记录在同一数据库事务内执行。
// 文件提交方必须在本方法返回失败时恢复文件；普通恢复 provision=false，不执行 persist，也不创建资源。
func (s *Service) Prepare(ctx context.Context, p pm.Plugin, tenant, path string, provision bool, persist func(*gorm.DB, map[string]string) error) error {
	id, err := uuid.Parse(tenant)
	if err != nil || id == uuid.Nil || id.String() != tenant {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_TENANT_INVALID")
	}
	if s == nil || s.db == nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_SERVICE_REQUIRED")
	}
	verifiedPath, err := ManifestPath(p)
	if err != nil {
		return err
	}
	if path != verifiedPath {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_MANIFEST_CONFLICT")
	}
	if provision && persist == nil {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_COMMIT_REQUIRED")
	}
	run := func(db *gorm.DB) error {
		if provision && db.Dialector.Name() == "postgres" {
			// 同一租户的主题创建/认领串行，避免两个安装事务抢占同一主题。
			if err := db.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", "plugin-event-fabric:"+tenant).Error; err != nil {
				return err
			}
		}
		dir := directory.NewDirectoryService(directory.Options{DB: db})
		permissions := acl.NewACLService(acl.Options{DB: db}) // 不把事务中尚未提交的 ACL 放进共享缓存。
		store := manifest.NewBindingStore(repo.NewManifestBindingRepository(db))
		existing, err := ExplicitBindings(p)
		if err != nil {
			return err
		}
		if p.HostConfig != nil {
			if configured, ok := Bridge(p.HostConfig.Spec)["tenant_uuid"]; ok && configured != tenant {
				return fmt.Errorf("EVENT_FABRIC_INSTALL_BINDING_TENANT_MISMATCH")
			}
		}
		var actions map[string][]string
		bindings := existing
		if path != "" {
			plan, seed, declaredActions, err := SeedPlan(p, tenant, path, nil)
			if err != nil {
				return err
			}
			actions = declaredActions
			wanted := make(map[string]string, len(plan.Topics))
			for _, topic := range plan.Topics {
				wanted[topic.Key] = topic.FullTopic
			}
			old, err := store.ListTopics(ctx, tenant, p.ID)
			if err != nil {
				return err
			}
			generated := map[string]string{}
			for _, record := range old {
				generated[record.TopicKey] = record.FullTopic
			}
			for key, full := range existing {
				knownGenerated := generated[key] == full
				if provision && !knownGenerated {
					relative, ok := strings.CutPrefix(full, tenant+".")
					split := strings.LastIndex(relative, ".")
					if ok && split > 0 && split < len(relative)-1 {
						topic, err := dir.FindTopicByFullName(ctx, tenant, relative[:split], relative[split+1:])
						if err != nil {
							return err
						}
						knownGenerated = topic != nil && topic.TenantKey == tenant && topic.FullTopic == full && directory.GeneratedPluginBinding(topic, p.ID, key)
					}
				}
				if full != wanted[key] && (!provision || !knownGenerated) {
					return fmt.Errorf("EVENT_FABRIC_INSTALL_BINDING_CONFLICT: %s", key)
				}
			}
			if provision {
				if err := checkNamespaces(ctx, db, p.ID, tenant, plan); err != nil {
					return err
				}
				seeder := manifest.NewSeedService(manifest.SeedServiceOptions{Directory: dir, ACL: permissions, Bindings: store, NamespaceAllowed: s.namespaceAllowed})
				result, err := seeder.PreparePlan(ctx, plan, seed)
				if err != nil {
					return err
				}
				if result == nil || len(result.Topics) != len(wanted) {
					return fmt.Errorf("EVENT_FABRIC_INSTALL_RECEIPTS_INCOMPLETE")
				}
				bindings = map[string]string{}
				for _, receipt := range result.Topics {
					if receipt.TopicID == "" || receipt.FullTopic != wanted[receipt.Key] || bindings[receipt.Key] != "" {
						return fmt.Errorf("EVENT_FABRIC_INSTALL_RECEIPT_INVALID")
					}
					bindings[receipt.Key] = receipt.FullTopic
				}
			} else {
				if len(existing) != len(wanted) {
					return fmt.Errorf("EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED")
				}
				for key, full := range wanted {
					if existing[key] != full {
						return fmt.Errorf("EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED: %s", key)
					}
				}
			}
		} else {
			actions, err = ExplicitActions(p, bindings)
			if err != nil {
				return err
			}
		}
		bindings, err = Verify(ctx, dir, permissions, p.ID, tenant, bindings, actions)
		if err != nil {
			return err
		}
		if provision {
			metadata, _ := json.Marshal(map[string]any{"plugin_id": p.ID, "plugin_version": p.Version, "topic_count": len(bindings)})
			event := auditmodel.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: tenant, Source: "plugin.event_fabric", Operation: "PLUGIN_EVENT_FABRIC_PREPARE", ResourceType: "plugin", ResourceID: p.ID, ResourceName: p.ID, Outcome: "SUCCESS", Severity: "INFO", Meta: metadata}
			if claims := reqctx.GetClaims(ctx); claims != nil && claims.UserID > 0 {
				id := int64(claims.UserID)
				event.ActorUserID = &id
			}
			if err := db.Create(&event).Error; err != nil {
				return err
			}
			return persist(db, bindings)
		}
		return nil
	}
	if provision {
		return s.db.WithContext(ctx).Transaction(run)
	}
	return run(s.db.WithContext(ctx))
}

// 拒绝覆盖已被其他插件或公共资源占用的命名空间；别名拼写碰撞也不能产生共享所有权。
func checkNamespaces(ctx context.Context, db *gorm.DB, plugin, tenant string, plan *manifest.SeedPlan) error {
	var topics []model.TopicDefinition
	if err := db.WithContext(ctx).Where("tenant_key = ?", tenant).Find(&topics).Error; err != nil {
		return err
	}
	owners := repo.NewManifestBindingRepository(db)
	for _, wanted := range plan.Topics {
		for i := range topics {
			topic := &topics[i]
			if topic.Namespace != wanted.Topic.Namespace && !strings.HasPrefix(topic.Namespace, wanted.Topic.Namespace+".") && !strings.HasPrefix(wanted.Topic.Namespace, topic.Namespace+".") {
				continue
			}
			owner := directory.PluginOwner(topic)
			if owner != "" && owner != plugin {
				return fmt.Errorf("EVENT_FABRIC_INSTALL_NAMESPACE_DENIED: %s", wanted.Topic.Namespace)
			}
			previous, err := owners.TopicOwners(ctx, tenant, topic.UUID.String())
			if err != nil {
				return err
			}
			for _, id := range previous {
				if id != plugin {
					return fmt.Errorf("EVENT_FABRIC_INSTALL_NAMESPACE_DENIED: %s", wanted.Topic.Namespace)
				}
			}
			if owner == "" && len(previous) == 0 {
				return fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_OWNER_REQUIRED: %s", wanted.Key)
			}
		}
	}
	return nil
}

// TenantPlugin 保留包内声明，用租户配置中的映射替换全局进程的映射，禁止借用 System 映射。
func TenantPlugin(p pm.Plugin, config map[string]any) pm.Plugin {
	bridge := map[string]any{}
	if p.HostConfig != nil {
		root, _ := p.HostConfig.Spec["event_bridge"].(map[string]any)
		for _, b := range []map[string]any{root, Bridge(p.HostConfig.Spec)} {
			if value, ok := b["topic_manifest"]; ok {
				bridge["topic_manifest"] = value
			}
		}
	}
	for key, value := range Bridge(config) {
		if key != "topic_manifest" {
			bridge[key] = value
		}
	}
	p.HostConfig = &pm.HostConfig{Spec: map[string]any{"runtime": map[string]any{"event_bridge": bridge}}}
	if raw, exists := config["event_bridge"]; exists {
		if old, ok := raw.(map[string]any); ok {
			copy := map[string]any{}
			for key, value := range old {
				if key != "topic_manifest" {
					copy[key] = value
				}
			}
			p.HostConfig.Spec["event_bridge"] = copy
		} else {
			p.HostConfig.Spec["event_bridge"] = raw
		}
	}
	return p
}

func SetBindings(config map[string]any, tenant string, bindings map[string]string) {
	runtime, _ := config["runtime"].(map[string]any)
	if runtime == nil {
		runtime = map[string]any{}
		config["runtime"] = runtime
	}
	bridge, _ := runtime["event_bridge"].(map[string]any)
	if bridge == nil {
		bridge = map[string]any{}
		runtime["event_bridge"] = bridge
	}
	values := map[string]any{}
	for key, full := range bindings {
		values[key] = full
	}
	bridge["tenant_uuid"] = tenant
	bridge["topic_bindings"] = values
	bridge["contract_version"] = "event_fabric.service.v1"
	if root, ok := config["event_bridge"].(map[string]any); ok {
		root["tenant_uuid"] = tenant
		root["topic_bindings"] = values
		root["contract_version"] = "event_fabric.service.v1"
	}
}
