package agentmodelhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentmodelhubv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent_model_hub/v1"
	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	connectorguard "github.com/ArtisanCloud/PowerX/internal/service/connector_guard"
	costquota "github.com/ArtisanCloud/PowerX/internal/service/cost_quota"
	modelrouting "github.com/ArtisanCloud/PowerX/internal/service/model_routing"
	providerregistry "github.com/ArtisanCloud/PowerX/internal/service/provider_registry"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Server struct {
	agentmodelhubv1.UnimplementedAgentModelHubServiceServer
	providerRegistry *providerregistry.Service
	routingSvc       *modelrouting.Service
	costSvc          *costquota.Service
	connectorSvc     *connectorguard.Service
}

func NewServer(deps *appshared.Deps) *Server {
	return &Server{
		providerRegistry: newProviderRegistryService(deps),
		routingSvc:       newRoutingService(deps),
		costSvc:          newCostService(deps),
		connectorSvc:     newConnectorService(deps),
	}
}

func (s *Server) RegisterProvider(ctx context.Context, req *agentmodelhubv1.RegisterProviderRequest) (*agentmodelhubv1.RegisterProviderResponse, error) {
	if req == nil || req.GetProfile() == nil {
		return nil, status.Error(codes.InvalidArgument, "profile required")
	}
	if s.providerRegistry == nil {
		return nil, status.Error(codes.Unavailable, "provider registry unavailable")
	}
	profileInput := req.GetProfile()
	record, err := s.providerRegistry.RegisterProvider(ctx, "default", nil, providerregistry.ProviderProfileInput{
		Name:            strings.TrimSpace(profileInput.GetName()),
		Capabilities:    append([]string(nil), profileInput.GetCapabilities()...),
		PrimaryEndpoint: profileInput.GetPrimaryEndpoint(),
		Regions:         append([]string(nil), profileInput.GetRegions()...),
		TenantWhitelist: protoTenantRefsToService(profileInput.GetTenantWhitelist()),
		Credentials:     profileInput.GetCredentials(),
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.RegisterProviderResponse{
		Profile: toProtoProfile(record),
	}, nil
}

func (s *Server) ValidateProvider(ctx context.Context, req *agentmodelhubv1.ValidateProviderRequest) (*agentmodelhubv1.ValidateProviderResponse, error) {
	if req == nil || strings.TrimSpace(req.GetProviderId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "provider_id required")
	}
	if s.providerRegistry == nil {
		return nil, status.Error(codes.Unavailable, "provider registry unavailable")
	}
	id, err := uuid.Parse(req.GetProviderId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid provider_id")
	}
	if _, err := s.providerRegistry.ValidateProvider(ctx, id, req.GetSuite(), nil); err != nil {
		if errors.Is(err, providerregistry.ErrProviderNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.ValidateProviderResponse{}, nil
}

func (s *Server) PublishProvider(ctx context.Context, req *agentmodelhubv1.PublishProviderRequest) (*agentmodelhubv1.PublishProviderResponse, error) {
	if req == nil || strings.TrimSpace(req.GetProviderId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "provider_id required")
	}
	if s.providerRegistry == nil {
		return nil, status.Error(codes.Unavailable, "provider registry unavailable")
	}
	id, err := uuid.Parse(req.GetProviderId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid provider_id")
	}
	_, err = s.providerRegistry.PublishProvider(ctx, id, providerregistry.PublishOptions{
		TenantWhitelist:       protoTenantRefsToService(req.GetTenantWhitelist()),
		RolloutStrategy:       rolloutStrategyToString(req.GetRolloutStrategy()),
		RollbackTimeoutMinute: req.GetRollbackTimeoutMinutes(),
	})
	if err != nil {
		if errors.Is(err, providerregistry.ErrProviderNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.PublishProviderResponse{}, nil
}

func (s *Server) UpsertRoutingPolicy(ctx context.Context, req *agentmodelhubv1.UpsertRoutingPolicyRequest) (*agentmodelhubv1.UpsertRoutingPolicyResponse, error) {
	if req == nil || req.GetPolicy() == nil {
		return nil, status.Error(codes.InvalidArgument, "policy required")
	}
	if s.routingSvc == nil {
		return nil, status.Error(codes.Unavailable, "routing service unavailable")
	}
	env := "default"
	policy := req.GetPolicy()
	rulesJSON, err := json.Marshal(protoRulesToAny(policy.GetRules()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid rules")
	}
	fallback := protoFallbackToArray(policy.GetFallbackChain())
	fallbackJSON, err := json.Marshal(fallback)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid fallback")
	}
	safeMode := safeModeToMap(policy.GetSafeModeThresholds())
	approval := approvalConfigToMap(policy.GetApprovalConfig())
	input := modelrouting.PolicyInput{
		TenantScope:        strings.TrimSpace(policy.GetTenantScope()),
		Rules:              datatypes.JSON(rulesJSON),
		FallbackChain:      datatypes.JSON(fallbackJSON),
		SafeModeThresholds: mapToJSONMap(safeMode),
		ApprovalRecord:     mapToJSONMap(approval),
		Status:             "draft",
	}
	record, err := s.routingSvc.UpsertPolicyVersion(ctx, env, input)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.UpsertRoutingPolicyResponse{Policy: routingPolicyToProto(record)}, nil
}

func (s *Server) UpdateRoutingPolicyStatus(ctx context.Context, req *agentmodelhubv1.UpdateRoutingPolicyStatusRequest) (*agentmodelhubv1.UpdateRoutingPolicyStatusResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantScope()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_scope required")
	}
	if s.routingSvc == nil {
		return nil, status.Error(codes.Unavailable, "routing service unavailable")
	}
	target := policyStatusToString(req.GetTargetStatus())
	if target == "" {
		return nil, status.Error(codes.InvalidArgument, "target_status required")
	}
	input := modelrouting.StatusUpdateInput{
		TargetStatus: target,
		Reason:       req.GetReason(),
		Actor:        req.GetActor(),
		Approval:     protoApprovalOutcomeToService(req.GetApproval()),
	}
	policy, err := s.routingSvc.UpdatePolicyStatus(ctx, "default", req.GetTenantScope(), req.GetVersion(), input)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.UpdateRoutingPolicyStatusResponse{
		Policy: routingPolicyToProto(policy),
	}, nil
}

func (s *Server) RouteTask(ctx context.Context, req *agentmodelhubv1.RouteTaskRequest) (*agentmodelhubv1.RouteTaskResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	if s.routingSvc == nil {
		return nil, status.Error(codes.Unavailable, "routing service unavailable")
	}
	taskCtx := make(map[string]any, len(req.GetTaskContext()))
	for k, v := range req.GetTaskContext() {
		taskCtx[k] = v
	}
	result, err := s.routingSvc.DecideRoute(ctx, "default", req.GetTenantId(), taskCtx)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.RouteTaskResponse{
		PolicyVersion:     result.PolicyVersion,
		PrimaryProviderId: result.PrimaryProviderID,
		FallbackChain:     result.FallbackChain,
		TraceId:           result.TraceID,
	}, nil
}

func (s *Server) RollbackRoutingPolicy(ctx context.Context, req *agentmodelhubv1.RollbackRoutingPolicyRequest) (*agentmodelhubv1.RollbackRoutingPolicyResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantScope()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_scope required")
	}
	if s.routingSvc == nil {
		return nil, status.Error(codes.Unavailable, "routing service unavailable")
	}
	env := "default"
	if _, err := s.routingSvc.RollbackPolicy(ctx, env, req.GetTenantScope(), req.GetTargetVersion()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.RollbackRoutingPolicyResponse{}, nil
}

func (s *Server) ToggleSafeMode(ctx context.Context, req *agentmodelhubv1.ToggleSafeModeRequest) (*agentmodelhubv1.ToggleSafeModeResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantScope()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_scope required")
	}
	if s.routingSvc == nil {
		return nil, status.Error(codes.Unavailable, "routing service unavailable")
	}
	var ttl time.Duration
	if req.GetTtlSeconds() > 0 {
		ttl = time.Duration(req.GetTtlSeconds()) * time.Second
	}
	state, err := s.routingSvc.ToggleSafeMode(ctx, "default", req.GetTenantScope(), req.GetEnabled(), ttl, req.GetActor(), req.GetReason())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.ToggleSafeModeResponse{
		State: safeModeStateToProto(state),
	}, nil
}

func (s *Server) ReportUsage(ctx context.Context, req *agentmodelhubv1.ReportUsageRequest) (*agentmodelhubv1.ReportUsageResponse, error) {
	report := req.GetReport()
	if report == nil || strings.TrimSpace(report.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "report.tenant_id required")
	}
	if s.costSvc == nil {
		return nil, status.Error(codes.Unavailable, "cost quota service unavailable")
	}
	input := costquota.UsageIngestInput{
		TenantID: strings.TrimSpace(report.GetTenantId()),
	}
	if parsed, err := uuid.Parse(strings.TrimSpace(report.GetProviderId())); err == nil {
		input.ProviderID = &parsed
	}
	for _, evt := range report.GetEvents() {
		input.Events = append(input.Events, costquota.UsageIngestEvent{
			CostUSD: evt.GetCostUsd(),
			Tokens:  evt.GetTokens(),
		})
	}
	if _, err := s.costSvc.ProcessUsage(ctx, "default", input); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.ReportUsageResponse{}, nil
}

func (s *Server) GetQuotaSnapshot(ctx context.Context, req *agentmodelhubv1.GetQuotaSnapshotRequest) (*agentmodelhubv1.GetQuotaSnapshotResponse, error) {
	if req == nil || strings.TrimSpace(req.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	if s.costSvc == nil {
		return nil, status.Error(codes.Unavailable, "cost quota service unavailable")
	}
	ledgers, err := s.costSvc.ListLedgers(ctx, "default", req.GetTenantId())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	snapshot := &agentmodelhubv1.QuotaSnapshot{
		TenantId: req.GetTenantId(),
	}
	for _, ledger := range ledgers {
		health := quotaHealthStatus(&ledger)
		snapshot.Quotas = append(snapshot.Quotas, &agentmodelhubv1.QuotaEntry{
			ProviderId: providerDisplayID(ledger.ProviderProfileID),
			Limit:      ledger.QuotaLimit,
			Usage:      ledger.UsageActual,
			Status:     mapQuotaStatus(health),
		})
	}
	return &agentmodelhubv1.GetQuotaSnapshotResponse{Snapshot: snapshot}, nil
}

func (s *Server) EnforceQuotaAction(ctx context.Context, req *agentmodelhubv1.EnforceQuotaActionRequest) (*agentmodelhubv1.EnforceQuotaActionResponse, error) {
	if req == nil || req.GetRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}
	if s.costSvc == nil {
		return nil, status.Error(codes.Unavailable, "cost quota service unavailable")
	}
	body := req.GetRequest()
	input := costquota.EnforcementInput{
		TenantID:    body.GetTenantId(),
		Reason:      body.GetReason(),
		TicketID:    body.GetTicketId(),
		RequestedBy: "",
		Action:      mapEnforcementAction(body.GetAction()),
	}
	if parsed, err := uuid.Parse(strings.TrimSpace(body.GetProviderId())); err == nil {
		input.ProviderID = &parsed
	}
	if _, err := s.costSvc.EnforceAction(ctx, "default", input); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.EnforceQuotaActionResponse{}, nil
}

func (s *Server) UpsertConnectorInstance(ctx context.Context, req *agentmodelhubv1.UpsertConnectorInstanceRequest) (*agentmodelhubv1.UpsertConnectorInstanceResponse, error) {
	if req == nil || strings.TrimSpace(req.GetPlatform()) == "" {
		return nil, status.Error(codes.InvalidArgument, "platform required")
	}
	if s.connectorSvc == nil {
		return nil, status.Error(codes.Unavailable, "connector service unavailable")
	}
	payload := req.GetInstance()
	if payload == nil || strings.TrimSpace(payload.GetTenantId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "instance.tenant_id required")
	}
	mapping := datatypes.JSON([]byte("{}"))
	if trimmed := strings.TrimSpace(payload.GetMappingTemplateJson()); trimmed != "" {
		mapping = datatypes.JSON([]byte(trimmed))
	}
	result, err := s.connectorSvc.UpsertInstance(ctx, "default", connectorguard.ConnectorInstanceInput{
		TenantScope:          payload.GetTenantId(),
		Platform:             req.GetPlatform(),
		Region:               payload.GetRegion(),
		OAuthRef:             payload.GetOauthRef(),
		WebhookSigningKeyRef: payload.GetWebhookSigningKeyRef(),
		MappingTemplate:      mapping,
		RateLimitPerMinute:   payload.GetRateLimitPerMinute(),
		Status:               "",
		InstanceID:           "",
	})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.UpsertConnectorInstanceResponse{
		Instance: connectorInstanceToProto(result),
	}, nil
}

