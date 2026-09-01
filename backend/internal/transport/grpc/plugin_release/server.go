package plugin_release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/runtime"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/datatypes"
)

type server struct {
	pluginreleasepb.UnimplementedPluginReleaseServiceServer
	svc *svc.Service
}

// RegisterServer wires plugin release gRPC handlers.
func RegisterServer(registrar grpc.ServiceRegistrar, deps *shared.Deps) {
	if registrar == nil || deps == nil || deps.PluginReleaseService == nil {
		return
	}
	pluginreleasepb.RegisterPluginReleaseServiceServer(registrar, &server{
		svc: deps.PluginReleaseService,
	})
}

func (s *server) StartLocalInstall(ctx context.Context, req *pluginreleasepb.StartLocalInstallRequest) (*pluginreleasepb.LocalInstallSession, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}

	tenantUUID, err := s.resolveTenantUUID(ctx, "")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	session, err := localSvc.Start(ctx, local.StartInput{
		TenantUUID:          tenantUUID,
		DeveloperMemberUUID: reqctx.GetMemberUUID(ctx),
		ArtifactURI:         req.GetArtifactUri(),
		FeatureFlags:        req.GetFeatureFlags(),
		ResetCache:          req.GetResetCache(),
		Actor:               actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapLocalError(err)
	}

	return toProtoSession(session), nil
}

func (s *server) StopLocalInstall(ctx context.Context, req *pluginreleasepb.StopLocalInstallRequest) (*pluginreleasepb.StopLocalInstallResponse, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}
	tenantUUID, err := s.resolveTenantUUID(ctx, "")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	sessionUUID, err := parseSessionID(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := localSvc.Stop(ctx, local.StopInput{
		SessionID:  sessionUUID,
		TenantUUID: tenantUUID,
		Force:      req.GetForce(),
		Actor:      actorFromContext(ctx),
	}); err != nil {
		return nil, mapLocalError(err)
	}

	targetStatus := models.LocalInstallStatusSuccess
	if req.GetForce() {
		targetStatus = models.LocalInstallStatusFailed
	}

	return &pluginreleasepb.StopLocalInstallResponse{
		SessionId: req.GetSessionId(),
		Status:    targetStatus,
	}, nil
}

func (s *server) GetLocalInstallSession(ctx context.Context, req *pluginreleasepb.GetLocalInstallSessionRequest) (*pluginreleasepb.LocalInstallSession, error) {
	localSvc := s.localInstall()
	if localSvc == nil {
		return nil, status.Error(codes.Unavailable, "local install service unavailable")
	}
	sessionUUID, err := parseSessionID(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tenantUUID, err := s.resolveTenantUUID(ctx, "")
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	session, err := localSvc.Get(ctx, tenantUUID, sessionUUID)
	if err != nil {
		return nil, mapLocalError(err)
	}

	return toProtoSession(session), nil
}

func (s *server) PushHotReload(stream pluginreleasepb.PluginReleaseService_PushHotReloadServer) error {
	localSvc := s.localInstall()
	if localSvc == nil {
		return status.Error(codes.Unavailable, "local install service unavailable")
	}

	ctx := stream.Context()
	var (
		sessionUUID uuid.UUID
		sessionID   string
		lastSeq     int64
		ackStatus   = "accepted"
	)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			if sessionID == "" {
				return status.Error(codes.InvalidArgument, "no hot reload chunks received")
			}
			return stream.SendAndClose(&pluginreleasepb.HotReloadAck{
				SessionId:       sessionID,
				AppliedSequence: lastSeq,
				Status:          ackStatus,
				Message:         "",
			})
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive chunk failed: %v", err)
		}

		chunkSession := strings.TrimSpace(chunk.GetSessionId())
		if chunkSession == "" {
			return status.Error(codes.InvalidArgument, "session_id is required in stream")
		}
		if sessionID == "" {
			sessionID = chunkSession
			var parseErr error
			sessionUUID, parseErr = uuid.Parse(sessionID)
			if parseErr != nil {
				return status.Error(codes.InvalidArgument, "invalid session_id")
			}
			tenantUUID, tenantErr := s.resolveTenantUUID(ctx, "")
			if tenantErr != nil {
				return status.Error(codes.Unauthenticated, tenantErr.Error())
			}
			if _, err := localSvc.Get(ctx, tenantUUID, sessionUUID); err != nil {
				return mapLocalError(err)
			}
		} else if sessionID != chunkSession {
			return status.Error(codes.InvalidArgument, "session_id mismatch within stream")
		}

		lastSeq = chunk.GetSequence()
		pointers := map[string]any{
			"hotreload_last_sequence": lastSeq,
			"hotreload_updated_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}
		if chunk.GetChangelog() != "" {
			pointers["hotreload_changelog"] = chunk.GetChangelog()
		}
		if err := localSvc.UpdateLogPointers(ctx, sessionUUID, pointers); err != nil {
			return mapLocalError(err)
		}
		if chunk.GetEof() {
			ackStatus = "completed"
		}
	}
}

