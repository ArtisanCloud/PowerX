package manifest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	eventaudit "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// directoryClient 抽象目录服务依赖，便于测试。
type directoryClient interface {
	FindTopicByFullName(ctx context.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
	CreateTopic(ctx context.Context, input directory.CreateTopicInput) (*directory.Topic, error)
	UpsertTopic(ctx context.Context, input directory.UpsertTopicInput) (*directory.Topic, bool, error)
	UpdateLifecycle(ctx context.Context, input directory.UpdateLifecycleInput) (*directory.Topic, error)
}

// aclClient 抽象 ACL 服务依赖。
type aclClient interface {
	Grant(ctx context.Context, req acl.GrantRequest) ([]*acl.Binding, error)
}

// SeedServiceOptions 配置 SeedService 行为。
type SeedServiceOptions struct {
	Directory  directoryClient
	ACL        aclClient
	Audit      eventaudit.Service
	Logger     *pxlog.Logger
	Clock      func() time.Time
	MaxRetries int
	Bindings   BindingStore
	// PluginUsageGuard rejects new plugin-owned Event Fabric resources while a plugin is draining.
	PluginUsageGuard func(ctx context.Context, pluginID string) error
	// NamespaceAllowed 由 Core 配置注入，包内声明不能扩大命名空间归属。
	NamespaceAllowed func(pluginID, namespace string) bool
}

// SeedService 根据 manifest 计划自动创建 Topic 与 ACL。
type SeedService struct {
	dir              directoryClient
	acl              aclClient
	audit            eventaudit.Service
	logger           *pxlog.Logger
	clock            func() time.Time
	maxRetries       int
	bindings         BindingStore
	guard            func(ctx context.Context, pluginID string) error
	namespaceAllowed func(string, string) bool
}

// SeedResult 描述一次播种操作的结果。
type SeedResult struct {
	Topics []TopicResult
}

// TopicResult 描述单个 Topic 的播种结果。
type TopicResult struct {
	Key            string
	FullTopic      string
	TopicID        string
	Created        bool
	GrantedActions int
}

// NewSeedService 构造播种服务。
func NewSeedService(opts SeedServiceOptions) *SeedService {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = pxlog.GetGlobalLogger()
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}
	return &SeedService{
		dir:              opts.Directory,
		acl:              opts.ACL,
		audit:            opts.Audit,
		logger:           opts.Logger,
		clock:            opts.Clock,
		maxRetries:       opts.MaxRetries,
		bindings:         opts.Bindings,
		guard:            opts.PluginUsageGuard,
		namespaceAllowed: opts.NamespaceAllowed,
	}
}

// ApplyManifest 解析并播种 manifest。
func (s *SeedService) ApplyManifest(ctx context.Context, m *Manifest, seedCtx SeedContext) (*SeedResult, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest is nil")
	}
	plan, err := m.Render(seedCtx)
	if err != nil {
		return nil, err
	}
	return s.ApplyPlan(ctx, plan, seedCtx)
}

// ApplyPlan 根据 SeedPlan 执行播种。
func (s *SeedService) ApplyPlan(ctx context.Context, plan *SeedPlan, seedCtx SeedContext) (*SeedResult, error) {
	return s.applyPlan(ctx, plan, seedCtx, true)
}

// PreparePlan only prepares declared resources before process launch. Existing
// bindings outside this plan stay intact if a new plugin version cannot start.
func (s *SeedService) PreparePlan(ctx context.Context, plan *SeedPlan, seedCtx SeedContext) (*SeedResult, error) {
	if plan == nil || seedCtx.PluginID == "" || seedCtx.TenantUUID == "" {
		return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_OWNER_REQUIRED")
	}
	for _, topic := range plan.Topics {
		for _, grant := range topic.ACL {
			if grant.PrincipalType != "plugin" || grant.PrincipalID != seedCtx.PluginID {
				return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_OWNER_INVALID: %s", topic.Key)
			}
			for _, action := range grant.Actions {
				if action != acl.PrincipalActionPublish && action != acl.PrincipalActionSubscribe {
					return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_ACTION_INVALID: %s", topic.Key)
				}
			}
		}
	}
	return s.applyPlan(ctx, plan, seedCtx, false)
}

