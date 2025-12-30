package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	gatewayv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/integration_gateway/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// IntegrationGatewayServer 实现 specs/007 定义的 gRPC IntegrationGatewayService。
type IntegrationGatewayServer struct {
	gatewayv1.UnimplementedIntegrationGatewayServiceServer
	catalog  *capservice.RegistryService
	invoker  *capservice.InvocationService
	selector *capservice.Selector
}

// NewIntegrationGatewayServer 构建服务实例。
func NewIntegrationGatewayServer(deps *shared.Deps) *IntegrationGatewayServer {
	if deps == nil || deps.CapabilityCatalogSvc == nil {
		return nil
	}
	invocationSvc := deps.CapabilityInvocationSvc
	if invocationSvc == nil {
		var traceRepo *repo.InvocationTraceRepository
		var eventRepo *repo.CapabilityEventPublicationRepository
		if deps.DB != nil {
			traceRepo = repo.NewInvocationTraceRepository(deps.DB)
			eventRepo = repo.NewCapabilityEventPublicationRepository(deps.DB)
		}
		invocationSvc = capservice.NewInvocationService(capservice.InvocationServiceOptions{
			Catalog:     deps.CapabilityCatalogSvc,
			Router:      deps.RouterSvc,
			TraceRepo:   traceRepo,
			EventRepo:   eventRepo,
			EventBus:    deps.EventBus,
			Auditor:     deps.Auditor,
			VersionLock: deps.VersionLockStore,
		})
	}
	selector := deps.CapabilitySelector
	if selector == nil && invocationSvc != nil {
		selector = capservice.NewSelector(capservice.SelectorOptions{
			Invoker:  invocationSvc,
			EventBus: deps.EventBus,
		})
	}
	return &IntegrationGatewayServer{
		catalog:  deps.CapabilityCatalogSvc,
		invoker:  invocationSvc,
		selector: selector,
	}
}

// RegisterIntegrationGatewayServer 注册 gRPC 服务。
func RegisterIntegrationGatewayServer(server *grpc.Server, deps *shared.Deps) {
	if server == nil {
		return
	}
	srv := NewIntegrationGatewayServer(deps)
	if srv == nil {
		return
	}
	gatewayv1.RegisterIntegrationGatewayServiceServer(server, srv)
}

func (s *IntegrationGatewayServer) ListCapabilities(ctx context.Context, req *gatewayv1.ListCapabilitiesRequest) (*gatewayv1.ListCapabilitiesResponse, error) {
	if s.catalog == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrUnavailable, nil)
	}
	page := int(req.GetPage())
	if page <= 0 {
		page = 1
	}
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 {
		pageSize = 50
	}

	opts := capservice.CapabilityListOptions{
		PluginID:     strings.TrimSpace(req.GetPluginId()),
		Intent:       strings.TrimSpace(req.GetIntent()),
		Protocols:    req.GetProtocols(),
		Limit:        pageSize,
		Offset:       (page - 1) * pageSize,
		Status:       []string{"published"},
		IncludeTotal: true,
	}
	if req.GetIncludeDisabled() {
		opts.Status = nil
	}
	if source := strings.TrimSpace(req.GetSource()); source != "" {
		normalized, err := capservice.NormalizeCapabilitySource(source)
		if err != nil {
			return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("source must be corex or plugin"), err)
		}
		opts.Source = normalized
	}

	views, total, err := s.catalog.ListCapabilities(ctx, opts)
	if err != nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInternal, err)
	}

	items := make([]*gatewayv1.Capability, 0, len(views))
	for _, view := range views {
		capProto, convErr := capabilityViewToProto(view, false)
		if convErr != nil {
			return nil, status.Errorf(codes.Internal, "convert capability failed: %v", convErr)
		}
		items = append(items, capProto)
	}

	return &gatewayv1.ListCapabilitiesResponse{
		Items:    items,
		Page:     int32(page),
		PageSize: int32(pageSize),
		Total:    int32(total),
	}, nil
}

func (s *IntegrationGatewayServer) GetCapability(ctx context.Context, req *gatewayv1.GetCapabilityRequest) (*gatewayv1.Capability, error) {
	if s.catalog == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrUnavailable, nil)
	}
	if strings.TrimSpace(req.GetCapabilityId()) == "" {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("capability_id is required"), nil)
	}
	view, err := s.catalog.GetCapability(ctx, strings.TrimSpace(req.GetCapabilityId()), req.GetIncludeWorkflows())
	if err != nil {
		template := capability_registrydto.ErrInternal
		if errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = capability_registrydto.ErrNotFound
		}
		return nil, capability_registrydto.ToGRPCError(template, err)
	}
	capProto, err := capabilityViewToProto(view, req.GetIncludeWorkflows())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert capability failed: %v", err)
	}
	return capProto, nil
}

