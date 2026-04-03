package deploy_ops

import (
	"context"
	"errors"
	"os"
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
	repo         *repoops.ApprovalPolicyProfileRepository
	defaultMode  modelops.ApprovalMode
	overrideMode map[string]modelops.ApprovalMode
}

func NewApprovalPolicyService(db *gorm.DB) *ApprovalPolicyService {
	return &ApprovalPolicyService{
		repo:         repoops.NewApprovalPolicyProfileRepository(db),
		defaultMode:  parseMode(os.Getenv("POWERX_APPROVAL_DEFAULT_MODE"), modelops.ApprovalModeNone),
		overrideMode: parseOverrides(os.Getenv("POWERX_APPROVAL_ENV_OVERRIDES")),
	}
}

func (s *ApprovalPolicyService) GetPolicy(ctx context.Context, environment string) (*modelops.ApprovalPolicyProfile, error) {
	environment = strings.TrimSpace(strings.ToLower(environment))
	if environment == "" {
		environment = "prod"
	}

	if mode, ok := s.overrideMode[environment]; ok {
		return &modelops.ApprovalPolicyProfile{
			Environment:  environment,
			ApprovalMode: mode,
			UpdatedBy:    "env_override",
		}, nil
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
		ApprovalMode: s.defaultMode,
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

func parseOverrides(raw string) map[string]modelops.ApprovalMode {
	out := make(map[string]modelops.ApprovalMode)
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) != 2 {
			continue
		}
		env := strings.TrimSpace(strings.ToLower(parts[0]))
		if env == "" {
			continue
		}
		mode := parseMode(parts[1], "")
		if mode == "" {
			continue
		}
		out[env] = mode
	}
	return out
}

func parseMode(raw string, fallback modelops.ApprovalMode) modelops.ApprovalMode {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case string(modelops.ApprovalModeNone):
		return modelops.ApprovalModeNone
	case string(modelops.ApprovalModeDualApproval):
		return modelops.ApprovalModeDualApproval
	default:
		return fallback
	}
}