func (s *SeedService) applyPlan(ctx context.Context, plan *SeedPlan, seedCtx SeedContext, prune bool) (*SeedResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("seed plan is nil")
	}
	if s.dir == nil || s.acl == nil {
		return nil, fmt.Errorf("seed service not fully configured")
	}
	if err := s.ensurePluginAcceptsNewUsage(ctx, seedCtx.PluginID); err != nil {
		return nil, err
	}
	result := &SeedResult{Topics: make([]TopicResult, 0, len(plan.Topics))}
	var combinedErr error
	currentTopicKeys := make(map[string]struct{}, len(plan.Topics))
	for _, topicPlan := range plan.Topics {
		currentTopicKeys[normalizeTopicKey(topicPlan.Key)] = struct{}{}
		topicRes := TopicResult{
			Key:       topicPlan.Key,
			FullTopic: topicPlan.FullTopic,
		}
		var record *topicRecord
		var created bool
		var err error
		if prune {
			// 旧声明式同步不能绕过新安装路径写入的 Core 归属。
			existing, lookupErr := s.dir.FindTopicByFullName(ctx, topicPlan.Topic.TenantUUID, topicPlan.Topic.Namespace, topicPlan.Topic.Name)
			if lookupErr != nil {
				return result, lookupErr
			}
			if owner := directory.PluginOwner(existing); owner != "" && owner != seedCtx.PluginID {
				return result, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_OWNER_CONFLICT: %s", topicPlan.Key)
			}
			record, created, err = s.upsertTopic(ctx, topicPlan.Topic)
		} else {
			record, created, err = s.ensureOwnedTopic(ctx, topicPlan, seedCtx)
		}
		if err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("topic %s: %w", topicPlan.FullTopic, err))
			result.Topics = append(result.Topics, topicRes)
			continue
		}
		topicRes.FullTopic = record.FullName()
		topicRes.TopicID = record.ID
		topicRes.Created = created
		if created {
			s.writeAudit(ctx, "TOPIC_CREATE", record.ID, record.FullName(), seedCtx, nil)
		}
		if err := s.recordTopicBinding(ctx, topicPlan, record, seedCtx); err != nil {
			return result, fmt.Errorf("record topic binding: %w", err)
		}

		grantCount := 0
		for _, aclPlan := range topicPlan.ACL {
			if !prune && !created {
				if err := s.verifyExistingACL(ctx, record, aclPlan); err != nil {
					return result, err
				}
			}
			applied, grantErr := s.applyACL(ctx, topicPlan.Key, record, aclPlan, seedCtx)
			if grantErr != nil {
				combinedErr = errors.Join(combinedErr, fmt.Errorf("topic %s acl %s/%s: %w",
					topicPlan.FullTopic, aclPlan.PrincipalType, aclPlan.PrincipalID, grantErr))
				continue
			}
			grantCount += applied
		}
		topicRes.GrantedActions = grantCount
		result.Topics = append(result.Topics, topicRes)
		if s.logger != nil {
			s.logger.InfoF(ctx, "[event_fabric.seed] topic=%s created=%t granted=%d plugin=%s tenant=%s",
				record.FullName(), created, grantCount, seedCtx.PluginID, seedCtx.TenantUUID)
		}
	}
	if prune {
		if err := s.pruneStaleTopicBindings(ctx, seedCtx, currentTopicKeys); err != nil {
			combinedErr = errors.Join(combinedErr, err)
		}
	}
	if combinedErr != nil {
		return result, combinedErr
	}
	return result, nil
}

func (s *SeedService) ensurePluginAcceptsNewUsage(ctx context.Context, pluginID string) error {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" || s.guard == nil {
		return nil
	}
	return s.guard(ctx, pluginID)
}

func (s *SeedService) recordTopicBinding(ctx context.Context, topicPlan TopicPlan, record *topicRecord, seedCtx SeedContext) error {
	if s.bindings == nil || record == nil || strings.TrimSpace(seedCtx.PluginID) == "" {
		return nil
	}
	topicKey := topicPlan.Key
	if strings.TrimSpace(topicKey) == "" {
		topicKey = fmt.Sprintf("%s.%s", record.Namespace, record.Name)
	}
	err := s.bindings.UpsertTopic(ctx, TopicBindingRecord{
		TenantUUID: seedCtx.TenantUUID,
		PluginID:   seedCtx.PluginID,
		TopicKey:   topicKey,
		Namespace:  record.Namespace,
		Name:       record.Name,
		FullTopic:  record.FullName(),
		TopicID:    record.ID,
	})
	return err
}