func (s *server) CreateReleaseCandidate(ctx context.Context, req *pluginreleasepb.CreateReleaseCandidateRequest) (*pluginreleasepb.ReleaseCandidate, error) {
	pipelineSvc := s.pipeline()
	if pipelineSvc == nil {
		return nil, status.Error(codes.Unavailable, "pipeline service unavailable")
	}
	tenantUUID, err := s.resolveTenantUUID(ctx, "")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if tenantUUID == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant uuid required")
	}
	input := pipeline.SubmitCandidateInput{
		TenantUUID:    tenantUUID,
		PluginID:      strings.TrimSpace(req.GetPluginId()),
		Version:       strings.TrimSpace(req.GetVersion()),
		BuildArtifact: strings.TrimSpace(req.GetBuildArtifactUri()),
		CommitHash:    strings.TrimSpace(req.GetCommitHash()),
		ReleaseNotes:  strings.TrimSpace(req.GetReleaseNotes()),
		Labels:        req.GetLabels(),
		Actor:         actorFromContext(ctx),
	}
	candidate, err := pipelineSvc.SubmitCandidate(ctx, input)
	if err != nil {
		return nil, mapPipelineError(err)
	}
	return toProtoCandidate(candidate), nil
}

func (s *server) RunQualityGates(ctx context.Context, req *pluginreleasepb.RunQualityGatesRequest) (*pluginreleasepb.GateResult, error) {
	pipelineSvc := s.pipeline()
	if pipelineSvc == nil {
		return nil, status.Error(codes.Unavailable, "pipeline service unavailable")
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(req.GetCandidateId()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid candidate_id")
	}
	result, err := pipelineSvc.RunQualityGates(ctx, pipeline.RunQualityGatesInput{
		CandidateID: candidateUUID,
		Actor:       actorFromContext(ctx),
		ForceRescan: req.GetForceRescan(),
	})
	if err != nil {
		return nil, mapPipelineError(err)
	}
	return &pluginreleasepb.GateResult{
		CandidateId: result.CandidateID.String(),
		Status:      result.Status,
		Violations:  toProtoViolations(result.Violations),
	}, nil
}

func (s *server) GenerateReleasePlan(ctx context.Context, req *pluginreleasepb.GenerateReleasePlanRequest) (*pluginreleasepb.ReleasePlan, error) {
	pipelineSvc := s.pipeline()
	if pipelineSvc == nil {
		return nil, status.Error(codes.Unavailable, "pipeline service unavailable")
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(req.GetCandidateId()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid candidate_id")
	}
	windowStart, err := time.Parse(time.RFC3339, req.GetWindowStart())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid window_start")
	}
	windowEnd, err := time.Parse(time.RFC3339, req.GetWindowEnd())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid window_end")
	}
	plan, candidate, err := pipelineSvc.GenerateReleasePlan(ctx, pipeline.GeneratePlanInput{
		CandidateID:         candidateUUID,
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
		CanaryBatches:       fromProtoBatches(req.GetBatches()),
		RollbackScripts:     req.GetRollbackScripts(),
		NotificationTargets: req.GetNotificationTargets(),
		DependencyList:      nil,
		Actor:               actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapPipelineError(err)
	}
	return toProtoPlan(plan, candidateUUID, candidate), nil
}

func (s *server) TriggerCanary(req *pluginreleasepb.TriggerCanaryRequest, stream pluginreleasepb.PluginReleaseService_TriggerCanaryServer) error {
	runtimeSvc := s.runtime()
	if runtimeSvc == nil {
		return status.Error(codes.Unavailable, "runtime service unavailable")
	}
	planID, err := parsePlanID(req.GetPlanId())
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	events, err := runtimeSvc.TriggerCanary(stream.Context(), runtime.TriggerCanaryInput{
		PlanID:    planID,
		BatchName: strings.TrimSpace(req.GetBatchName()),
		Actor:     actorFromContext(stream.Context()),
	})
	if err != nil {
		return mapRuntimeError(err)
	}
	for _, evt := range events {
		if err := stream.Send(&pluginreleasepb.CanaryProgress{
			PlanId:            strconv.FormatUint(evt.PlanID, 10),
			BatchName:         evt.BatchName,
			Phase:             evt.Phase,
			ErrorRate:         evt.ErrorRate,
			LatencyP95Ms:      evt.LatencyP95MS,
			ThresholdBreached: evt.ThresholdBreached,
		}); err != nil {
			return status.Errorf(codes.Internal, "send canary progress failed: %v", err)
		}
	}
	return nil
}

func (s *server) FinalizeDeployment(ctx context.Context, req *pluginreleasepb.FinalizeDeploymentRequest) (*pluginreleasepb.FinalizeDeploymentResponse, error) {
	runtimeSvc := s.runtime()
	if runtimeSvc == nil {
		return nil, status.Error(codes.Unavailable, "runtime service unavailable")
	}
	planID, err := parsePlanID(req.GetPlanId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	plan, err := runtimeSvc.FinalizeDeployment(ctx, runtime.FinalizeInput{
		PlanID: planID,
		Action: req.GetAction(),
		Actor:  actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapRuntimeError(err)
	}
	return &pluginreleasepb.FinalizeDeploymentResponse{
		PlanId:     strconv.FormatUint(plan.ID, 10),
		FinalState: plan.Status,
	}, nil
}

func (s *server) UploadOfflinePackage(stream pluginreleasepb.PluginReleaseService_UploadOfflinePackageServer) error {
	distSvc := s.distribution()
	if distSvc == nil {
		return status.Error(codes.Unavailable, "distribution service unavailable")
	}
	ctx := stream.Context()
	var (
		candidateUUID uuid.UUID
		checksum      string
		buffer        bytes.Buffer
	)
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "receive offline package chunk failed: %v", err)
		}
		if candidateUUID == uuid.Nil {
			value := strings.TrimSpace(chunk.GetCandidateId())
			if value == "" {
				return status.Error(codes.InvalidArgument, "candidate_id is required")
			}
			candidateUUID, err = uuid.Parse(value)
			if err != nil {
				return status.Error(codes.InvalidArgument, "invalid candidate_id")
			}
		}
		if len(chunk.GetChunk()) > 0 {
			if _, err := buffer.Write(chunk.GetChunk()); err != nil {
				return status.Errorf(codes.Internal, "buffer chunk failed: %v", err)
			}
		}
		if trimmed := strings.TrimSpace(chunk.GetChecksum()); trimmed != "" {
			checksum = trimmed
		}
		if chunk.GetEof() {
			break
		}
	}
	if candidateUUID == uuid.Nil {
		return status.Error(codes.InvalidArgument, "candidate_id is required in stream")
	}
	if strings.TrimSpace(checksum) == "" {
		return status.Error(codes.InvalidArgument, "checksum is required in stream")
	}

	pkg, err := distSvc.StoreOfflinePackage(ctx, distribution.StoreOfflinePackageInput{
		CandidateID: candidateUUID,
		Content:     buffer.Bytes(),
		Checksum:    checksum,
		Actor:       actorFromContext(ctx),
	})
	if err != nil {
		return mapDistributionError(err)
	}
	return stream.SendAndClose(&pluginreleasepb.UploadOfflinePackageResponse{
		OfflinePackageId: strconv.FormatUint(pkg.ID, 10),
		PackageUri:       pkg.PackageURI,
	})
}

