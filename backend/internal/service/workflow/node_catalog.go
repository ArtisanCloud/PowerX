package workflow

import (
	"context"
	"errors"
	"sort"
)

var ErrNodeCatalogUnavailable = errors.New("workflow.node_catalog_unavailable")

type WorkflowNodeAdapterDeps struct {
	SkillInvoker       SkillInvoker
	CapabilityInvoker  CapabilityInvoker
	MetadataClassifier MetadataClassifier
	KnowledgeOperator  KnowledgeOperator
	EventPublisher     WorkflowEventPublisher
	HumanReviewStore   HumanReviewStore
}

type NodeCatalogItem struct {
	NodeKind              string         `json:"node_kind"`
	DisplayNameI18nKey    string         `json:"display_name_i18n_key"`
	DescriptionI18nKey    string         `json:"description_i18n_key,omitempty"`
	Category              string         `json:"category"`
	StepType              string         `json:"step_type"`
	InputSchema           NodeSchema     `json:"input_schema,omitempty"`
	OutputSchema          NodeSchema     `json:"output_schema,omitempty"`
	ConfigSchema          NodeSchema     `json:"config_schema,omitempty"`
	RequiredPermissions   []string       `json:"required_permissions,omitempty"`
	RequiredCapabilities  []string       `json:"required_capabilities,omitempty"`
	IdempotencyRequired   bool           `json:"idempotency_required,omitempty"`
	CompensationSupported bool           `json:"compensation_supported,omitempty"`
	SourceStatus          string         `json:"source_status"`
	Metadata              map[string]any `json:"metadata,omitempty"`
}

type NodeCatalogEnrichment struct {
	NodeKind              string
	RequiredPermissions   []string
	RequiredCapabilities  []string
	IdempotencyRequired   *bool
	CompensationSupported *bool
	SourceStatus          string
	Metadata              map[string]any
}

type NodeCatalogProvider interface {
	ListNodeCatalogEnrichments(ctx context.Context) ([]NodeCatalogEnrichment, error)
}

type NodeCatalogService struct {
	registry  *NodeAdapterRegistry
	providers []NodeCatalogProvider
}

func NewNodeCatalogService(registry *NodeAdapterRegistry, providers ...NodeCatalogProvider) *NodeCatalogService {
	return &NodeCatalogService{
		registry:  registry,
		providers: providers,
	}
}

func RegisterWorkflowNodeAdapters(registry *NodeAdapterRegistry, deps WorkflowNodeAdapterDeps) error {
	if registry == nil {
		return ErrNodeAdapterNil
	}
	for _, adapter := range []NodeAdapter{
		NewInputCaptureAdapter(),
		NewSkillAdapter(deps.SkillInvoker),
		NewCapabilityAdapter(deps.CapabilityInvoker),
		NewMetadataAdapter(deps.MetadataClassifier),
		NewKnowledgeStageAdapter(deps.KnowledgeOperator),
		NewKnowledgePublishAdapter(deps.KnowledgeOperator),
		NewDecisionGatewayAdapter(),
		NewParallelFanoutAdapter(),
		NewParallelJoinAdapter(),
		NewHumanReviewAdapter(deps.HumanReviewStore),
		NewEventAdapter(deps.EventPublisher),
		NewCompensationRollbackAdapter(),
	} {
		if err := registry.Register(adapter); err != nil {
			return err
		}
	}
	return nil
}

func (s *NodeCatalogService) List(ctx context.Context) ([]NodeCatalogItem, error) {
	if s == nil || s.registry == nil {
		return nil, ErrNodeCatalogUnavailable
	}
	items := make([]NodeCatalogItem, 0)
	for _, spec := range s.registry.List() {
		items = append(items, itemFromAdapterSpec(spec))
	}
	enrichments, err := s.loadEnrichments(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if enrichment, ok := enrichments[items[i].NodeKind]; ok {
			applyNodeCatalogEnrichment(&items[i], enrichment)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category == items[j].Category {
			return items[i].NodeKind < items[j].NodeKind
		}
		return items[i].Category < items[j].Category
	})
	return items, nil
}

func (s *NodeCatalogService) Get(ctx context.Context, nodeKind string) (NodeCatalogItem, error) {
	if s == nil || s.registry == nil {
		return NodeCatalogItem{}, ErrNodeCatalogUnavailable
	}
	adapter, err := s.registry.Adapter(nodeKind)
	if err != nil {
		return NodeCatalogItem{}, err
	}
	item := itemFromAdapterSpec(adapter.Spec())
	enrichments, err := s.loadEnrichments(ctx)
	if err != nil {
		return NodeCatalogItem{}, err
	}
	if enrichment, ok := enrichments[item.NodeKind]; ok {
		applyNodeCatalogEnrichment(&item, enrichment)
	}
	return item, nil
}