func (s *SeedService) pruneStaleTopicBindings(ctx context.Context, seedCtx SeedContext, currentTopicKeys map[string]struct{}) error {
	if s.bindings == nil || strings.TrimSpace(seedCtx.PluginID) == "" {
		return nil
	}
	existing, err := s.bindings.ListTopics(ctx, seedCtx.TenantUUID, seedCtx.PluginID)
	if err != nil {
		return fmt.Errorf("list stale topic bindings: %w", err)
	}
	var combinedErr error
	for _, binding := range existing {
		topicKey := normalizeTopicKey(binding.TopicKey)
		if _, ok := currentTopicKeys[topicKey]; ok {
			continue
		}
		if strings.TrimSpace(binding.TopicID) != "" {
			if _, err := s.dir.UpdateLifecycle(ctx, directory.UpdateLifecycleInput{
				TopicID:      binding.TopicID,
				TargetState:  eventfabricmodel.TopicLifecycleRetired,
				ChangeReason: fmt.Sprintf("plugin manifest no longer declares topic: plugin=%s version=%s topic_key=%s", seedCtx.PluginID, seedCtx.PluginVersion, binding.TopicKey),
			}); err != nil {
				combinedErr = errors.Join(combinedErr, fmt.Errorf("retire stale topic %s: %w", binding.FullTopic, err))
				continue
			}
			s.writeAudit(ctx, "TOPIC_RETIRE", binding.TopicID, binding.FullTopic, seedCtx, map[string]string{
				"topic_key": binding.TopicKey,
				"reason":    "manifest_prune",
			})
		}
		if err := s.bindings.DeleteACLByTopic(ctx, seedCtx.TenantUUID, seedCtx.PluginID, binding.TopicKey); err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("delete stale acl bindings %s: %w", binding.TopicKey, err))
			continue
		}
		if err := s.bindings.DeleteTopic(ctx, seedCtx.TenantUUID, seedCtx.PluginID, binding.TopicKey); err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("delete stale topic binding %s: %w", binding.TopicKey, err))
			continue
		}
		if s.logger != nil {
			s.logger.InfoF(ctx, "[event_fabric.seed] retired stale topic=%s plugin=%s tenant=%s",
				binding.FullTopic, seedCtx.PluginID, seedCtx.TenantUUID)
		}
	}
	return combinedErr
}

func (s *SeedService) recordAclBinding(ctx context.Context, topicKey string, plan ACLPlan, signature string, seedCtx SeedContext) error {
	if s.bindings == nil || strings.TrimSpace(seedCtx.PluginID) == "" {
		return nil
	}
	topicKey = strings.TrimSpace(topicKey)
	if topicKey == "" {
		topicKey = "default"
	}
	err := s.bindings.UpsertACL(ctx, ACLBindingRecord{
		TenantUUID:    seedCtx.TenantUUID,
		PluginID:      seedCtx.PluginID,
		TopicKey:      topicKey,
		PrincipalType: plan.PrincipalType,
		PrincipalID:   plan.PrincipalID,
		ActionsHash:   signature,
		Actions:       plan.Actions,
	})
	return err
}

func normalizeTopicKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return value
}

func (s *SeedService) ensureTopic(ctx context.Context, input directory.CreateTopicInput) (*topicRecord, bool, error) {
	tenantKey := strings.TrimSpace(input.TenantUUID)
	namespace := strings.TrimSpace(strings.ToLower(input.Namespace))
	name := strings.TrimSpace(strings.ToLower(input.Name))
	if tenantKey == "" || namespace == "" || name == "" {
		return nil, false, fmt.Errorf("invalid topic input")
	}

	existing, err := s.dir.FindTopicByFullName(ctx, tenantKey, namespace, name)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return topicFromModel(existing), false, nil
	}

	dto, err := s.dir.CreateTopic(ctx, input)
	if err != nil {
		if isTopicExistsErr(err) {
			if retryExisting, findErr := s.dir.FindTopicByFullName(ctx, tenantKey, namespace, name); findErr == nil && retryExisting != nil {
				return topicFromModel(retryExisting), false, nil
			}
		}
		return nil, false, err
	}
	return topicFromDTO(dto), true, nil
}

func (s *SeedService) upsertTopic(ctx context.Context, input directory.CreateTopicInput) (*topicRecord, bool, error) {
	dto, created, err := s.dir.UpsertTopic(ctx, directory.UpsertTopicInput(input))
	if err != nil {
		return nil, false, err
	}
	return topicFromDTO(dto), created, nil
}

