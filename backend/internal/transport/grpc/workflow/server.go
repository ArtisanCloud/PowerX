package workflow

import (
	"context"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server 提供 WorkflowService 的 gRPC 桥接。
type Server struct {
	workflowv1.UnimplementedWorkflowServiceServer

	svc *workflowsvc.Service
}

// NewServer 构建 gRPC server 实例。
func NewServer(svc *workflowsvc.Service) *Server {
	return &Server{svc: svc}
}

func (s *Server) CreateDefinition(ctx context.Context, req *workflowv1.CreateDefinitionRequest) (*workflowv1.CreateDefinitionResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetName()) == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	input := workflowsvc.CreateDefinitionInput{
		TenantUUID:         tenantUUID,
		Name:               strings.TrimSpace(req.GetName()),
		Description:        strings.TrimSpace(req.GetDescription()),
		CreatedBy:          memberUUIDFromContext(req.GetCtx()),
		Steps:              pbStepsToInternal(req.GetSteps()),
		DefaultRetryPolicy: retryPolicyToMap(req.GetDefaultRetryPolicy()),
		CompensationPolicy: compensationPolicyToMap(req.GetCompensationPolicy()),
		SlaPolicy:          slaPolicyToMap(req.GetSlaPolicy()),
		Metadata:           structToMap(req.GetMetadata()),
	}

	definition, err := s.svc.CreateDefinition(ctx, input)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &workflowv1.CreateDefinitionResponse{
		Meta:       okMeta(ctx),
		Definition: modelDefinitionToPB(definition, tenantUUID),
	}, nil
}

