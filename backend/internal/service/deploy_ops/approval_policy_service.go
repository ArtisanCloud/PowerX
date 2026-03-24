package deploy_ops

import (
	"context"
	"errors"
	"strings"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

var (
	ErrApprovalRequired = errors.New("approval required for this operation")
	ErrInvalidApproval  = errors.New("invalid approval policy")
)

type ApprovalPolicyService struct {
	repo *repoops.ApprovalPolicyProfileRepository
}

func NewApprovalPolicyService(db *gorm.DB) *ApprovalPolicyService {
	return &ApprovalPolicyService{repo: repoops.NewApprovalPolicyProfileRepository(db)}
}

func (s *ApprovalPolicyService) GetPolicy(ctx context.Context, environment string) (*modelops.ApprovalPolicyProfile, error) {
	environment = strings.TrimSpace(strings.ToLower(environment))
	if environment == "" {
		environment = "prod"
	}

	row, err := s.repo.FindByEnvironment(ctx, environment)
	if err != nil {
		return nil, err
	}
	if row != nil {
		return row, nil
	}

	return &modelops.ApprovalPolicyProfile{
		Environment:  environment,
		ApprovalMode: modelops.ApprovalModeNone,
		UpdatedBy:    "system",
	}, nil
}

func (s *ApprovalPolicyService) EnsureAllowed(ctx context.Context, environment string, approvalTickets int) error {
	policy, err := s.GetPolicy(ctx, environment)
	if err != nil {
		return err
	}

	switch policy.ApprovalMode {
	case modelops.ApprovalModeNone:
		return nil
	case modelops.ApprovalModeDualApproval:
		if approvalTickets >= 2 {
			return nil
		}
		return ErrApprovalRequired
	default:
		return ErrInvalidApproval
	}
}

func (s *ApprovalPolicyService) UpsertPolicy(ctx context.Context, environment string, mode modelops.ApprovalMode, operator string) (*modelops.ApprovalPolicyProfile, error) {
	row := &modelops.ApprovalPolicyProfile{
		Environment:  strings.TrimSpace(strings.ToLower(environment)),
		ApprovalMode: mode,
		UpdatedBy:    strings.TrimSpace(operator),
	}
	row.Normalize()
	return s.repo.UpsertByEnvironment(ctx, row)
}
