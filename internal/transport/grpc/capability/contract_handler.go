package capability

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	validator "github.com/ArtisanCloud/PowerX/internal/contract/capability"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type ContractServer struct {
	capb.UnimplementedCapabilityRegistryServiceServer
	svc *svc.ContractService
}

func NewContractServer(deps *shared.Deps) *ContractServer {
	validator := validator.NewValidator(validator.ValidatorOptions{})
	service := svc.NewContractService(deps.DB, validator, deps.AuditSvc)
	return &ContractServer{svc: service}
}

func (s *ContractServer) GetCapability(ctx context.Context, req *capb.GetCapabilityRequest) (*capb.CapabilityContract, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	tenantID := parseTenantID(req.GetTenantId())
	contract, err := s.svc.GetContract(ctx, tenantID, req.GetCapabilityKey(), req.GetVersion())
	if err != nil {
		return nil, grpcError(err)
	}
	return toPBContract(contract)
}

func (s *ContractServer) ListCapabilities(ctx context.Context, req *capb.ListCapabilitiesRequest) (*capb.ListCapabilitiesResponse, error) {
	if req == nil {
		req = &capb.ListCapabilitiesRequest{}
	}
	tenantID := parseTenantID(req.GetTenantId())
	limit := int(req.GetPageSize())
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := parsePageToken(req.GetPageToken())

	items, total, err := s.svc.ListContracts(ctx, tenantID, req.GetCapabilityKey(), limit, offset)
	if err != nil {
		return nil, grpcError(err)
	}

	pbItems := make([]*capb.CapabilityContract, 0, len(items))
	for _, item := range items {
		pbContract, err := toPBContract(item)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		pbItems = append(pbItems, pbContract)
	}

	var nextToken string
	if int64(offset+len(items)) < total {
		nextToken = strconv.Itoa(offset + len(items))
	}

	return &capb.ListCapabilitiesResponse{Items: pbItems, NextPageToken: nextToken}, nil
}

func (s *ContractServer) UpsertCapability(ctx context.Context, req *capb.UpsertCapabilityRequest) (*capb.CapabilityContract, error) {
	if req == nil || req.GetContract() == nil {
		return nil, status.Error(codes.InvalidArgument, "contract payload is required")
	}
	input, err := fromPBContract(req.GetContract())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	contract, issues, err := s.svc.UpsertDraft(ctx, input)
	if err != nil {
		if errors.Is(err, svc.ErrValidation) {
			return nil, status.Error(codes.InvalidArgument, issuesToMessage(issues))
		}
		return nil, grpcError(err)
	}
	return toPBContract(contract)
}

func (s *ContractServer) PublishCapability(ctx context.Context, req *capb.PublishCapabilityRequest) (*capb.PublishCapabilityResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	if req.GetEffectiveAt() == nil {
		return nil, status.Error(codes.InvalidArgument, "effective_at is required")
	}
	input := &svc.PublishInput{
		TenantID:      0,
		CapabilityKey: req.GetCapabilityKey(),
		Version:       req.GetVersion(),
		EffectiveAt:   req.GetEffectiveAt().AsTime(),
		Notes:         req.GetNotes(),
	}
	contract, issues, err := s.svc.PublishContract(ctx, input)
	if err != nil {
		if errors.Is(err, svc.ErrValidation) {
			return &capb.PublishCapabilityResponse{Issues: toPBIssues(issues)}, nil
		}
		return nil, grpcError(err)
	}
	pbContract, err := toPBContract(contract)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &capb.PublishCapabilityResponse{Contract: pbContract, Issues: toPBIssues(issues)}, nil
}

func (s *ContractServer) DeprecateCapability(ctx context.Context, req *capb.DeprecateCapabilityRequest) (*capb.CapabilityContract, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	if req.GetDeprecatedAt() == nil {
		return nil, status.Error(codes.InvalidArgument, "deprecated_at is required")
	}
	input := &svc.DeprecateInput{
		TenantID:              0,
		CapabilityKey:         req.GetCapabilityKey(),
		Version:               req.GetVersion(),
		DeprecatedAt:          req.GetDeprecatedAt().AsTime(),
		ReplacementCapability: req.GetReplacementCapability(),
		AdvisoryMessage:       req.GetAdvisoryMessage(),
	}
	contract, err := s.svc.DeprecateContract(ctx, input)
	if err != nil {
		return nil, grpcError(err)
	}
	return toPBContract(contract)
}

func (s *ContractServer) ListTransportProfiles(ctx context.Context, req *capb.ListTransportProfilesRequest) (*capb.ListTransportProfilesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	contract, err := s.svc.GetContract(ctx, 0, req.GetCapabilityKey(), req.GetVersion())
	if err != nil {
		return nil, grpcError(err)
	}
	profiles := make([]*capb.TransportProfile, 0, len(contract.TransportProfiles))
	for _, profile := range contract.TransportProfiles {
		pbProfile, err := toPBTransportProfile(profile)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		profiles = append(profiles, pbProfile)
	}
	return &capb.ListTransportProfilesResponse{Transports: profiles}, nil
}