func (s *server) SubmitMarketplaceListing(ctx context.Context, req *pluginreleasepb.SubmitMarketplaceListingRequest) (*pluginreleasepb.MarketplaceListing, error) {
	distSvc := s.distribution()
	if distSvc == nil {
		return nil, status.Error(codes.Unavailable, "distribution service unavailable")
	}
	packageID, err := strconv.ParseUint(strings.TrimSpace(req.GetOfflinePackageId()), 10, 64)
	if err != nil || packageID == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid offline_package_id")
	}
	listing, err := distSvc.SubmitListing(ctx, distribution.SubmitListingInput{
		OfflinePackageID: packageID,
		Channel:          strings.TrimSpace(req.GetChannel()),
		Pricing:          decodeJSONMapString(req.GetPricingJson()),
		SupportPolicy:    decodeJSONMapString(req.GetSupportPolicyJson()),
		SubmissionForm:   nil,
		Actor:            actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapDistributionError(err)
	}
	return &pluginreleasepb.MarketplaceListing{
		ListingId:    strconv.FormatUint(listing.ID, 10),
		Channel:      listing.Channel,
		ReviewStatus: listing.ReviewStatus,
		ReviewCount:  uint32(listing.ReviewCount),
	}, nil
}

func (s *server) ImportOfflinePackage(ctx context.Context, req *pluginreleasepb.ImportOfflinePackageRequest) (*pluginreleasepb.ImportOfflinePackageResponse, error) {
	distSvc := s.distribution()
	if distSvc == nil {
		return nil, status.Error(codes.Unavailable, "distribution service unavailable")
	}
	tenantUUID, err := s.resolveTenantUUID(ctx, "")
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if tenantUUID == "" {
		return nil, status.Error(codes.InvalidArgument, "tenant uuid required")
	}
	job, err := distSvc.StartOfflineImport(ctx, distribution.OfflineImportInput{
		TenantUUID:      tenantUUID,
		PackageURI:      strings.TrimSpace(req.GetPackageUri()),
		Checksum:        strings.TrimSpace(req.GetChecksum()),
		DryRun:          req.GetDryRun(),
		LicenseAccepted: true,
		Actor:           actorFromContext(ctx),
	})
	if err != nil {
		return nil, mapDistributionError(err)
	}
	return &pluginreleasepb.ImportOfflinePackageResponse{
		JobId:  job.ID,
		Status: job.Status,
	}, nil
}