func (s *Server) PauseConnectorInstance(ctx context.Context, req *agentmodelhubv1.PauseConnectorInstanceRequest) (*agentmodelhubv1.PauseConnectorInstanceResponse, error) {
	if req == nil || strings.TrimSpace(req.GetInstanceId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "instance_id required")
	}
	if s.connectorSvc == nil {
		return nil, status.Error(codes.Unavailable, "connector service unavailable")
	}
	id, err := uuid.Parse(req.GetInstanceId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid instance_id")
	}
	if err := s.connectorSvc.PauseInstance(ctx, id, req.GetReason()); err != nil {
		if errors.Is(err, connectorguard.ErrConnectorNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &agentmodelhubv1.PauseConnectorInstanceResponse{}, nil
}

func newProviderRegistryService(deps *appshared.Deps) *providerregistry.Service {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return providerregistry.NewService(providerregistry.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			TenantKeySvc:    buildTenantKeyService(deps.DB),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
		Artifacts: providerregistry.ValidationArtifactOptions{
			MediaManager: deps.MediaMgr,
			Bucket:       "agent",
			Prefix:       "providers",
		},
	})
}

func newRoutingService(deps *appshared.Deps) *modelrouting.Service {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return modelrouting.NewService(modelrouting.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
}

func newCostService(deps *appshared.Deps) *costquota.Service {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return costquota.NewService(costquota.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
}

func newConnectorService(deps *appshared.Deps) *connectorguard.Service {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return connectorguard.NewService(connectorguard.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			TenantKeySvc:    buildTenantKeyService(deps.DB),
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
}

func toProtoProfile(profile *model.ProviderProfile) *agentmodelhubv1.ProviderProfile {
	if profile == nil {
		return nil
	}
	data := &agentmodelhubv1.ProviderProfileInput{
		Name:            profile.Name,
		Capabilities:    cloneStringSlice([]string(profile.Capabilities)),
		PrimaryEndpoint: profile.PrimaryEndpoint,
		Regions:         cloneStringSlice([]string(profile.Regions)),
		TenantWhitelist: serviceTenantRefsToProto(providerregistry.DecodeTenantWhitelist(profile.TenantWhitelist)),
		Credentials:     convertSecretRefs(profile.SecretRefs),
	}
	return &agentmodelhubv1.ProviderProfile{
		ProviderId:    profile.UUID.String(),
		Data:          data,
		RolloutStatus: mapStatus(profile.RolloutStatus),
	}
}

func mapStatus(status string) agentmodelhubv1.RolloutStatus {
	switch strings.ToLower(status) {
	case "validating":
		return agentmodelhubv1.RolloutStatus_ROLLOUT_STATUS_VALIDATING
	case "gray":
		return agentmodelhubv1.RolloutStatus_ROLLOUT_STATUS_GRAY
	case "live":
		return agentmodelhubv1.RolloutStatus_ROLLOUT_STATUS_LIVE
	case "rolled_back":
		return agentmodelhubv1.RolloutStatus_ROLLOUT_STATUS_ROLLED_BACK
	default:
		return agentmodelhubv1.RolloutStatus_ROLLOUT_STATUS_DRAFT
	}
}

func protoTenantRefsToService(refs []*agentmodelhubv1.TenantRef) []providerregistry.TenantRef {
	out := make([]providerregistry.TenantRef, 0, len(refs))
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		out = append(out, providerregistry.TenantRef{
			TenantID:    strings.TrimSpace(ref.GetTenantId()),
			Environment: strings.TrimSpace(ref.GetEnvironment()),
		})
	}
	return out
}

func serviceTenantRefsToProto(refs []providerregistry.TenantRef) []*agentmodelhubv1.TenantRef {
	out := make([]*agentmodelhubv1.TenantRef, 0, len(refs))
	for _, ref := range refs {
		ref := ref
		out = append(out, &agentmodelhubv1.TenantRef{
			TenantId:    ref.TenantID,
			Environment: ref.Environment,
		})
	}
	return out
}

func getString(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func connectorInstanceToProto(inst *model.ConnectorInstance) *agentmodelhubv1.ConnectorInstance {
	if inst == nil {
		return nil
	}
	mapping := ""
	if len(inst.MappingTemplate) > 0 {
		mapping = string(inst.MappingTemplate)
	}
	return &agentmodelhubv1.ConnectorInstance{
		InstanceId: inst.UUID.String(),
		Platform:   inst.Platform,
		Data: &agentmodelhubv1.ConnectorInstanceInput{
			TenantId:             inst.TenantScope,
			Region:               inst.Region,
			OauthRef:             inst.OAuthRef,
			WebhookSigningKeyRef: inst.WebhookSigningKeyRef,
			MappingTemplateJson:  mapping,
			RateLimitPerMinute:   inst.RateLimitPerMinute,
		},
		Status:    connectorStatusToProto(inst.Status),
		ErrorRate: inst.ErrorRate,
	}
}

func connectorStatusToProto(status string) agentmodelhubv1.ConnectorStatus {
	switch strings.ToLower(status) {
	case "paused":
		return agentmodelhubv1.ConnectorStatus_CONNECTOR_STATUS_PAUSED
	case "degrading":
		return agentmodelhubv1.ConnectorStatus_CONNECTOR_STATUS_DEGRADING
	default:
		return agentmodelhubv1.ConnectorStatus_CONNECTOR_STATUS_ACTIVE
	}
}

func rolloutStrategyToString(strategy agentmodelhubv1.RolloutStrategy) string {
	switch strategy {
	case agentmodelhubv1.RolloutStrategy_ROLLOUT_STRATEGY_FULL:
		return "full"
	case agentmodelhubv1.RolloutStrategy_ROLLOUT_STRATEGY_PERCENTAGE:
		return "percentage"
	case agentmodelhubv1.RolloutStrategy_ROLLOUT_STRATEGY_CANARY:
		return "canary"
	default:
		return ""
	}
}

func protoRulesToAny(rules []*agentmodelhubv1.RoutingRule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		candidates := make([]map[string]any, 0, len(rule.GetCandidates()))
		for _, cand := range rule.GetCandidates() {
			if cand == nil {
				continue
			}
			candidates = append(candidates, map[string]any{
				"providerId": cand.GetProviderId(),
				"weight":     cand.GetWeight(),
			})
		}
		sla := map[string]any{}
		if rule.GetSla() != nil {
			sla["latencyMs"] = rule.GetSla().GetLatencyMs()
			sla["costCeiling"] = rule.GetSla().GetCostCeiling()
		}
		out = append(out, map[string]any{
			"taskPattern": rule.GetTaskPattern(),
			"candidates":  candidates,
			"sla":         sla,
		})
	}
	return out
}

func protoFallbackToArray(weights []*agentmodelhubv1.ProviderWeight) []string {
	out := make([]string, 0, len(weights))
	for _, w := range weights {
		if w == nil {
			continue
		}
		if id := strings.TrimSpace(w.GetProviderId()); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func mapToJSONMap(src map[string]any) datatypes.JSONMap {
	if len(src) == 0 {
		return datatypes.JSONMap{}
	}
	out := datatypes.JSONMap{}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func approvalConfigToMap(cfg *agentmodelhubv1.ApprovalConfig) map[string]any {
	if cfg == nil {
		return map[string]any{}
	}
	return map[string]any{
		"workflowId":        cfg.GetWorkflowId(),
		"requiredApprovers": cfg.GetRequiredApprovers(),
	}
}

func routingPolicyToProto(policy *model.RoutingPolicy) *agentmodelhubv1.RoutingPolicy {
	if policy == nil {
		return nil
	}
	return &agentmodelhubv1.RoutingPolicy{
		PolicyId: policy.UUID.String(),
		Version:  policy.Version,
		Data: &agentmodelhubv1.RoutingPolicyInput{
			TenantScope:        policy.TenantScope,
			Rules:              jsonToProtoRules(policy.Rules),
			FallbackChain:      jsonToProtoFallback(policy.FallbackChain),
			SafeModeThresholds: jsonToProtoSafeMode(policy.SafeModeThresholds),
			ApprovalConfig:     jsonToProtoApproval(policy.ApprovalRecord),
		},
		Status: mapPolicyStatus(policy.Status),
	}
}

func jsonToProtoRules(raw datatypes.JSON) []*agentmodelhubv1.RoutingRule {
	var payload []struct {
		TaskPattern string           `json:"taskPattern"`
		Candidates  []map[string]any `json:"candidates"`
		SLA         map[string]any   `json:"sla"`
	}
	_ = json.Unmarshal(raw, &payload)
	out := make([]*agentmodelhubv1.RoutingRule, 0, len(payload))
	for _, item := range payload {
		candidates := make([]*agentmodelhubv1.ProviderWeight, 0, len(item.Candidates))
		for _, cand := range item.Candidates {
			candidates = append(candidates, &agentmodelhubv1.ProviderWeight{
				ProviderId: getString(cand["providerId"]),
				Weight:     toFloat64(cand["weight"]),
			})
		}
		out = append(out, &agentmodelhubv1.RoutingRule{
			TaskPattern: item.TaskPattern,
			Candidates:  candidates,
			Sla: &agentmodelhubv1.SLAConstraints{
				LatencyMs:   uint32(toFloat64(item.SLA["latencyMs"])),
				CostCeiling: toFloat64(item.SLA["costCeiling"]),
			},
		})
	}
	return out
}

func jsonToProtoFallback(raw datatypes.JSON) []*agentmodelhubv1.ProviderWeight {
	var arr []string
	_ = json.Unmarshal(raw, &arr)
	out := make([]*agentmodelhubv1.ProviderWeight, 0, len(arr))
	for _, id := range arr {
		out = append(out, &agentmodelhubv1.ProviderWeight{ProviderId: id})
	}
	return out
}

func jsonToProtoSafeMode(raw datatypes.JSONMap) *agentmodelhubv1.SafeModeThresholds {
	if raw == nil {
		return &agentmodelhubv1.SafeModeThresholds{}
	}
	return &agentmodelhubv1.SafeModeThresholds{
		MinHitRate:             toFloat64(raw["minHitRate"]),
		MaxLatencyMs:           uint32(toFloat64(raw["maxLatencyMs"])),
		MaxFallbackFailureRate: toFloat64(raw["maxFallbackFailureRate"]),
	}
}

func jsonToProtoApproval(raw datatypes.JSONMap) *agentmodelhubv1.ApprovalConfig {
	if raw == nil {
		return &agentmodelhubv1.ApprovalConfig{}
	}
	return &agentmodelhubv1.ApprovalConfig{
		WorkflowId:        getString(raw["workflowId"]),
		RequiredApprovers: uint32(toFloat64(raw["requiredApprovers"])),
	}
}

func mapPolicyStatus(status string) agentmodelhubv1.PolicyStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "staged":
		return agentmodelhubv1.PolicyStatus_POLICY_STATUS_STAGED
	case "active":
		return agentmodelhubv1.PolicyStatus_POLICY_STATUS_ACTIVE
	case "rolled_back":
		return agentmodelhubv1.PolicyStatus_POLICY_STATUS_ROLLED_BACK
	default:
		return agentmodelhubv1.PolicyStatus_POLICY_STATUS_DRAFT
	}
}

func policyStatusToString(status agentmodelhubv1.PolicyStatus) string {
	switch status {
	case agentmodelhubv1.PolicyStatus_POLICY_STATUS_STAGED:
		return "staged"
	case agentmodelhubv1.PolicyStatus_POLICY_STATUS_ACTIVE:
		return "active"
	case agentmodelhubv1.PolicyStatus_POLICY_STATUS_ROLLED_BACK:
		return "rolled_back"
	case agentmodelhubv1.PolicyStatus_POLICY_STATUS_DRAFT:
		return "draft"
	default:
		return ""
	}
}

func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

func safeModeToMap(th *agentmodelhubv1.SafeModeThresholds) map[string]any {
	if th == nil {
		return map[string]any{}
	}
	return map[string]any{
		"minHitRate":             th.GetMinHitRate(),
		"maxLatencyMs":           th.GetMaxLatencyMs(),
		"maxFallbackFailureRate": th.GetMaxFallbackFailureRate(),
	}
}

func protoApprovalOutcomeToService(payload *agentmodelhubv1.ApprovalOutcome) *modelrouting.ApprovalUpdate {
	if payload == nil {
		return nil
	}
	approvers := make([]string, 0, len(payload.GetApprovers()))
	for _, ap := range payload.GetApprovers() {
		if trimmed := strings.TrimSpace(ap); trimmed != "" {
			approvers = append(approvers, trimmed)
		}
	}
	var decided time.Time
	if ts := payload.GetDecidedAt(); ts != nil {
		decided = ts.AsTime()
	}
	return &modelrouting.ApprovalUpdate{
		WorkflowID:        strings.TrimSpace(payload.GetWorkflowId()),
		RequestedBy:       strings.TrimSpace(payload.GetRequestedBy()),
		Approvers:         approvers,
		Outcome:           strings.TrimSpace(payload.GetOutcome()),
		Notes:             strings.TrimSpace(payload.GetNotes()),
		RequiredApprovers: payload.GetRequiredApprovers(),
		DecidedAt:         decided,
	}
}

func safeModeStateToProto(state *modelrouting.SafeModeState) *agentmodelhubv1.SafeModeState {
	if state == nil {
		return nil
	}
	result := &agentmodelhubv1.SafeModeState{
		TenantScope: state.TenantScope,
		Env:         state.Env,
		Enabled:     state.Enabled,
		Reason:      state.Reason,
		Actor:       state.Actor,
	}
	if !state.UpdatedAt.IsZero() {
		result.UpdatedAt = timestamppb.New(state.UpdatedAt)
	}
	if state.ExpiresAt != nil && !state.ExpiresAt.IsZero() {
		result.ExpiresAt = timestamppb.New(state.ExpiresAt.UTC())
	}
	return result
}

func mapQuotaStatus(status string) agentmodelhubv1.QuotaStatus {
	switch strings.ToLower(status) {
	case "warning":
		return agentmodelhubv1.QuotaStatus_QUOTA_STATUS_WARNING
	case "breached":
		return agentmodelhubv1.QuotaStatus_QUOTA_STATUS_BREACHED
	default:
		return agentmodelhubv1.QuotaStatus_QUOTA_STATUS_HEALTHY
	}
}

func mapEnforcementAction(action agentmodelhubv1.EnforcementAction) string {
	switch action {
	case agentmodelhubv1.EnforcementAction_ENFORCEMENT_ACTION_DEGRADE:
		return "degrade"
	case agentmodelhubv1.EnforcementAction_ENFORCEMENT_ACTION_DISABLE:
		return "disable"
	default:
		return "throttle"
	}
}

func quotaHealthStatus(ledger *model.CostQuotaLedger) string {
	if ledger == nil || ledger.QuotaLimit <= 0 {
		return "healthy"
	}
	usage := ledger.UsageActual / ledger.QuotaLimit
	switch {
	case usage >= 1:
		return "breached"
	case usage >= 0.9:
		return "warning"
	default:
		return "healthy"
	}
}

func providerDisplayID(id *uuid.UUID) string {
	if id == nil {
		return "tenant"
	}
	return id.String()
}

func pickPrimaryAndFallbackRouting(policy *model.RoutingPolicy) (string, []string) {
	var rules []struct {
		Candidates []struct {
			ProviderID string `json:"providerId"`
		} `json:"candidates"`
	}
	_ = json.Unmarshal(policy.Rules, &rules)
	primary := ""
	if len(rules) > 0 && len(rules[0].Candidates) > 0 {
		primary = strings.TrimSpace(rules[0].Candidates[0].ProviderID)
	}
	var fallback []string
	_ = json.Unmarshal(policy.FallbackChain, &fallback)
	return primary, fallback
}

func cloneStringSlice(src []string) []string {
	if len(src) == 0 {
		return []string{}
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

func convertSecretRefs(src datatypes.JSONMap) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		if str, ok := v.(string); ok && str != "" {
			out[k] = str
		}
	}
	return out
}

func buildTenantKeyService(db *gorm.DB) *tenantkeys.TenantKeyService {
	if db == nil {
		return nil
	}
	return tenantkeys.NewTenantKeyService(db)
}