func (s *Server) PublishDefinition(ctx context.Context, req *workflowv1.PublishDefinitionRequest) (*workflowv1.PublishDefinitionResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	defUUID, err := uuid.Parse(req.GetDefinitionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid definition_id")
	}

	result, err := s.svc.PublishDefinition(ctx, workflowsvc.PublishDefinitionInput{
		TenantUUID:     tenantUUID,
		DefinitionUUID: defUUID,
		PublishedBy:    memberUUIDFromContext(req.GetCtx()),
		ChangeNote:     req.GetChangeNote(),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &workflowv1.PublishDefinitionResponse{
		Meta:       okMeta(ctx),
		Definition: modelDefinitionToPB(result, tenantUUID),
	}, nil
}

func (s *Server) ArchiveDefinition(ctx context.Context, req *workflowv1.ArchiveDefinitionRequest) (*workflowv1.ArchiveDefinitionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ArchiveDefinition not implemented")
}

func (s *Server) ListDefinitions(ctx context.Context, req *workflowv1.ListDefinitionsRequest) (*workflowv1.ListDefinitionsResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}

	statusFilter := []string{}
	if req.GetStatus() != workflowv1.WorkflowDefinitionStatus_WORKFLOW_DEFINITION_STATUS_UNSPECIFIED {
		statusFilter = append(statusFilter, strings.ToLower(req.GetStatus().String()))
	}

	limit := 20
	offset := 0
	if req.GetPage() != nil {
		if req.GetPage().GetPageSize() > 0 {
			limit = int(req.GetPage().GetPageSize())
		}
		if req.GetPage().GetOffset() > 0 {
			offset = int(req.GetPage().GetOffset())
		}
	}

	defs, total, err := s.svc.ListDefinitions(ctx, tenantUUID, statusFilter, req.GetKeyword(), limit, offset)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	items := make([]*workflowv1.WorkflowDefinition, 0, len(defs))
	for i := range defs {
		items = append(items, modelDefinitionToPB(&defs[i], tenantUUID))
	}

	return &workflowv1.ListDefinitionsResponse{
		Meta:        okMeta(ctx),
		Definitions: items,
		Page: &commonv1.PageResponse{
			Total: total,
		},
	}, nil
}

func (s *Server) GetDefinition(ctx context.Context, req *workflowv1.GetDefinitionRequest) (*workflowv1.GetDefinitionResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	defUUID, err := uuid.Parse(req.GetDefinitionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid definition_id")
	}
	var versionPtr *int32
	if req.GetVersion() > 0 {
		version := req.GetVersion()
		versionPtr = &version
	}

	definition, err := s.svc.GetDefinition(ctx, tenantUUID, defUUID, versionPtr)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &workflowv1.GetDefinitionResponse{
		Meta:       okMeta(ctx),
		Definition: modelDefinitionToPB(definition, tenantUUID),
	}, nil
}

func (s *Server) StartInstance(ctx context.Context, req *workflowv1.StartInstanceRequest) (*workflowv1.StartInstanceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	defUUID, err := uuid.Parse(req.GetDefinitionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid definition_id")
	}

	instance, err := s.svc.StartInstance(ctx, workflowsvc.StartInstanceInput{
		TenantUUID:        tenantUUID,
		DefinitionUUID:    defUUID,
		DefinitionVersion: req.GetDefinitionVersion(),
		Input:             structToMap(req.GetInput()),
		Tags:              structToStringMap(req.GetTags()),
		CorrelationID:     req.GetCorrelationId(),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &workflowv1.StartInstanceResponse{
		Meta:     okMeta(ctx),
		Instance: modelInstanceToPB(instance, nil, tenantUUID),
	}, nil
}

func (s *Server) GetInstance(ctx context.Context, req *workflowv1.GetInstanceRequest) (*workflowv1.GetInstanceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	instUUID, err := uuid.Parse(req.GetInstanceId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid instance_id")
	}

	inst, steps, err := s.svc.GetInstance(ctx, tenantUUID, instUUID, req.GetIncludeSteps())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	return &workflowv1.GetInstanceResponse{
		Meta:     okMeta(ctx),
		Instance: modelInstanceToPB(inst, steps, tenantUUID),
	}, nil
}

func (s *Server) ListInstances(ctx context.Context, req *workflowv1.ListInstancesRequest) (*workflowv1.ListInstancesResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}

	filter := workflowrepo.InstanceListFilter{
		TenantUUID: tenantUUID,
		PageSize:   20,
		Page:       1,
	}
	if pageReq := req.GetPage(); pageReq != nil {
		if pageReq.GetPageSize() > 0 {
			filter.PageSize = int(pageReq.GetPageSize())
		}
		if pageReq.GetOffset() > 0 {
			size := filter.PageSize
			if size <= 0 {
				size = 20
			}
			filter.Page = int(pageReq.GetOffset())/size + 1
		}
	}
	if req.GetDefinitionId() != "" {
		if defUUID, err := uuid.Parse(req.GetDefinitionId()); err == nil {
			filter.DefinitionUUID = defUUID
		}
	}
	if req.GetState() != workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_UNSPECIFIED {
		filter.State = strings.ToLower(req.GetState().String())
	}

	instances, total, err := s.svc.ListInstances(ctx, filter)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	items := make([]*workflowv1.WorkflowInstance, 0, len(instances))
	for i := range instances {
		if req.GetIncludeSteps() {
			inst, steps, err := s.svc.GetInstance(ctx, tenantUUID, instances[i].UUID, true)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			items = append(items, modelInstanceToPB(inst, steps, tenantUUID))
		} else {
			items = append(items, modelInstanceToPB(&instances[i], nil, tenantUUID))
		}
	}

	return &workflowv1.ListInstancesResponse{
		Meta:      okMeta(ctx),
		Instances: items,
		Page: &commonv1.PageResponse{
			Total: total,
		},
	}, nil
}

func (s *Server) ControlInstance(ctx context.Context, req *workflowv1.ControlInstanceRequest) (*workflowv1.ControlInstanceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}
	instUUID, err := uuid.Parse(req.GetInstanceId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid instance_id")
	}

	action := controlActionString(req.GetAction())
	assignmentID := parseUint64(req.GetAssignmentId())
	operator := memberUUIDFromContext(req.GetCtx())

	_, err = s.svc.ControlInstance(ctx, workflowsvc.ControlInstanceInput{
		TenantUUID:   tenantUUID,
		InstanceUUID: instUUID,
		Action:       action,
		Operator:     operator,
		StepID:       req.GetStepId(),
		AssignmentID: assignmentID,
		Reason:       req.GetReason(),
		Payload:      structToMap(req.GetPayload()),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	instDetail, steps, err := s.svc.GetInstance(ctx, tenantUUID, instUUID, true)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return &workflowv1.ControlInstanceResponse{
		Meta:     okMeta(ctx),
		Instance: modelInstanceToPB(instDetail, steps, tenantUUID),
	}, nil
}

func (s *Server) ExportInstances(ctx context.Context, req *workflowv1.ExportInstancesRequest) (*workflowv1.ExportInstancesResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Internal, "workflow service unavailable")
	}
	tenantUUID, err := s.requireTenantContext(ctx, req.GetCtx())
	if err != nil {
		return nil, err
	}

	filter := workflowsvc.ExportFilter{
		TenantUUID:         tenantUUID,
		IncludeStepDetails: req.GetIncludeStepDetails(),
		Format:             protoFormatToInternal(req.GetFormat()),
	}

	if req.GetDefinitionId() != "" {
		definitionUUID, err := uuid.Parse(req.GetDefinitionId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid definition_id")
		}
		filter.DefinitionUUID = &definitionUUID
	}
	if req.GetState() != workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_UNSPECIFIED {
		filter.State = protoStateToInternal(req.GetState())
	}
	if ts := req.GetCreatedFrom(); ts != nil {
		t := ts.AsTime()
		filter.CreatedFrom = &t
	}
	if ts := req.GetCreatedTo(); ts != nil {
		t := ts.AsTime()
		filter.CreatedTo = &t
	}

	result, err := s.svc.ExportInstances(ctx, filter)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	rows := convertExportRowsToPB(result.Rows, tenantUUID)
	return &workflowv1.ExportInstancesResponse{
		Meta:        okMeta(ctx),
		Rows:        rows,
		DownloadUrl: result.DownloadURL,
	}, nil
}

func (s *Server) requireTenantContext(ctx context.Context, rpcCtx *commonv1.RequestContext) (string, error) {
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	if tenantUUID == "" && rpcCtx != nil {
		if attrs := rpcCtx.GetAttributes(); attrs != nil {
			tenantUUID = strings.TrimSpace(attrs["tenant_uuid"])
		}
	}
	if tenantUUID == "" {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is required")
	}
	canonical, err := reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "tenant uuid is invalid")
	}
	return canonical, nil
}