func (s *IntegrationGatewayServer) InvokeCapability(ctx context.Context, req *gatewayv1.InvokeCapabilityRequest) (*gatewayv1.InvokeCapabilityResponse, error) {
	if s.selector == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrUnavailable, nil)
	}
	if strings.TrimSpace(req.GetCapabilityId()) == "" || strings.TrimSpace(req.GetTenantUuid()) == "" {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("capability_id and tenant_uuid are required"), nil)
	}

	payload := map[string]interface{}{}
	if req.GetPayload() != nil {
		payload = req.GetPayload().AsMap()
	}
	contextMap := cloneInvokeContext(req.GetContext())

	result, err := s.selector.Invoke(ctx, capservice.CapabilityInvokeRequest{
		CapabilityID:      strings.TrimSpace(req.GetCapabilityId()),
		TenantUUID:        strings.TrimSpace(req.GetTenantUuid()),
		PreferredProtocol: strings.TrimSpace(req.GetPreferredProtocol()),
		IdempotencyKey:    strings.TrimSpace(req.GetIdempotencyKey()),
		TraceID:           strings.TrimSpace(req.GetTraceId()),
		Payload:           payload,
		Context:           contextMap,
	})
	if err != nil {
		template := selectInvokeErrorTemplate(err)
		return nil, capability_registrydto.ToGRPCError(template, err)
	}

	resp := &gatewayv1.InvokeCapabilityResponse{
		TraceId:      result.TraceID,
		Status:       result.Status,
		ProtocolUsed: result.ProtocolUsed,
		FallbackUsed: result.FallbackUsed,
	}
	if len(result.Result) > 0 {
		if structVal, convErr := structpb.NewStruct(result.Result); convErr == nil {
			resp.Result = structVal
		}
	}
	return resp, nil
}

func (s *IntegrationGatewayServer) ListWorkflowTemplates(ctx context.Context, req *gatewayv1.ListWorkflowTemplatesRequest) (*gatewayv1.ListWorkflowTemplatesResponse, error) {
	if s.catalog == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrUnavailable, nil)
	}
	if strings.TrimSpace(req.GetCapabilityId()) == "" {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("capability_id is required"), nil)
	}
	view, err := s.catalog.GetCapability(ctx, strings.TrimSpace(req.GetCapabilityId()), true)
	if err != nil {
		template := capability_registrydto.ErrInternal
		if errors.Is(err, repo.ErrCapabilityRecordNotFound) {
			template = capability_registrydto.ErrNotFound
		}
		return nil, capability_registrydto.ToGRPCError(template, err)
	}
	dto := capability_registrydto.CapabilityViewToDTO(view, true)
	items := make([]*gatewayv1.WorkflowTemplate, 0, len(dto.WorkflowTemplates))
	for _, tpl := range dto.WorkflowTemplates {
		items = append(items, workflowTemplateDTOToProto(tpl))
	}
	return &gatewayv1.ListWorkflowTemplatesResponse{Items: items}, nil
}

func (s *IntegrationGatewayServer) StreamInvocation(*gatewayv1.StreamInvocationRequest, gatewayv1.IntegrationGatewayService_StreamInvocationServer) error {
	return status.Error(codes.Unimplemented, "stream invocation is not implemented yet")
}

func (s *IntegrationGatewayServer) ListCapabilitySyncJobs(ctx context.Context, req *gatewayv1.ListCapabilitySyncJobsRequest) (*gatewayv1.ListCapabilitySyncJobsResponse, error) {
	if s.catalog == nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrUnavailable, nil)
	}
	filter := capservice.CapabilitySyncJobListOptions{
		PluginID: strings.TrimSpace(req.GetPluginId()),
	}
	if statusFilter := strings.TrimSpace(req.GetStatus()); statusFilter != "" {
		filter.Status = []string{statusFilter}
	}
	jobs, err := s.catalog.ListSyncJobs(ctx, filter)
	if err != nil {
		return nil, capability_registrydto.ToGRPCError(capability_registrydto.ErrInternal, err)
	}
	items := make([]*gatewayv1.CapabilitySyncJob, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, capabilitySyncJobToProto(capability_registrydto.SyncJobToDTO(job)))
	}
	return &gatewayv1.ListCapabilitySyncJobsResponse{Items: items}, nil
}

func capabilityViewToProto(view capservice.CapabilityRecordView, includeWorkflows bool) (*gatewayv1.Capability, error) {
	dto := capability_registrydto.CapabilityViewToDTO(view, includeWorkflows)
	protoCap := &gatewayv1.Capability{
		CapabilityId:     dto.CapabilityID,
		PluginId:         dto.PluginID,
		Source:           dto.Source,
		Title:            dto.Title,
		Description:      dto.Description,
		Intents:          dto.Intents,
		ToolScope:        dto.ToolScope,
		CapabilitiesHash: dto.CapabilitiesHash,
		ProtocolHash:     dto.ProtocolHash,
		Status:           dto.Status,
	}
	if dto.Policy != nil {
		protoCap.Policy = &gatewayv1.Policy{
			Prefer:               dto.Policy.Prefer,
			Fallback:             dto.Policy.Fallback,
			RollbackCapabilityId: dto.Policy.RollbackCapability,
		}
	}
	protoCap.Protocols = make([]*gatewayv1.ProtocolBinding, 0, len(dto.Protocols))
	for _, binding := range dto.Protocols {
		protoBinding := &gatewayv1.ProtocolBinding{
			Channel:     binding.Channel,
			Endpoint:    binding.Endpoint,
			SchemaRef:   binding.SchemaRef,
			Method:      binding.Method,
			Rpc:         binding.RPC,
			ToolRef:     binding.ToolRef,
			AuthType:    binding.AuthType,
			HealthState: binding.HealthState,
		}
		if binding.LastCheckedAt != nil {
			if ts, err := time.Parse(time.RFC3339, *binding.LastCheckedAt); err == nil {
				protoBinding.LastCheckedAt = timestamppb.New(ts)
			}
		}
		protoCap.Protocols = append(protoCap.Protocols, protoBinding)
	}
	if includeWorkflows {
		protoCap.WorkflowTemplates = make([]*gatewayv1.WorkflowTemplate, 0, len(dto.WorkflowTemplates))
		for _, tpl := range dto.WorkflowTemplates {
			protoCap.WorkflowTemplates = append(protoCap.WorkflowTemplates, workflowTemplateDTOToProto(tpl))
		}
	}
	return protoCap, nil
}

