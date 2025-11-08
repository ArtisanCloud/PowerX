package capability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	validator "github.com/ArtisanCloud/PowerX/internal/contract/capability"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type ContractServer struct {
	capb.UnimplementedCapabilityRegistryServiceServer
	contractSvc *svc.ContractService
	policySvc   *svc.VersionPolicyService
	adapterSvc  *svc.AdapterService
}

func NewContractServer(deps *shared.Deps) *ContractServer {
	validator := validator.NewValidator(validator.ValidatorOptions{})
	service := svc.NewContractService(deps.DB, validator, deps.AuditSvc)
	policyService := svc.NewVersionPolicyService(deps.DB, deps.AuditSvc)
	return &ContractServer{
		contractSvc: service,
		policySvc:   policyService,
		adapterSvc:  svc.NewAdapterService(deps.DB, nil, nil),
	}
}

func (s *ContractServer) GetCapability(ctx context.Context, req *capb.GetCapabilityRequest) (*capb.CapabilityContractResponse, error) {
	if req == nil {
		return capabilityErrorResponse(ctx, http.StatusBadRequest, "request cannot be nil", nil), nil
	}
	tenantID := parseTenantID(req.GetTenantId())
	contract, err := s.contractSvc.GetContract(ctx, tenantID, req.GetCapabilityKey(), req.GetVersion())
	if err != nil {
		return capabilityServiceErrorResponse(ctx, err)
	}
	pbContract, err := toPBContract(contract)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert contract: %v", err)
	}
	return &capb.CapabilityContractResponse{
		Meta: okMeta(ctx),
		Data: pbContract,
	}, nil
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

	items, total, err := s.contractSvc.ListContracts(ctx, tenantID, req.GetCapabilityKey(), limit, offset)
	if err != nil {
		return listCapabilitiesServiceErrorResponse(ctx, err)
	}

	pbItems := make([]*capb.CapabilityContract, 0, len(items))
	for _, item := range items {
		pbContract, err := toPBContract(item)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "convert contract: %v", err)
		}
		pbItems = append(pbItems, pbContract)
	}

	var nextToken string
	if int64(offset+len(items)) < total {
		nextToken = strconv.Itoa(offset + len(items))
	}

	return &capb.ListCapabilitiesResponse{
		Meta: okMeta(ctx),
		Data: &capb.ListCapabilitiesData{
			Items:         pbItems,
			NextPageToken: nextToken,
		},
	}, nil
}

func (s *ContractServer) UpsertCapability(ctx context.Context, req *capb.UpsertCapabilityRequest) (*capb.CapabilityContractResponse, error) {
	if req == nil || req.GetContract() == nil {
		return capabilityErrorResponse(ctx, http.StatusBadRequest, "contract payload is required", nil), nil
	}
	input, err := fromPBContract(req.GetContract())
	if err != nil {
		return capabilityErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	contract, issues, err := s.contractSvc.UpsertDraft(ctx, input)
	if err != nil {
		if errors.Is(err, svc.ErrValidation) {
			return capabilityValidationResponse(ctx, issues), nil
		}
		return capabilityServiceErrorResponse(ctx, err)
	}
	pbContract, err := toPBContract(contract)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert contract: %v", err)
	}
	return &capb.CapabilityContractResponse{
		Meta: okMeta(ctx),
		Data: pbContract,
	}, nil
}

func (s *ContractServer) PublishCapability(ctx context.Context, req *capb.PublishCapabilityRequest) (*capb.PublishCapabilityResponse, error) {
	if req == nil {
		return publishCapabilityErrorResponse(ctx, http.StatusBadRequest, "request cannot be nil", nil), nil
	}
	if req.GetEffectiveAt() == nil {
		return publishCapabilityErrorResponse(ctx, http.StatusBadRequest, "effective_at is required", nil), nil
	}
	input := &svc.PublishInput{
		TenantID:      0,
		CapabilityKey: req.GetCapabilityKey(),
		Version:       req.GetVersion(),
		EffectiveAt:   req.GetEffectiveAt().AsTime(),
		Notes:         req.GetNotes(),
	}
	contract, issues, err := s.contractSvc.PublishContract(ctx, input)
	if err != nil {
		if errors.Is(err, svc.ErrValidation) {
			return &capb.PublishCapabilityResponse{
				Meta:   badMeta(ctx, http.StatusBadRequest, issuesToMessage(issues)),
				Issues: toPBIssues(issues),
				Error:  errorExtra(nil, issuesDetailsStruct(issues)),
			}, nil
		}
		return publishCapabilityServiceErrorResponse(ctx, err)
	}
	pbContract, err := toPBContract(contract)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert contract: %v", err)
	}
	return &capb.PublishCapabilityResponse{
		Meta:     okMeta(ctx),
		Contract: pbContract,
		Issues:   toPBIssues(issues),
	}, nil
}

