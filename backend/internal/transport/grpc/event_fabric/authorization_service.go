package eventfabric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	authorizationpb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authorizationservice "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthorizationServer 提供授权评估 gRPC 接口。
type AuthorizationServer struct {
	authorizationpb.UnimplementedAuthorizationServiceServer
	service authorizationservice.Service
}

// NewAuthorizationServer 构建授权服务实例。
func NewAuthorizationServer(svc authorizationservice.Service) *AuthorizationServer {
	return &AuthorizationServer{service: svc}
}

// Evaluate 执行授权评估。
func (s *AuthorizationServer) Evaluate(ctx context.Context, req *authorizationpb.EvaluateRequest) (*authorizationpb.EvaluateResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization service unavailable")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request payload required")
	}

	tenantID, err := parseUUID(req.GetTenant().GetTenantUuid(), "tenant_uuid")
	if err != nil {
		return nil, err
	}
	subjectID, err := parseUUID(req.GetSubject().GetId(), "subject.id")
	if err != nil {
		return nil, err
	}
	subjectType, err := toSubjectType(req.GetSubject().GetType())
	if err != nil {
		return nil, err
	}

	evalReq := authorizationservice.EvaluateRequest{
		TenantID:    tenantID,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Capability:  req.GetCapability(),
		Resource:    req.GetResource(),
		ContextTags: append([]string(nil), req.GetContextTags()...),
		Attributes:  toStringMap(req.GetAttributes()),
		RequestID:   strings.TrimSpace(req.GetRequestId()),
	}

	result, err := s.service.Evaluate(ctx, evalReq)
	if err != nil {
		return nil, mapServiceError("evaluate", err)
	}

	resp := &authorizationpb.EvaluateResponse{
		Decision:     toProtoDecision(result.Decision),
		Reason:       result.Reason,
		GrantVersion: formatVersion(result.GrantVersion),
		AuditEventId: result.AuditEventID,
		Challenge:    toProtoChallenge(result.Challenge),
	}
	return resp, nil
}

// InvalidateGrantCache 主动失效缓存。
func (s *AuthorizationServer) InvalidateGrantCache(ctx context.Context, req *authorizationpb.InvalidateGrantCacheRequest) (*authorizationpb.InvalidateGrantCacheResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization service unavailable")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request payload required")
	}

	tenantID, err := parseUUID(req.GetTenant().GetTenantUuid(), "tenant_uuid")
	if err != nil {
		return nil, err
	}
	subjectID, err := parseUUID(req.GetSubject().GetId(), "subject.id")
	if err != nil {
		return nil, err
	}
	subjectType, err := toSubjectType(req.GetSubject().GetType())
	if err != nil {
		return nil, err
	}

	key := authorizationservice.GrantCacheKey{
		TenantUUID:  tenantID.String(),
		SubjectType: subjectType,
		SubjectID:   subjectID,
	}
	if err := s.service.InvalidateGrantCache(ctx, key); err != nil {
		return nil, mapServiceError("invalidate_cache", err)
	}

	return &authorizationpb.InvalidateGrantCacheResponse{
		RequestId: uuid.NewString(),
		Status:    "accepted",
	}, nil
}

// GetGrantSnapshot 返回授权快照。
func (s *AuthorizationServer) GetGrantSnapshot(ctx context.Context, req *authorizationpb.GetGrantSnapshotRequest) (*authorizationpb.GetGrantSnapshotResponse, error) {
	if s.service == nil {
		return nil, status.Error(codes.FailedPrecondition, "authorization service unavailable")
	}
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request payload required")
	}

	tenantID, err := parseUUID(req.GetTenant().GetTenantUuid(), "tenant_uuid")
	if err != nil {
		return nil, err
	}
	subjectID, err := parseUUID(req.GetSubject().GetId(), "subject.id")
	if err != nil {
		return nil, err
	}
	subjectType, err := toSubjectType(req.GetSubject().GetType())
	if err != nil {
		return nil, err
	}

	snapshot, err := s.service.GetGrantSnapshot(ctx, tenantID, subjectType, subjectID)
	if err != nil {
		return nil, mapServiceError("get_snapshot", err)
	}
	return &authorizationpb.GetGrantSnapshotResponse{Grant: toProtoGrant(snapshot)}, nil
}

// RegisterAuthorizationServer 返回注册函数。
func RegisterAuthorizationServer(deps *shared.Deps) Registrar {
	return func(server grpc.ServiceRegistrar) {
		if deps == nil || deps.EventFabric == nil || deps.EventFabric.Authorization == nil {
			return
		}
		svc := deps.EventFabric.Authorization.Service
		if svc == nil {
			return
		}
		authorizationpb.RegisterAuthorizationServiceServer(server, NewAuthorizationServer(svc))
	}
}