func okMeta(ctx context.Context) *commonv1.ResponseMeta {
	return &commonv1.ResponseMeta{
		Code:      httpStatusOK,
		Message:   "success",
		Timestamp: time.Now().Unix(),
		RequestId: requestIDFromContext(ctx),
	}
}

const (
	httpStatusOK = 200
)

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-request-id"); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func controlActionString(action workflowv1.ControlAction) string {
	switch action {
	case workflowv1.ControlAction_CONTROL_ACTION_PAUSE:
		return "pause"
	case workflowv1.ControlAction_CONTROL_ACTION_RESUME:
		return "resume"
	case workflowv1.ControlAction_CONTROL_ACTION_CANCEL:
		return "cancel"
	case workflowv1.ControlAction_CONTROL_ACTION_RETRY_STEP:
		return "retry_step"
	case workflowv1.ControlAction_CONTROL_ACTION_TRIGGER_COMPENSATION:
		return "trigger_compensation"
	case workflowv1.ControlAction_CONTROL_ACTION_REASSIGN_STEP:
		return "reassign_step"
	default:
		return strings.ToLower(action.String())
	}
}

func parseUint64(value string) uint64 {
	if value == "" {
		return 0
	}
	if id, err := strconv.ParseUint(value, 10, 64); err == nil {
		return id
	}
	return 0
}

func protoStateToInternal(state workflowv1.WorkflowInstanceState) string {
	if state == workflowv1.WorkflowInstanceState_WORKFLOW_INSTANCE_STATE_UNSPECIFIED {
		return ""
	}
	raw := strings.ToLower(state.String())
	return strings.TrimPrefix(raw, "workflow_instance_state_")
}

func protoFormatToInternal(format workflowv1.ExportFormat) workflowsvc.ExportFormat {
	switch format {
	case workflowv1.ExportFormat_EXPORT_FORMAT_JSON:
		return workflowsvc.ExportFormatJSON
	default:
		return workflowsvc.ExportFormatCSV
	}
}

func convertExportRowsToPB(rows []workflowsvc.ExportRow, tenantUUID string) []*workflowv1.WorkflowInstanceExportRow {
	if len(rows) == 0 {
		return nil
	}
	payload := make([]*workflowv1.WorkflowInstanceExportRow, 0, len(rows))
	for _, row := range rows {
		item := &workflowv1.WorkflowInstanceExportRow{
			InstanceId:        row.InstanceID,
			DefinitionId:      row.DefinitionID,
			DefinitionVersion: row.DefinitionVersion,
			State:             workflowInstanceState(row.State),
			TenantUuid:        row.TenantUUID,
			CorrelationId:     row.CorrelationID,
		}
		if row.StartedAt != nil {
			item.StartedAt = timestamppb.New(*row.StartedAt)
		}
		if row.CompletedAt != nil {
			item.CompletedAt = timestamppb.New(*row.CompletedAt)
		}
		if len(row.Steps) > 0 {
			steps := make([]*workflowv1.StepExecutionExport, 0, len(row.Steps))
			for _, step := range row.Steps {
				stepPayload := &workflowv1.StepExecutionExport{
					StepId:           step.StepID,
					Type:             workflowStepType(step.Type),
					State:            workflowStepState(step.State),
					SubjectType:      stepSubjectType(step.SubjectType),
					SubjectId:        step.SubjectID,
					Attempts:         int32(step.Attempts),
					ToolGrantVersion: step.ToolGrantVersion,
					LastError:        step.LastError,
				}
				if !step.LastTransitionAt.IsZero() {
					stepPayload.LastTransitionAt = timestamppb.New(step.LastTransitionAt)
				}
				steps = append(steps, stepPayload)
			}
			item.Steps = steps
		}
		payload = append(payload, item)
	}
	return payload
}