func (s *ContractServer) DeprecateCapability(ctx context.Context, req *capb.DeprecateCapabilityRequest) (*capb.CapabilityContractResponse, error) {
	if req == nil {
		return capabilityErrorResponse(ctx, http.StatusBadRequest, "request cannot be nil", nil), nil
	}
	if req.GetDeprecatedAt() == nil {
		return capabilityErrorResponse(ctx, http.StatusBadRequest, "deprecated_at is required", nil), nil
	}
	input := &svc.DeprecateInput{
		TenantID:              0,
		CapabilityKey:         req.GetCapabilityKey(),
		Version:               req.GetVersion(),
		DeprecatedAt:          req.GetDeprecatedAt().AsTime(),
		ReplacementCapability: req.GetReplacementCapability(),
		AdvisoryMessage:       req.GetAdvisoryMessage(),
	}
	contract, err := s.contractSvc.DeprecateContract(ctx, input)
	if err != nil {
		return capabilityServiceErrorResponse(ctx, err)
	}
	pbContract, err := toPBContract(contract)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert contract: %v", err)
	}
	return &capb.CapabilityContractResponse{
		Meta: okMeta(ctx),
		Data: pbContract,
	}, nil
}

func (s *ContractServer) ListTransportProfiles(ctx context.Context, req *capb.ListTransportProfilesRequest) (*capb.ListTransportProfilesResponse, error) {
	if req == nil {
		return listTransportProfilesErrorResponse(ctx, http.StatusBadRequest, "request cannot be nil", nil), nil
	}
	tenantID := reqctx.GetTenantID(ctx)
	items, err := s.adapterSvc.ListProfiles(ctx, tenantID, req.GetCapabilityKey(), req.GetVersion())
	if err != nil {
		return listTransportProfilesServiceErrorResponse(ctx, err)
	}
	profiles := make([]*capb.TransportProfile, 0, len(items))
	for _, item := range items {
		pbProfile, err := toPBTransportProfileFromView(item)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "convert transport profile: %v", err)
		}
		profiles = append(profiles, pbProfile)
	}
	return &capb.ListTransportProfilesResponse{
		Meta: okMeta(ctx),
		Data: &capb.ListTransportProfilesData{Transports: profiles},
	}, nil
}

func (s *ContractServer) GetVersionPolicy(ctx context.Context, req *capb.GetVersionPolicyRequest) (*capb.CapabilityVersionPolicyResponse, error) {
	if req == nil {
		return capabilityPolicyErrorResponse(ctx, http.StatusBadRequest, "request cannot be nil", nil), nil
	}
	policy, err := s.policySvc.GetVersionPolicy(ctx, 0, req.GetCapabilityKey())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &capb.CapabilityVersionPolicyResponse{
				Meta:  badMeta(ctx, http.StatusNotFound, err.Error()),
				Error: errorExtra(err, nil),
			}, nil
		}
		return capabilityPolicyServiceErrorResponse(ctx, err)
	}
	pbPolicy, err := toPBVersionPolicy(policy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert version policy: %v", err)
	}
	return &capb.CapabilityVersionPolicyResponse{
		Meta: okMeta(ctx),
		Data: pbPolicy,
	}, nil
}