func (s *SeedService) applyACL(ctx context.Context, topicKey string, topic *topicRecord, plan ACLPlan, seedCtx SeedContext) (int, error) {
	if len(plan.Actions) == 0 {
		return 0, nil
	}
	topicKey = strings.TrimSpace(topicKey)
	if topicKey == "" {
		topicKey = fmt.Sprintf("%s.%s", topic.Namespace, topic.Name)
	}
	// 同一业务 key 可以在升级时绑定新物理主题，不能沿用旧 Topic 的 ACL 命中记录。
	signature := topic.ID + ":" + hashActions(plan.Actions)
	if s.bindings != nil && strings.TrimSpace(seedCtx.PluginID) != "" {
		existing, err := s.bindings.GetACL(ctx, seedCtx.TenantUUID, seedCtx.PluginID, topicKey, plan.PrincipalType, plan.PrincipalID)
		if err != nil {
			return 0, err
		}
		if existing != nil && existing.ActionsHash == signature {
			if s.logger != nil {
				s.logger.DebugF(ctx, "[event_fabric.seed] skip acl grant tenant=%s plugin=%s topic=%s principal=%s reason=unchanged",
					seedCtx.TenantUUID, seedCtx.PluginID, topic.FullName(), plan.PrincipalID)
			}
			return 0, nil
		}
	}

	req := acl.GrantRequest{
		TenantUUID:    topic.TenantKey,
		TopicUUID:     topic.ID,
		PrincipalType: plan.PrincipalType,
		PrincipalID:   plan.PrincipalID,
		Actions:       plan.Actions,
		OperatorID:    seedCtx.Operator,
		Justification: fmt.Sprintf("plugin=%s version=%s", seedCtx.PluginID, seedCtx.PluginVersion),
	}
	if _, err := s.acl.Grant(ctx, req); err != nil {
		return 0, err
	}
	s.writeAudit(ctx, "ACL_GRANT", topic.ID, topic.FullName(), seedCtx, map[string]string{
		"principal_type": plan.PrincipalType,
		"principal_id":   plan.PrincipalID,
	})
	if err := s.recordAclBinding(ctx, topicKey, plan, signature, seedCtx); err != nil {
		return 0, fmt.Errorf("record acl binding: %w", err)
	}
	return len(plan.Actions), nil
}

func (s *SeedService) writeAudit(ctx context.Context, action, id, topic string, seedCtx SeedContext, metadata map[string]string) {
	if s.audit == nil {
		return
	}
	meta := map[string]string{
		"plugin_id":      seedCtx.PluginID,
		"plugin_version": seedCtx.PluginVersion,
	}
	for k, v := range metadata {
		meta[k] = v
	}
	record := eventaudit.Record{
		ID:          id,
		TenantID:    seedCtx.TenantUUID,
		Topic:       topic,
		PrincipalID: seedCtx.PluginID,
		Action:      action,
		Status:      "SUCCESS",
		HappenedAt:  s.clock().UTC(),
		Metadata:    meta,
	}
	_ = s.audit.Write(ctx, record)
}

type topicRecord struct {
	ID        string
	TenantKey string
	Namespace string
	Name      string
	FullTopic string
}

func (t topicRecord) FullName() string {
	if strings.TrimSpace(t.FullTopic) != "" {
		return t.FullTopic
	}
	return fmt.Sprintf("%s.%s.%s", t.TenantKey, t.Namespace, t.Name)
}

func topicFromModel(model *eventfabricmodel.TopicDefinition) *topicRecord {
	if model == nil {
		return nil
	}
	return &topicRecord{
		ID:        model.UUID.String(),
		TenantKey: model.TenantKey,
		Namespace: model.Namespace,
		Name:      model.Name,
		FullTopic: model.FullTopic,
	}
}

func topicFromDTO(topic *directory.Topic) *topicRecord {
	if topic == nil {
		return nil
	}
	tenantKey := topic.TenantKey
	if tenantKey == "" {
		tenantKey = topic.TenantUUID
	}
	return &topicRecord{
		ID:        topic.ID,
		TenantKey: tenantKey,
		Namespace: topic.Namespace,
		Name:      topic.Name,
		FullTopic: topic.FullTopic,
	}
}

func isTopicExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}