func workflowTemplateDTOToProto(dto capability_registrydto.WorkflowTemplateDTO) *gatewayv1.WorkflowTemplate {
	protoTpl := &gatewayv1.WorkflowTemplate{
		TemplateId:               dto.TemplateID,
		Name:                     dto.Name,
		Description:              dto.Description,
		RequiresManualUpgrade:    dto.RequiresManualUpgrade,
		CapabilitiesHashSnapshot: dto.CapabilitiesHashSnapshot,
	}
	if dto.ParamsSchema != nil {
		if structVal, err := toStruct(dto.ParamsSchema); err == nil {
			protoTpl.ParamsSchema = structVal
		}
	}
	protoTpl.Steps = convertWorkflowSteps(dto.Steps)
	return protoTpl
}

func convertWorkflowSteps(raw interface{}) []*gatewayv1.WorkflowStep {
	values, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	steps := make([]*gatewayv1.WorkflowStep, 0, len(values))
	for _, item := range values {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		step := &gatewayv1.WorkflowStep{}
		if id, ok := obj["capability_id"].(string); ok {
			step.CapabilityId = id
		}
		if protocol, ok := obj["protocol"].(string); ok {
			step.Protocol = protocol
		}
		if params, ok := obj["params"]; ok {
			if structVal, err := toStruct(params); err == nil {
				step.Params = structVal
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func capabilitySyncJobToProto(job capability_registrydto.CapabilitySyncJobDTO) *gatewayv1.CapabilitySyncJob {
	pbJob := &gatewayv1.CapabilitySyncJob{
		JobId:         job.JobID,
		PluginId:      job.PluginID,
		PluginVersion: job.PluginVersion,
		Status:        job.Status,
		HashBefore:    job.HashBefore,
		HashAfter:     job.HashAfter,
		ErrorSummary:  job.ErrorSummary,
	}
	if job.StartedAt != nil {
		if ts, err := time.Parse(time.RFC3339, *job.StartedAt); err == nil {
			pbJob.StartedAt = timestamppb.New(ts)
		}
	}
	if job.FinishedAt != nil {
		if ts, err := time.Parse(time.RFC3339, *job.FinishedAt); err == nil {
			pbJob.FinishedAt = timestamppb.New(ts)
		}
	}
	return pbJob
}

func toStruct(value interface{}) (*structpb.Struct, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return structpb.NewStruct(typed)
	case []byte:
		var out map[string]interface{}
		if err := json.Unmarshal(typed, &out); err != nil {
			return nil, err
		}
		return structpb.NewStruct(out)
	default:
		bytes, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var out map[string]interface{}
		if err := json.Unmarshal(bytes, &out); err != nil {
			return nil, err
		}
		return structpb.NewStruct(out)
	}
}

func selectInvokeErrorTemplate(err error) capability_registrydto.ErrorTemplate {
	switch {
	case errors.Is(err, capservice.ErrManualUpgradeRequired):
		return capability_registrydto.ErrVersionLocked
	case errors.Is(err, capservice.ErrSelectorCapabilityRequired):
		return capability_registrydto.ErrNotFound.WithHint("capability not found or not published for tenant")
	case errors.Is(err, capservice.ErrSelectorCapabilityForbidden):
		return capability_registrydto.ErrCapabilityForbidden
	case errors.Is(err, capservice.ErrSelectorTenantRequired):
		return capability_registrydto.ErrTenantUUIDMissing
	case errors.Is(err, capservice.ErrSelectorSafeModeActive):
		return capability_registrydto.ErrSafeModeActive
	case errors.Is(err, capservice.ErrSelectorToolGrantRequired):
		return capability_registrydto.ErrToolGrantMissing
	case errors.Is(err, capservice.ErrSelectorFeatureFlagMissing):
		return capability_registrydto.ErrFeatureFlagMissing
	case errors.Is(err, capservice.ErrSelectorUnavailable):
		return capability_registrydto.ErrUnavailable
	default:
		return capability_registrydto.ErrInvokeFailed
	}
}

func cloneInvokeContext(src map[string]string) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
