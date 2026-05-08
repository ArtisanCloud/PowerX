package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	repoagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"gorm.io/gorm"
)

var (
	ErrContextRefForbidden = errors.New("agent.context_ref_forbidden")
	ErrContextRefNotFound  = errors.New("agent.context_ref_not_found")
)

type ContextRefService struct {
	repo *repoagent.AgentSharedContextRefRepository
	now  func() time.Time
}

func NewContextRefService(db *gorm.DB) *ContextRefService {
	if db == nil {
		return &ContextRefService{}
	}
	return &ContextRefService{
		repo: repoagent.NewAgentSharedContextRefRepository(db),
		now:  time.Now,
	}
}

func (s *ContextRefService) CanAccess(ctx context.Context, tenantUUID string, childAgentID uint64, contextRefID string) error {
	if strings.TrimSpace(contextRefID) == "" {
		return nil
	}
	if s == nil || s.repo == nil {
		return ErrContextRefForbidden
	}
	err := s.repo.ValidateAccess(ctx, tenantUUID, childAgentID, contextRefID, s.now())
	if err == nil {
		return nil
	}
	if errors.Is(err, repoagent.ErrContextRefNotFound) {
		return ErrContextRefNotFound
	}
	return ErrContextRefForbidden
}