func (s *Service) ListNodeCatalog(ctx context.Context) ([]NodeCatalogItem, error) {
	if s == nil || s.nodeCatalog == nil {
		return nil, ErrNodeCatalogUnavailable
	}
	return s.nodeCatalog.List(ctx)
}

func (s *Service) GetNodeCatalogItem(ctx context.Context, nodeKind string) (NodeCatalogItem, error) {
	if s == nil || s.nodeCatalog == nil {
		return NodeCatalogItem{}, ErrNodeCatalogUnavailable
	}
	return s.nodeCatalog.Get(ctx, nodeKind)
}

func (s *NodeCatalogService) loadEnrichments(ctx context.Context) (map[string]NodeCatalogEnrichment, error) {
	out := map[string]NodeCatalogEnrichment{}
	for _, provider := range s.providers {
		if provider == nil {
			continue
		}
		items, err := provider.ListNodeCatalogEnrichments(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			nodeKind := normalizeNodeKind(item.NodeKind)
			if nodeKind == "" {
				continue
			}
			item.NodeKind = nodeKind
			out[nodeKind] = mergeNodeCatalogEnrichment(out[nodeKind], item)
		}
	}
	return out, nil
}

func itemFromAdapterSpec(spec NodeAdapterSpec) NodeCatalogItem {
	nodeKind := normalizeNodeKind(spec.NodeKind)
	return NodeCatalogItem{
		NodeKind:           nodeKind,
		DisplayNameI18nKey: spec.DisplayName,
		Category:           spec.Category,
		StepType:           stepTypeForNodeKind(nodeKind),
		InputSchema:        spec.InputSchema,
		OutputSchema:       spec.OutputSchema,
		ConfigSchema:       spec.InputSchema,
		SourceStatus:       "available",
	}
}

func applyNodeCatalogEnrichment(item *NodeCatalogItem, enrichment NodeCatalogEnrichment) {
	item.RequiredPermissions = normalizeStrings(append(item.RequiredPermissions, enrichment.RequiredPermissions...))
	item.RequiredCapabilities = normalizeStrings(append(item.RequiredCapabilities, enrichment.RequiredCapabilities...))
	if enrichment.IdempotencyRequired != nil {
		item.IdempotencyRequired = *enrichment.IdempotencyRequired
	}
	if enrichment.CompensationSupported != nil {
		item.CompensationSupported = *enrichment.CompensationSupported
	}
	if enrichment.SourceStatus != "" {
		item.SourceStatus = enrichment.SourceStatus
	}
	if len(enrichment.Metadata) > 0 {
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		for k, v := range enrichment.Metadata {
			item.Metadata[k] = v
		}
	}
}

func mergeNodeCatalogEnrichment(current NodeCatalogEnrichment, next NodeCatalogEnrichment) NodeCatalogEnrichment {
	current.NodeKind = normalizeNodeKind(next.NodeKind)
	current.RequiredPermissions = normalizeStrings(append(current.RequiredPermissions, next.RequiredPermissions...))
	current.RequiredCapabilities = normalizeStrings(append(current.RequiredCapabilities, next.RequiredCapabilities...))
	if next.IdempotencyRequired != nil {
		current.IdempotencyRequired = next.IdempotencyRequired
	}
	if next.CompensationSupported != nil {
		current.CompensationSupported = next.CompensationSupported
	}
	if next.SourceStatus != "" {
		current.SourceStatus = next.SourceStatus
	}
	if len(next.Metadata) > 0 {
		if current.Metadata == nil {
			current.Metadata = map[string]any{}
		}
		for k, v := range next.Metadata {
			current.Metadata[k] = v
		}
	}
	return current
}

func stepTypeForNodeKind(nodeKind string) string {
	switch normalizeNodeKind(nodeKind) {
	case "input.capture":
		return "system"
	case "skill.invoke", "capability.invoke", "metadata.classify", "knowledge.stage", "knowledge.publish", "event.emit":
		return "system"
	case "human.review":
		return "human_approval"
	case "decision.gateway":
		return "decision"
	case "parallel.fanout", "parallel.join":
		return "parallel"
	case "compensation.rollback":
		return "compensation"
	default:
		return "system"
	}
}