func (s *server) localInstall() *local.InstallService {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.LocalInstall()
}

func (s *server) resolveTenantUUID(ctx context.Context, payload string) (string, error) {
	if canonical, err := s.optionalTenantUUID(ctx, payload); err != nil {
		return "", err
	} else if canonical != "" {
		return canonical, nil
	}
	if ctxUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx)); ctxUUID != "" {
		canonical, err := reqctx.CanonicalTenantUUID(ctxUUID)
		if err != nil {
			return "", fmt.Errorf("tenant uuid invalid")
		}
		return canonical, nil
	}
	return "", errors.New("tenant uuid required")
}

func (s *server) optionalTenantUUID(_ context.Context, candidate string) (string, error) {
	trimmed := strings.TrimSpace(candidate)
	if trimmed == "" {
		return "", nil
	}
	canonical, err := reqctx.CanonicalTenantUUID(trimmed)
	if err != nil {
		return "", fmt.Errorf("tenant uuid invalid")
	}
	return canonical, nil
}

func (s *server) pipeline() *pipeline.Service {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Pipeline()
}

func (s *server) runtime() *runtime.Service {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Runtime()
}

func (s *server) distribution() *distribution.Service {
	if s == nil || s.svc == nil {
		return nil
	}
	return s.svc.Distribution()
}

func parseSessionID(value string) (uuid.UUID, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return uuid.Nil, fmt.Errorf("session_id is required")
	}
	sessionUUID, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid session_id")
	}
	return sessionUUID, nil
}

func parsePlanID(value string) (uint64, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("plan_id is required")
	}
	return strconv.ParseUint(trimmed, 10, 64)
}

func actorFromContext(ctx context.Context) string {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("authorization"); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func mapLocalError(err error) error {
	switch {
	case errors.Is(err, local.ErrFeatureDisabled):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, local.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, local.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, local.ErrSignatureInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, local.ErrActiveSession):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, local.ErrArtifactTooLarge):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, local.ErrSessionNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "grpc.plugin_release"}), "local install operation failed: %v", err)
		return status.Error(codes.Internal, "local install operation failed")
	}
}

func toProtoSession(session *models.LocalInstallSession) *pluginreleasepb.LocalInstallSession {
	if session == nil {
		return nil
	}
	resp := &pluginreleasepb.LocalInstallSession{
		SessionId:           session.UUID.String(),
		TenantUuid:          strings.TrimSpace(session.TenantUUID),
		DeveloperMemberUuid: session.DeveloperMemberUUID,
		ArtifactUri:         session.ArtifactURI,
		FeatureFlags: func() []string {
			flags := local.ExtractFeatureFlags(session.FeatureFlags)
			if flags == nil {
				return []string{}
			}
			return flags
		}(),
		Status: session.Status,
		LogUrl: extractLogURL(session.LogPointers),
	}
	if !session.CreatedAt.IsZero() {
		resp.CreatedAt = session.CreatedAt.UTC().Format(time.RFC3339)
	}
	if session.ExpiredAt != nil {
		resp.ExpiresAt = session.ExpiredAt.UTC().Format(time.RFC3339)
	}
	return resp
}