func (s *ContractServer) UpsertVersionPolicy(ctx context.Context, req *capb.UpsertVersionPolicyRequest) (*capb.CapabilityVersionPolicyResponse, error) {
	if req == nil || req.GetPolicy() == nil {
		return capabilityPolicyErrorResponse(ctx, http.StatusBadRequest, "policy payload is required", nil), nil
	}
	input, err := fromPBVersionPolicy(req.GetPolicy())
	if err != nil {
		return capabilityPolicyErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
	}
	policy, err := s.policySvc.UpsertVersionPolicy(ctx, input)
	if err != nil {
		if errors.Is(err, svc.ErrPolicyValidation) {
			return capabilityPolicyErrorResponse(ctx, http.StatusBadRequest, err.Error(), err), nil
		}
		return capabilityPolicyServiceErrorResponse(ctx, err)
	}
	pbPolicy, err := toPBVersionPolicy(policy)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "convert version policy: %v", err)
	}
	return &capb.CapabilityVersionPolicyResponse{
		Meta: okMeta(ctx),
		Data: pbPolicy,
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
	for idx, issue := range issues {
		code := strings.TrimSpace(issue.Code)
		if code == "" {
			code = fmt.Sprintf("issue_%d", idx)
		}
		messages = append(messages, fmt.Sprintf("%s: %s", code, issue.Message))
	}
	return strings.Join(messages, "; ")
}

func issuesDetailsStruct(issues []validator.ValidationIssue) *structpb.Struct {
	if len(issues) == 0 {
		return nil
	}
	details := make(map[string]interface{}, len(issues))
	for idx, issue := range issues {
		key := strings.TrimSpace(issue.Code)
		if key == "" {
			key = fmt.Sprintf("issue_%d", idx)
		}
		entry := map[string]interface{}{
			"message":  issue.Message,
			"severity": string(issue.Severity),
		}
		if len(issue.Details) > 0 {
			entry["details"] = issue.Details
		}
		details[key] = entry
	}
	st, err := structpb.NewStruct(details)
	if err != nil {
		return nil
	}
	return st
}

func okMeta(ctx context.Context) *commonv1.ResponseMeta {
	reqID := reqctx.GetTraceID(ctx)
	return &commonv1.ResponseMeta{
		Code:      int32(http.StatusOK),
		Message:   "success",
		Timestamp: time.Now().Unix(),
		RequestId: reqID,
	}
}

func badMeta(ctx context.Context, code int, msg string) *commonv1.ResponseMeta {
	reqID := reqctx.GetTraceID(ctx)
	return &commonv1.ResponseMeta{
		Code:      int32(code),
		Message:   strings.TrimSpace(msg),
		Timestamp: time.Now().Unix(),
		RequestId: reqID,
	}
}

func errorExtra(err error, details *structpb.Struct) *commonv1.ErrorExtra {
	if err == nil && details == nil {
		return nil
	}
	extra := &commonv1.ErrorExtra{}
	if err != nil {
		extra.Error = err.Error()
	}
	if details != nil {
		extra.Details = details
	}
	return extra
}

func classifyCapabilityError(err error) (int, bool) {
	switch {
	case err == nil:
		return http.StatusOK, false
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, false
	default:
		return http.StatusInternalServerError, true
	}
}

func capabilityServiceErrorResponse(ctx context.Context, err error) (*capb.CapabilityContractResponse, error) {
	code, internal := classifyCapabilityError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "capability service error: %v", err)
	}
	return &capb.CapabilityContractResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err, nil),
	}, nil
}

func capabilityValidationResponse(ctx context.Context, issues []validator.ValidationIssue) *capb.CapabilityContractResponse {
	msg := issuesToMessage(issues)
	return &capb.CapabilityContractResponse{
		Meta:  badMeta(ctx, http.StatusBadRequest, msg),
		Error: errorExtra(errors.New(msg), issuesDetailsStruct(issues)),
	}
}

func capabilityErrorResponse(ctx context.Context, code int, msg string, err error) *capb.CapabilityContractResponse {
	return &capb.CapabilityContractResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err, nil),
	}
}

func listCapabilitiesServiceErrorResponse(ctx context.Context, err error) (*capb.ListCapabilitiesResponse, error) {
	code, internal := classifyCapabilityError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "capability service error: %v", err)
	}
	return &capb.ListCapabilitiesResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err, nil),
	}, nil
}

func publishCapabilityServiceErrorResponse(ctx context.Context, err error) (*capb.PublishCapabilityResponse, error) {
	code, internal := classifyCapabilityError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "capability service error: %v", err)
	}
	return &capb.PublishCapabilityResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err, nil),
	}, nil
}

func publishCapabilityErrorResponse(ctx context.Context, code int, msg string, err error) *capb.PublishCapabilityResponse {
	return &capb.PublishCapabilityResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err, nil),
	}
}