func (s *ContractServer) GetVersionPolicy(context.Context, *capb.GetVersionPolicyRequest) (*capb.CapabilityVersionPolicy, error) {
	return nil, status.Error(codes.Unimplemented, "version policy service not implemented yet")
}

func (s *ContractServer) UpsertVersionPolicy(context.Context, *capb.UpsertVersionPolicyRequest) (*capb.CapabilityVersionPolicy, error) {
	return nil, status.Error(codes.Unimplemented, "version policy service not implemented yet")
}

// ---------- 转换逻辑 ----------

func toPBContract(model *svc.Contract) (*capb.CapabilityContract, error) {
	if model == nil {
		return nil, nil
	}
	obsStruct, err := structFromMap(model.ObservabilityConfig)
	if err != nil {
		return nil, err
	}
	pb := &capb.CapabilityContract{
		CapabilityKey:       model.CapabilityKey,
		Version:             model.Version,
		TenantId:            strconv.FormatUint(model.TenantID, 10),
		ProviderId:          model.ProviderID,
		DisplayName:         model.DisplayName,
		Description:         model.Description,
		LifecycleState:      lifecycleToPB(model.LifecycleState),
		SecurityScope:       model.SecurityScope,
		ToolGrantRequired:   model.ToolGrantRequired,
		ObservabilityConfig: obsStruct,
		CreatedAt:           timestamppb.New(model.CreatedAt),
		UpdatedAt:           timestamppb.New(model.UpdatedAt),
	}
	for _, pref := range model.TransportPreferences {
		pb.TransportPreferences = append(pb.TransportPreferences, &capb.TransportPreference{
			Transport: transportToPB(pref.Transport),
			Mode:      transportModeToPB(pref.Mode),
		})
	}
	for _, schema := range model.IOSchemas {
		rules, err := structFromMap(schema.ValidationRules)
		if err != nil {
			return nil, err
		}
		pb.IoSchemas = append(pb.IoSchemas, &capb.IOSchemaDescriptor{
			Direction:       ioDirectionToPB(schema.Direction),
			Format:          ioFormatToPB(schema.Format),
			SchemaUri:       schema.SchemaURI,
			SchemaHash:      schema.SchemaHash,
			ValidationRules: rules,
		})
	}
	for _, entry := range model.ErrorTaxonomy {
		pb.ErrorTaxonomy = append(pb.ErrorTaxonomy, &capb.ErrorTaxonomyEntry{
			Namespace: entry.Namespace,
			Category:  entry.Category,
			Code:      entry.Code,
			Severity:  severityToPB(string(entry.Severity)),
			Stage:     stageToPB(entry.Stage),
		})
	}
	return pb, nil
}

func toPBTransportProfile(profile validator.TransportProfile) (*capb.TransportProfile, error) {
	qosStruct, err := structFromMap(profile.QoS)
	if err != nil {
		return nil, err
	}
	selectorStruct, err := structFromMap(profile.EndpointSelector)
	if err != nil {
		return nil, err
	}
	return &capb.TransportProfile{
		Transport: transportToPB(profile.Transport),
		Mode:      transportModeToPB(profile.Mode),
		TimeoutMs: int32(profile.TimeoutMillis),
		Retry: &capb.RetryPolicy{
			MaxAttempts: int32(intFromMap(profile.Retry, "max_attempts")),
			BackoffMs:   int32(intFromMap(profile.Retry, "backoff_ms")),
			Idempotent:  boolFromMap(profile.Retry, "idempotent"),
		},
		Streaming:        profile.Streaming,
		Qos:              qosStruct,
		EndpointSelector: selectorStruct,
	}, nil
}