func hashActions(actions []acl.PrincipalAction) string {
	if len(actions) == 0 {
		return ""
	}
	values := make([]string, len(actions))
	for i, act := range actions {
		values[i] = strings.ToLower(strings.TrimSpace(string(act)))
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

// ensureOwnedTopic 在任何绑定写入之前检查归属。旧主题必须已有同插件的权威绑定；不自动认领公共主题。
func (s *SeedService) ensureOwnedTopic(ctx context.Context, plan TopicPlan, seed SeedContext) (*topicRecord, bool, error) {
	if s.bindings == nil || seed.PluginID == "" || plan.Topic.TenantUUID != seed.TenantUUID {
		return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_OWNER_REQUIRED")
	}
	allowed := s.namespaceAllowed
	if allowed == nil {
		allowed = PluginNamespaceAllowed
	}
	previous, err := s.bindings.GetTopic(ctx, seed.TenantUUID, seed.PluginID, plan.Key)
	if err != nil {
		return nil, false, err
	}
	existing, err := s.dir.FindTopicByFullName(ctx, seed.TenantUUID, plan.Topic.Namespace, plan.Topic.Name)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		owner := directory.PluginOwner(existing)
		if (owner != "" && owner != seed.PluginID) || (existing.ScopeType == eventfabricmodel.TopicScopePlugin && existing.ScopeID != seed.PluginID && existing.ScopeID != seed.TenantUUID) {
			return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_OWNER_CONFLICT: %s", plan.Key)
		}
		if owner == "" && (previous == nil || previous.TopicID != existing.UUID.String() || previous.FullTopic != existing.FullTopic) {
			return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_OWNER_REQUIRED: %s", plan.Key)
		}
		if existing.Lifecycle != eventfabricmodel.TopicLifecycleActive {
			return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_NOT_READY: %s", plan.Key)
		}
		if owners, ok := s.bindings.(interface {
			TopicOwners(context.Context, string, string) ([]string, error)
		}); ok {
			plugins, err := owners.TopicOwners(ctx, seed.TenantUUID, existing.UUID.String())
			if err != nil {
				return nil, false, err
			}
			for _, plugin := range plugins {
				if plugin != seed.PluginID {
					return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_OWNER_CONFLICT: %s", plan.Key)
				}
			}
		}
		if marker, ok := s.dir.(interface {
			MarkPluginTopic(context.Context, *eventfabricmodel.TopicDefinition, string, string) error
		}); ok {
			if err := marker.MarkPluginTopic(ctx, existing, seed.PluginID, plan.Key); err != nil {
				return nil, false, err
			}
		}
		return topicFromModel(existing), false, nil
	}
	if !allowed(seed.PluginID, plan.Topic.Namespace) {
		return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_NAMESPACE_DENIED: %s", plan.Topic.Namespace)
	}
	if previous != nil && previous.FullTopic == plan.FullTopic {
		return nil, false, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_MISSING: %s", plan.Key)
	}
	input := plan.Topic
	input.PluginOwner = seed.PluginID
	input.PluginTopicKey = plan.Key
	record, created, err := s.ensureTopic(ctx, input)
	if err != nil {
		return nil, false, err
	}
	// 并发创建命中已有记录时重新检查归属，不把冲突当成授权依据。
	if !created {
		return s.ensureOwnedTopic(ctx, plan, seed)
	}
	return record, true, nil
}

// PluginNamespaceAllowed 是未配置额外归属时的默认规则，允许插件名及其常用前缀。
func PluginNamespaceAllowed(pluginID, namespace string) bool {
	alias := strings.TrimPrefix(pluginID, "com.powerx.plugins.")
	if alias == pluginID || alias == "" || strings.Contains(alias, ".") {
		return false
	}
	alias = strings.ReplaceAll(alias, "-", "_")
	for _, root := range []string{alias, "powerx." + alias, "_topic." + alias, pluginID} {
		if namespace == root || strings.HasPrefix(namespace, root+".") {
			return true
		}
	}
	return false
}

func (s *SeedService) verifyExistingACL(ctx context.Context, topic *topicRecord, plan ACLPlan) error {
	reader, ok := s.acl.(interface {
		HasPermission(context.Context, string, uuid.UUID, string, string, time.Time) (bool, error)
	})
	if !ok {
		return fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_VERIFIER_REQUIRED")
	}
	for _, action := range plan.Actions {
		granted, err := reader.HasPermission(ctx, topic.TenantKey, uuid.MustParse(topic.ID), plan.PrincipalID, string(action), s.clock())
		if err != nil {
			return err
		}
		if !granted {
			return fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_REQUIRED: %s", topic.FullName())
		}
	}
	return nil
}