func toProtoCandidate(candidate *models.PluginReleaseCandidate) *pluginreleasepb.ReleaseCandidate {
	if candidate == nil {
		return nil
	}
	resp := &pluginreleasepb.ReleaseCandidate{
		CandidateId:      candidate.UUID.String(),
		TenantUuid:       candidate.TenantUUID,
		PluginId:         candidate.PluginID,
		Version:          candidate.Version,
		GateStatus:       candidate.GateStatus,
		ApprovalStatus:   candidate.ApprovalStatus,
		BuildArtifactUri: candidate.BuildArtifactURI,
		ReleaseNotes:     candidate.ReleaseNotes,
	}
	return resp
}

func toProtoPlan(plan *models.ReleasePlan, candidateUUID uuid.UUID, candidate *models.PluginReleaseCandidate) *pluginreleasepb.ReleasePlan {
	if plan == nil {
		return nil
	}
	candidateID := candidateUUID.String()
	if candidate != nil {
		candidateID = candidate.UUID.String()
	}
	return &pluginreleasepb.ReleasePlan{
		PlanId:      strconv.FormatUint(plan.ID, 10),
		CandidateId: candidateID,
		Status:      plan.Status,
		Batches:     decodeProtoBatches(plan.CanaryBatches),
	}
}

func toProtoViolations(violations []pipeline.GateViolation) []*pluginreleasepb.GateViolation {
	if len(violations) == 0 {
		return nil
	}
	result := make([]*pluginreleasepb.GateViolation, 0, len(violations))
	for _, v := range violations {
		result = append(result, &pluginreleasepb.GateViolation{
			Code:    v.Code,
			Message: v.Message,
			Owner:   v.Owner,
		})
	}
	return result
}

func fromProtoBatches(batches []*pluginreleasepb.CanaryBatch) []pipeline.CanaryBatchInput {
	if len(batches) == 0 {
		return nil
	}
	result := make([]pipeline.CanaryBatchInput, 0, len(batches))
	for _, batch := range batches {
		if batch == nil {
			continue
		}
		result = append(result, pipeline.CanaryBatchInput{
			Name:                batch.GetName(),
			TenantScope:         batch.GetTenantScope(),
			MetricThresholds:    batch.GetMetricThresholds(),
			RollbackTimeoutMins: batch.GetRollbackTimeoutMinutes(),
		})
	}
	return result
}

func decodeProtoBatches(raw datatypes.JSON) []*pluginreleasepb.CanaryBatch {
	if len(raw) == 0 {
		return nil
	}
	var payload []struct {
		Name                string             `json:"name"`
		TenantScope         []string           `json:"tenant_scope"`
		MetricThresholds    map[string]float64 `json:"metric_thresholds"`
		RollbackTimeoutMins uint32             `json:"rollback_timeout_mins"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	result := make([]*pluginreleasepb.CanaryBatch, 0, len(payload))
	for _, item := range payload {
		result = append(result, &pluginreleasepb.CanaryBatch{
			Name:                   item.Name,
			TenantScope:            item.TenantScope,
			MetricThresholds:       item.MetricThresholds,
			RollbackTimeoutMinutes: item.RollbackTimeoutMins,
		})
	}
	return result
}

func decodeJSONMapString(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload
}

func mapPipelineError(err error) error {
	switch {
	case errors.Is(err, pipeline.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, pipeline.ErrCandidateNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, pipeline.ErrPlanWindowInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, pipeline.ErrGateNotPassed):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "grpc.plugin_release"}), "pipeline operation failed: %v", err)
		return status.Error(codes.Internal, "pipeline operation failed")
	}
}

func mapRuntimeError(err error) error {
	switch {
	case errors.Is(err, runtime.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, runtime.ErrPlanNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, runtime.ErrBatchNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "grpc.plugin_release"}), "runtime operation failed: %v", err)
		return status.Error(codes.Internal, "runtime operation failed")
	}
}

func mapDistributionError(err error) error {
	switch {
	case errors.Is(err, distribution.ErrFeatureDisabled):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, distribution.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, distribution.ErrCandidateNotFound),
		errors.Is(err, distribution.ErrPackageNotFound),
		errors.Is(err, distribution.ErrListingNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		logger.WarnF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "grpc.plugin_release"}), "distribution operation failed: %v", err)
		return status.Error(codes.Internal, "distribution operation failed")
	}
}

func extractLogURL(raw datatypes.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if v, ok := payload["log_url"].(string); ok && v != "" {
		return v
	}
	if v, ok := payload["logUrl"].(string); ok && v != "" {
		return v
	}
	return ""
}
