package knowledge_space

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	tenant_release "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/tenant_release"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func mapReleaseError(err error) error {
	switch {
	case errors.Is(err, tenant_release.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, tenant_release.ErrPolicyNotFound), errors.Is(err, tenant_release.ErrBatchNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, tenant_release.ErrBatchPaused):
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func fromProtoBatches(items []*knowledgev1.ReleaseBatch) []tenant_release.BatchSpec {
	if len(items) == 0 {
		return nil
	}
	result := make([]tenant_release.BatchSpec, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		result = append(result, tenant_release.BatchSpec{
			Name:    item.GetName(),
			Tenants: item.GetTenants(),
		})
	}
	return result
}

func parsePolicyID(raw string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
}

func (s *Server) UpsertReleasePolicy(ctx context.Context, req *knowledgev1.UpsertReleasePolicyRequest) (*knowledgev1.UpsertReleasePolicyResponse, error) {
	if s.release == nil {
		return nil, status.Error(codes.Unimplemented, "release service not available")
	}
	policy, err := s.release.UpsertPolicy(ctx, tenant_release.UpsertPolicyInput{
		MatrixVersion: req.GetMatrixVersion(),
		PilotTenants:  req.GetPilotTenants(),
		Batches:       fromProtoBatches(req.GetBatches()),
		Guardrails:    req.GetGuardrails(),
		ApprovedBy:    req.GetApprovedBy(),
		CreatedBy:     req.GetCreatedBy(),
	})
	if err != nil {
		return nil, mapReleaseError(err)
	}
	return &knowledgev1.UpsertReleasePolicyResponse{
		PolicyId: fmt.Sprintf("%d", policy.ID),
		Status:   policy.Status,
	}, nil
}

func (s *Server) PublishRelease(ctx context.Context, req *knowledgev1.PublishReleaseRequest) (*knowledgev1.PublishReleaseResponse, error) {
	if s.release == nil {
		return nil, status.Error(codes.Unimplemented, "release service not available")
	}
	policyID, err := parsePolicyID(req.GetPolicyId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid policy id: %v", err)
	}
	res, err := s.release.Publish(ctx, tenant_release.PublishInput{
		PolicyID:    policyID,
		VersionID:   req.GetVersionId(),
		RequestedBy: req.GetRequestedBy(),
	})
	if err != nil {
		return nil, mapReleaseError(err)
	}
	return &knowledgev1.PublishReleaseResponse{
		ReleaseId:  res.ReleaseID,
		VersionId:  res.VersionID,
		BatchToken: res.BatchToken,
		BatchIndex: int32(res.BatchIndex),
		Tenants:    res.Tenants,
	}, nil
}

func (s *Server) PromoteRelease(ctx context.Context, req *knowledgev1.PromoteReleaseRequest) (*knowledgev1.PromoteReleaseResponse, error) {
	if s.release == nil {
		return nil, status.Error(codes.Unimplemented, "release service not available")
	}
	policyID, err := parsePolicyID(req.GetPolicyId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid policy id: %v", err)
	}
	result, serr := s.release.Promote(ctx, tenant_release.PromoteInput{
		PolicyID:    policyID,
		VersionID:   req.GetVersionId(),
		BatchToken:  req.GetBatchToken(),
		Alerts:      req.GetAlerts(),
		RequestedBy: req.GetRequestedBy(),
	})
	if serr != nil && !errors.Is(serr, tenant_release.ErrBatchPaused) {
		return nil, mapReleaseError(serr)
	}
	return &knowledgev1.PromoteReleaseResponse{
		NextBatchToken: result.BatchToken,
		BatchIndex:     int32(result.BatchIndex),
		Tenants:        result.Tenants,
		State:          result.State,
		TenantCoverage: result.TenantCoverage,
	}, nil
}

func (s *Server) RollbackRelease(ctx context.Context, req *knowledgev1.RollbackReleaseRequest) (*knowledgev1.RollbackReleaseResponse, error) {
	if s.release == nil {
		return nil, status.Error(codes.Unimplemented, "release service not available")
	}
	policyID, err := parsePolicyID(req.GetPolicyId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid policy id: %v", err)
	}
	res, err := s.release.Rollback(ctx, tenant_release.RollbackInput{
		PolicyID:    policyID,
		VersionID:   req.GetVersionId(),
		Reason:      req.GetReason(),
		RequestedBy: req.GetRequestedBy(),
	})
	if err != nil {
		return nil, mapReleaseError(err)
	}
	return &knowledgev1.RollbackReleaseResponse{Status: res.Status}, nil
}

