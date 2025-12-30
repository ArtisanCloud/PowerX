package manifest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

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
}

// SeedService 根据 manifest 计划自动创建 Topic 与 ACL。
type SeedService struct {
	dir        directoryClient
	acl        aclClient
	audit      eventaudit.Service
	logger     *pxlog.Logger
	clock      func() time.Time
	maxRetries int
	bindings   BindingStore
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
		dir:        opts.Directory,
		acl:        opts.ACL,
		audit:      opts.Audit,
		logger:     opts.Logger,
		clock:      opts.Clock,
		maxRetries: opts.MaxRetries,
		bindings:   opts.Bindings,
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
	if plan == nil {
		return nil, fmt.Errorf("seed plan is nil")
	}
	if s.dir == nil || s.acl == nil {
		return nil, fmt.Errorf("seed service not fully configured")
	}
	result := &SeedResult{Topics: make([]TopicResult, 0, len(plan.Topics))}
	var combinedErr error
	for _, topicPlan := range plan.Topics {
		topicRes := TopicResult{
			Key:       topicPlan.Key,
			FullTopic: topicPlan.FullTopic,
		}
		record, created, err := s.ensureTopic(ctx, topicPlan.Topic)
		if err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("topic %s: %w", topicPlan.FullTopic, err))
			result.Topics = append(result.Topics, topicRes)
			continue
		}
		topicRes.TopicID = record.ID
		topicRes.Created = created
		if created {
			s.writeAudit(ctx, "TOPIC_CREATE", record.ID, record.FullName(), seedCtx, nil)
		}
		s.recordTopicBinding(ctx, topicPlan, record, seedCtx)

		grantCount := 0
		for _, aclPlan := range topicPlan.ACL {
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
	if combinedErr != nil {
		return result, combinedErr
	}
	return result, nil
}

func (s *SeedService) recordTopicBinding(ctx context.Context, topicPlan TopicPlan, record *topicRecord, seedCtx SeedContext) {
	if s.bindings == nil || record == nil || strings.TrimSpace(seedCtx.PluginID) == "" {
		return
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
	if err != nil && s.logger != nil {
		s.logger.WarnF(ctx, "[event_fabric.seed] record topic binding failed tenant=%s plugin=%s topic=%s err=%v",
			seedCtx.TenantUUID, seedCtx.PluginID, record.FullName(), err)
	}
}

func (s *SeedService) recordAclBinding(ctx context.Context, topicKey string, plan ACLPlan, signature string, seedCtx SeedContext) {
	if s.bindings == nil || strings.TrimSpace(seedCtx.PluginID) == "" {
		return
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
	if err != nil && s.logger != nil {
		s.logger.WarnF(ctx, "[event_fabric.seed] record acl binding failed tenant=%s plugin=%s topic_key=%s principal=%s err=%v",
			seedCtx.TenantUUID, seedCtx.PluginID, topicKey, plan.PrincipalID, err)
	}
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

func (s *SeedService) applyACL(ctx context.Context, topicKey string, topic *topicRecord, plan ACLPlan, seedCtx SeedContext) (int, error) {
	if len(plan.Actions) == 0 {
		return 0, nil
	}
	topicKey = strings.TrimSpace(topicKey)
	if topicKey == "" {
		topicKey = fmt.Sprintf("%s.%s", topic.Namespace, topic.Name)
	}
	signature := hashActions(plan.Actions)
	if s.bindings != nil && strings.TrimSpace(seedCtx.PluginID) != "" {
		existing, err := s.bindings.GetACL(ctx, seedCtx.TenantUUID, seedCtx.PluginID, topicKey, plan.PrincipalType, plan.PrincipalID)
		if err == nil && existing != nil && existing.ActionsHash == signature {
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
	s.recordAclBinding(ctx, topicKey, plan, signature, seedCtx)
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