func listTransportProfilesServiceErrorResponse(ctx context.Context, err error) (*capb.ListTransportProfilesResponse, error) {
	if errors.Is(err, svc.ErrAdapterNotFound) {
		return &capb.ListTransportProfilesResponse{
			Meta:  badMeta(ctx, http.StatusNotFound, err.Error()),
			Error: errorExtra(err, nil),
		}, nil
	}
	code, internal := classifyCapabilityError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "capability service error: %v", err)
	}
	return &capb.ListTransportProfilesResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err, nil),
	}, nil
}

func listTransportProfilesErrorResponse(ctx context.Context, code int, msg string, err error) *capb.ListTransportProfilesResponse {
	return &capb.ListTransportProfilesResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err, nil),
	}
}

func capabilityPolicyServiceErrorResponse(ctx context.Context, err error) (*capb.CapabilityVersionPolicyResponse, error) {
	code, internal := classifyCapabilityError(err)
	if internal {
		return nil, status.Errorf(codes.Internal, "capability policy error: %v", err)
	}
	return &capb.CapabilityVersionPolicyResponse{
		Meta:  badMeta(ctx, code, err.Error()),
		Error: errorExtra(err, nil),
	}, nil
}

func capabilityPolicyErrorResponse(ctx context.Context, code int, msg string, err error) *capb.CapabilityVersionPolicyResponse {
	return &capb.CapabilityVersionPolicyResponse{
		Meta:  badMeta(ctx, code, msg),
		Error: errorExtra(err, nil),
	}
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

func toPBTransportProfileFromView(view svc.TransportProfile) (*capb.TransportProfile, error) {
	validatorProfile := validator.TransportProfile{
		Transport:        view.Transport,
		Mode:             view.Mode,
		TimeoutMillis:    view.TimeoutMillis,
		Streaming:        view.Streaming,
		Retry:            view.Retry,
		QoS:              view.QoS,
		EndpointSelector: view.EndpointSelector,
	}
	pbProfile, err := toPBTransportProfile(validatorProfile)
	if err != nil {
		return nil, err
	}
	if health, err := toPBHealthReport(view.HealthReport); err == nil {
		pbProfile.LastHealthStatus = health
	}
	return pbProfile, nil
}

func toPBHealthReport(report *svc.TransportHealthReport) (*capb.TransportHealthStatus, error) {
	if report == nil {
		return nil, nil
	}
	pbReport := &capb.TransportHealthStatus{Status: report.Status}
	if !report.CheckedAt.IsZero() {
		pbReport.CheckedAt = timestamppb.New(report.CheckedAt)
	}
	if report.LastError != nil {
		pbReport.LastError = &capb.ErrorTaxonomyEntry{
			Namespace:       report.LastError.Namespace,
			Category:        report.LastError.Category,
			Code:            report.LastError.Code,
			Severity:        severityToPB(report.LastError.Severity),
			Stage:           stageToPB(report.LastError.Stage),
			SuggestedAction: report.LastError.SuggestedAction,
		}
	}
	return pbReport, nil
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

func toPBVersionPolicy(policy *svc.VersionPolicy) (*capb.CapabilityVersionPolicy, error) {
	if policy == nil {
		return nil, nil
	}
	matrixStruct, err := structFromMap(policy.CompatibilityMatrix)
	if err != nil {
		return nil, err
	}
	pb := &capb.CapabilityVersionPolicy{
		CapabilityKey:       policy.CapabilityKey,
		DefaultStrategy:     versionStrategyToPB(policy.DefaultStrategy),
		CompatibilityMatrix: matrixStruct,
		DeprecationPolicy:   mapToDeprecationPolicy(policy.DeprecationPolicy),
		UpdatedAt:           timestamppb.New(policy.UpdatedAt),
	}
	for _, rule := range policy.AllowedVersions {
		pb.AllowedVersions = append(pb.AllowedVersions, &capb.VersionRule{
			Version:        rule.Version,
			CompatibleWith: rule.CompatibleWith,
			Status:         versionStatusToPB(rule.Status),
		})
	}
	return pb, nil
}

func fromPBVersionPolicy(pbPolicy *capb.CapabilityVersionPolicy) (*svc.VersionPolicyUpsertInput, error) {
	if pbPolicy == nil {
		return nil, errors.New("policy payload is nil")
	}
	rules := make([]svc.VersionRule, 0, len(pbPolicy.GetAllowedVersions()))
	for _, rule := range pbPolicy.GetAllowedVersions() {
		rules = append(rules, svc.VersionRule{
			Version:        rule.GetVersion(),
			CompatibleWith: rule.GetCompatibleWith(),
			Status:         pbVersionStatusToString(rule.GetStatus()),
		})
	}
	return &svc.VersionPolicyUpsertInput{
		TenantID:            0,
		CapabilityKey:       pbPolicy.GetCapabilityKey(),
		DefaultStrategy:     pbVersionStrategyToString(pbPolicy.GetDefaultStrategy()),
		AllowedVersions:     rules,
		CompatibilityMatrix: mapFromStruct(pbPolicy.GetCompatibilityMatrix()),
		DeprecationPolicy:   pbDeprecationPolicyToMap(pbPolicy.GetDeprecationPolicy()),
		AuditConfig:         map[string]interface{}{},
	}, nil
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

func versionStrategyToPB(val string) capb.VersionStrategy {
	switch strings.ToLower(val) {
	case "latest_minor":
		return capb.VersionStrategy_VERSION_STRATEGY_LATEST_MINOR
	case "fixed_major":
		return capb.VersionStrategy_VERSION_STRATEGY_FIXED_MAJOR
	case "custom":
		return capb.VersionStrategy_VERSION_STRATEGY_CUSTOM
	default:
		return capb.VersionStrategy_VERSION_STRATEGY_UNSPECIFIED
	}
}

func versionStatusToPB(val string) capb.VersionStatus {
	switch strings.ToLower(val) {
	case "active":
		return capb.VersionStatus_VERSION_STATUS_ACTIVE
	case "deprecated":
		return capb.VersionStatus_VERSION_STATUS_DEPRECATED
	case "blocked":
		return capb.VersionStatus_VERSION_STATUS_BLOCKED
	default:
		return capb.VersionStatus_VERSION_STATUS_UNSPECIFIED
	}
}

func pbVersionStrategyToString(v capb.VersionStrategy) string {
	switch v {
	case capb.VersionStrategy_VERSION_STRATEGY_LATEST_MINOR:
		return "latest_minor"
	case capb.VersionStrategy_VERSION_STRATEGY_FIXED_MAJOR:
		return "fixed_major"
	case capb.VersionStrategy_VERSION_STRATEGY_CUSTOM:
		return "custom"
	default:
		return ""
	}
}

func pbVersionStatusToString(v capb.VersionStatus) string {
	switch v {
	case capb.VersionStatus_VERSION_STATUS_ACTIVE:
		return "active"
	case capb.VersionStatus_VERSION_STATUS_DEPRECATED:
		return "deprecated"
	case capb.VersionStatus_VERSION_STATUS_BLOCKED:
		return "blocked"
	default:
		return ""
	}
}

func mapToDeprecationPolicy(m map[string]interface{}) *capb.DeprecationPolicy {
	if len(m) == 0 {
		return nil
	}
	dp := &capb.DeprecationPolicy{}
	if v, ok := m["warning_period_days"]; ok {
		dp.WarningPeriodDays = int32(intFromAny(v))
	}
	if v, ok := m["auto_deprecate_after_days"]; ok {
		dp.AutoDeprecateAfterDays = int32(intFromAny(v))
	}
	if v, ok := m["notify_channels"]; ok {
		dp.NotifyChannels = stringSliceFromAny(v)
	}
	if dp.WarningPeriodDays == 0 && dp.AutoDeprecateAfterDays == 0 && len(dp.NotifyChannels) == 0 {
		return nil
	}
	return dp
}

func pbDeprecationPolicyToMap(dp *capb.DeprecationPolicy) map[string]interface{} {
	if dp == nil {
		return map[string]interface{}{}
	}
	result := map[string]interface{}{}
	if dp.GetWarningPeriodDays() > 0 {
		result["warning_period_days"] = dp.GetWarningPeriodDays()
	}
	if dp.GetAutoDeprecateAfterDays() > 0 {
		result["auto_deprecate_after_days"] = dp.GetAutoDeprecateAfterDays()
	}
	if len(dp.GetNotifyChannels()) > 0 {
		result["notify_channels"] = dp.GetNotifyChannels()
	}
	return result
}

func intFromAny(v interface{}) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case float32:
		return int(val)
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}

func stringSliceFromAny(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