func fromPBContract(pb *capb.CapabilityContract) (*svc.ContractUpsertInput, error) {
	if pb == nil {
		return nil, errors.New("contract payload is nil")
	}
	tenantID := parseTenantID(pb.GetTenantId())
	obs := mapFromStruct(pb.GetObservabilityConfig())

	ios := make([]validator.IOSchemaDescriptor, 0, len(pb.GetIoSchemas()))
	for _, schema := range pb.GetIoSchemas() {
		ios = append(ios, validator.IOSchemaDescriptor{
			Direction:       pbIOSchemaDirectionToString(schema.GetDirection()),
			Format:          pbIOSchemaFormatToString(schema.GetFormat()),
			SchemaURI:       schema.GetSchemaUri(),
			SchemaHash:      schema.GetSchemaHash(),
			ValidationRules: mapFromStruct(schema.GetValidationRules()),
		})
	}

	prefs := make([]validator.TransportPreference, 0, len(pb.GetTransportPreferences()))
	for _, pref := range pb.GetTransportPreferences() {
		prefs = append(prefs, validator.TransportPreference{
			Transport: pbTransportToString(pref.GetTransport()),
			Mode:      pbTransportModeToString(pref.GetMode()),
		})
	}

	profiles := []validator.TransportProfile{}

	errorsEntries := make([]validator.ErrorTaxonomyEntry, 0, len(pb.GetErrorTaxonomy()))
	for _, entry := range pb.GetErrorTaxonomy() {
		errorsEntries = append(errorsEntries, validator.ErrorTaxonomyEntry{
			Namespace: entry.GetNamespace(),
			Category:  entry.GetCategory(),
			Code:      entry.GetCode(),
			Severity:  pbSeverityToString(entry.GetSeverity()),
			Stage:     pbStageToString(entry.GetStage()),
		})
	}
	return &svc.ContractUpsertInput{
		TenantID:             tenantID,
		CapabilityKey:        pb.GetCapabilityKey(),
		Version:              pb.GetVersion(),
		ProviderID:           pb.GetProviderId(),
		DisplayName:          pb.GetDisplayName(),
		Description:          pb.GetDescription(),
		SecurityScope:        pb.GetSecurityScope(),
		ToolGrantRequired:    pb.GetToolGrantRequired(),
		ObservabilityConfig:  obs,
		IOSchemas:            ios,
		TransportPreferences: prefs,
		TransportProfiles:    profiles,
		ErrorTaxonomy:        errorsEntries,
	}, nil
}

func parseTenantID(val string) uint64 {
	if val == "" {
		return 0
	}
	id, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func parsePageToken(token string) int {
	if token == "" {
		return 0
	}
	v, err := strconv.Atoi(token)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func issuesToMessage(issues []validator.ValidationIssue) string {
	if len(issues) == 0 {
		return "validation failed"
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, fmt.Sprintf("%s: %s", issue.Code, issue.Message))
	}
	return strings.Join(messages, "; ")
}

func grpcError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return status.Error(codes.NotFound, err.Error())
	}
	return status.Error(codes.Internal, err.Error())
}

func mapFromStruct(s *structpb.Struct) map[string]interface{} {
	if s == nil {
		return map[string]interface{}{}
	}
	return s.AsMap()
}

func structFromMap(m map[string]interface{}) (*structpb.Struct, error) {
	if m == nil {
		m = map[string]interface{}{}
	}
	return structpb.NewStruct(m)
}

func transportToPB(val string) capb.TransportKind {
	switch strings.ToLower(val) {
	case "http":
		return capb.TransportKind_TRANSPORT_KIND_HTTP
	case "grpc":
		return capb.TransportKind_TRANSPORT_KIND_GRPC
	case "mcp":
		return capb.TransportKind_TRANSPORT_KIND_MCP
	case "agent":
		return capb.TransportKind_TRANSPORT_KIND_AGENT
	default:
		return capb.TransportKind_TRANSPORT_KIND_UNSPECIFIED
	}
}

func transportModeToPB(val string) capb.TransportMode {
	switch strings.ToLower(val) {
	case "prefer":
		return capb.TransportMode_TRANSPORT_MODE_PREFER
	case "only":
		return capb.TransportMode_TRANSPORT_MODE_ONLY
	case "fallback":
		return capb.TransportMode_TRANSPORT_MODE_FALLBACK
	default:
		return capb.TransportMode_TRANSPORT_MODE_UNSPECIFIED
	}
}

func ioDirectionToPB(val string) capb.IOSchemaDirection {
	switch strings.ToLower(val) {
	case "input":
		return capb.IOSchemaDirection_IO_SCHEMA_DIRECTION_INPUT
	case "output":
		return capb.IOSchemaDirection_IO_SCHEMA_DIRECTION_OUTPUT
	default:
		return capb.IOSchemaDirection_IO_SCHEMA_DIRECTION_UNSPECIFIED
	}
}

func ioFormatToPB(val string) capb.IOSchemaFormat {
	switch strings.ToLower(val) {
	case "json_schema":
		return capb.IOSchemaFormat_IO_SCHEMA_FORMAT_JSON_SCHEMA
	case "protobuf":
		return capb.IOSchemaFormat_IO_SCHEMA_FORMAT_PROTOBUF
	case "avro":
		return capb.IOSchemaFormat_IO_SCHEMA_FORMAT_AVRO
	default:
		return capb.IOSchemaFormat_IO_SCHEMA_FORMAT_UNSPECIFIED
	}
}