func mapServiceError(operation string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, authorizationservice.ErrServiceUnavailable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, authorizationservice.ErrGrantNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, authorizationservice.ErrCapabilityNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "invalid") || strings.Contains(msg, "required") || strings.Contains(msg, "unsupported") {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		return status.Errorf(codes.Internal, "%s failure: %v", operation, err)
	}
}

func parseUUID(value, field string) (uuid.UUID, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "%s is required", field)
	}
	parsed, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, status.Errorf(codes.InvalidArgument, "invalid %s: %v", field, err)
	}
	return parsed, nil
}

func toSubjectType(subject authorizationpb.SubjectType) (string, error) {
	switch subject {
	case authorizationpb.SubjectType_SUBJECT_TYPE_AGENT:
		return authorizationservice.SubjectTypeAgent, nil
	case authorizationpb.SubjectType_SUBJECT_TYPE_PLUGIN:
		return authorizationservice.SubjectTypePlugin, nil
	default:
		return "", status.Errorf(codes.InvalidArgument, "unsupported subject type %s", subject.String())
	}
}

func toStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toProtoDecision(decision string) authorizationpb.Decision {
	switch strings.ToLower(decision) {
	case authorizationservice.DecisionAllow:
		return authorizationpb.Decision_DECISION_ALLOW
	case authorizationservice.DecisionChallenge:
		return authorizationpb.Decision_DECISION_CHALLENGE
	case authorizationservice.DecisionBlock:
		return authorizationpb.Decision_DECISION_BLOCK
	default:
		return authorizationpb.Decision_DECISION_UNSPECIFIED
	}
}

func toProtoChallenge(info *authorizationservice.ChallengeInfo) *authorizationpb.ChallengeTicket {
	if info == nil {
		return nil
	}
	return &authorizationpb.ChallengeTicket{
		TicketId:     info.TicketID.String(),
		Status:       info.Status,
		SlaExpiresAt: info.SLAExpiresAt.UTC().Format(time.RFC3339),
	}
}

func toProtoGrant(snapshot *authorizationservice.GrantSnapshot) *authorizationpb.Grant {
	if snapshot == nil {
		return nil
	}
	grant := &authorizationpb.Grant{
		Id:           snapshot.GrantID.String(),
		Status:       snapshot.Status,
		Source:       snapshot.Source,
		Version:      formatVersion(snapshot.Version),
		TtlExpiresAt: formatTime(snapshot.TTLExpires),
		Conditions:   toProtoConditions(snapshot.Conditions),
	}
	for _, cap := range snapshot.Capabilities {
		capabilityName := fmt.Sprintf("%s.%s", cap.Namespace, cap.Action)
		grant.Capabilities = append(grant.Capabilities, &authorizationpb.GrantCapability{
			Capability: capabilityName,
			RateLimit:  toProtoRateLimit(cap),
		})
	}
	return grant
}

func toProtoConditions(conditions authorizationservice.ConditionsSnapshot) *authorizationpb.GrantConditions {
	if len(conditions.Resources) == 0 && len(conditions.ContextTags) == 0 && conditions.TimeWindow == nil {
		return nil
	}
	out := &authorizationpb.GrantConditions{
		Resources:   append([]string(nil), conditions.Resources...),
		ContextTags: append([]string(nil), conditions.ContextTags...),
	}
	if conditions.TimeWindow != nil {
		out.TimeWindow = &authorizationpb.TimeWindow{
			Start: conditions.TimeWindow.Start.UTC().Format(time.RFC3339),
			End:   conditions.TimeWindow.End.UTC().Format(time.RFC3339),
		}
	}
	return out
}

func toProtoRateLimit(cap authorizationservice.CapabilitySnapshot) *authorizationpb.RateLimit {
	if rate := mapToRateLimit(cap.CustomRateLimit); rate != nil {
		return rate
	}
	return mapToRateLimit(cap.DefaultRateLimit)
}

func mapToRateLimit(raw map[string]any) *authorizationpb.RateLimit {
	if len(raw) == 0 {
		return nil
	}
	limit := toUint(raw["limit"])
	if limit == 0 {
		return nil
	}
	interval := toUint(raw["interval_seconds"])
	burst := toUint(raw["burst"])
	return &authorizationpb.RateLimit{
		Limit:           uint32(limit),
		IntervalSeconds: uint32(interval),
		Burst:           uint32(burst),
	}
}

func toUint(value any) uint64 {
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil && i > 0 {
			return uint64(i)
		}
	case float64:
		if v > 0 {
			return uint64(v)
		}
	case float32:
		if v > 0 {
			return uint64(v)
		}
	case int64:
		if v > 0 {
			return uint64(v)
		}
	case int:
		if v > 0 {
			return uint64(v)
		}
	case uint64:
		return v
	case uint32:
		return uint64(v)
	}
	return 0
}

func formatVersion(version uint64) string {
	if version == 0 {
		return ""
	}
	return fmt.Sprintf("%d", version)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