func lifecycleToPB(val string) capb.LifecycleState {
	switch strings.ToLower(val) {
	case "draft":
		return capb.LifecycleState_LIFECYCLE_STATE_DRAFT
	case "published":
		return capb.LifecycleState_LIFECYCLE_STATE_PUBLISHED
	case "deprecated":
		return capb.LifecycleState_LIFECYCLE_STATE_DEPRECATED
	default:
		return capb.LifecycleState_LIFECYCLE_STATE_UNSPECIFIED
	}
}

func severityToPB(val string) capb.ErrorSeverity {
	switch strings.ToUpper(val) {
	case "INFO":
		return capb.ErrorSeverity_ERROR_SEVERITY_INFO
	case "WARNING":
		return capb.ErrorSeverity_ERROR_SEVERITY_WARNING
	case "ERROR":
		return capb.ErrorSeverity_ERROR_SEVERITY_ERROR
	case "FATAL":
		return capb.ErrorSeverity_ERROR_SEVERITY_FATAL
	default:
		return capb.ErrorSeverity_ERROR_SEVERITY_UNSPECIFIED
	}
}

func stageToPB(val string) capb.ErrorStage {
	switch strings.ToLower(val) {
	case "validate":
		return capb.ErrorStage_ERROR_STAGE_VALIDATE
	case "invoke":
		return capb.ErrorStage_ERROR_STAGE_INVOKE
	case "stream":
		return capb.ErrorStage_ERROR_STAGE_STREAM
	case "observe":
		return capb.ErrorStage_ERROR_STAGE_OBSERVE
	default:
		return capb.ErrorStage_ERROR_STAGE_UNSPECIFIED
	}
}

func intFromMap(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		}
	}
	return 0
}

func boolFromMap(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case bool:
			return val
		case string:
			return strings.ToLower(val) == "true"
		}
	}
	return false
}

func toPBIssues(items []validator.ValidationIssue) []*capb.ValidationIssue {
	if len(items) == 0 {
		return nil
	}
	result := make([]*capb.ValidationIssue, 0, len(items))
	for _, item := range items {
		details, _ := structpb.NewStruct(item.Details)
		result = append(result, &capb.ValidationIssue{
			Code:     item.Code,
			Message:  item.Message,
			Severity: severityToPB(string(item.Severity)),
			Details:  details,
		})
	}
	return result
}

func pbTransportToString(kind capb.TransportKind) string {
	switch kind {
	case capb.TransportKind_TRANSPORT_KIND_HTTP:
		return "http"
	case capb.TransportKind_TRANSPORT_KIND_GRPC:
		return "grpc"
	case capb.TransportKind_TRANSPORT_KIND_MCP:
		return "mcp"
	case capb.TransportKind_TRANSPORT_KIND_AGENT:
		return "agent"
	default:
		return ""
	}
}

func pbTransportModeToString(mode capb.TransportMode) string {
	switch mode {
	case capb.TransportMode_TRANSPORT_MODE_PREFER:
		return "prefer"
	case capb.TransportMode_TRANSPORT_MODE_ONLY:
		return "only"
	case capb.TransportMode_TRANSPORT_MODE_FALLBACK:
		return "fallback"
	default:
		return ""
	}
}

func pbIOSchemaDirectionToString(dir capb.IOSchemaDirection) string {
	switch dir {
	case capb.IOSchemaDirection_IO_SCHEMA_DIRECTION_INPUT:
		return "input"
	case capb.IOSchemaDirection_IO_SCHEMA_DIRECTION_OUTPUT:
		return "output"
	default:
		return ""
	}
}

func pbIOSchemaFormatToString(format capb.IOSchemaFormat) string {
	switch format {
	case capb.IOSchemaFormat_IO_SCHEMA_FORMAT_JSON_SCHEMA:
		return "json_schema"
	case capb.IOSchemaFormat_IO_SCHEMA_FORMAT_PROTOBUF:
		return "protobuf"
	case capb.IOSchemaFormat_IO_SCHEMA_FORMAT_AVRO:
		return "avro"
	default:
		return ""
	}
}

func pbSeverityToString(severity capb.ErrorSeverity) string {
	switch severity {
	case capb.ErrorSeverity_ERROR_SEVERITY_INFO:
		return "INFO"
	case capb.ErrorSeverity_ERROR_SEVERITY_WARNING:
		return "WARNING"
	case capb.ErrorSeverity_ERROR_SEVERITY_ERROR:
		return "ERROR"
	case capb.ErrorSeverity_ERROR_SEVERITY_FATAL:
		return "FATAL"
	default:
		return ""
	}
}

func pbStageToString(stage capb.ErrorStage) string {
	switch stage {
	case capb.ErrorStage_ERROR_STAGE_VALIDATE:
		return "validate"
	case capb.ErrorStage_ERROR_STAGE_INVOKE:
		return "invoke"
	case capb.ErrorStage_ERROR_STAGE_STREAM:
		return "stream"
	case capb.ErrorStage_ERROR_STAGE_OBSERVE:
		return "observe"
	default:
		return ""
	}
}
