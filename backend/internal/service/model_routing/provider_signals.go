package model_routing

import (
	"context"
	"strings"

	providerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ProviderSignals captures health/cost hints for decision making.
type ProviderSignals struct {
	ProviderID  string
	HealthScore float64
	CostPerCall float64
}

// ProviderSignalSource fetches provider-level telemetry used by the decision engine.
type ProviderSignalSource interface {
	Fetch(ctx context.Context, providerID string) ProviderSignals
}

type repoSignalSource struct {
	repo *providerrepo.ProviderProfileRepository
}

// newRepoSignalSource builds a repository-backed signal provider.
func newRepoSignalSource(db *gorm.DB) ProviderSignalSource {
	if db == nil {
		return nil
	}
	return &repoSignalSource{repo: providerrepo.NewProviderProfileRepository(db)}
}

func (r *repoSignalSource) Fetch(ctx context.Context, providerID string) ProviderSignals {
	if r == nil || r.repo == nil {
		return ProviderSignals{ProviderID: providerID}
	}
	id, err := uuid.Parse(strings.TrimSpace(providerID))
	if err != nil {
		return ProviderSignals{ProviderID: providerID}
	}
	profile, err := r.repo.FindByUUID(ctx, id)
	if err != nil || profile == nil {
		return ProviderSignals{ProviderID: providerID}
	}
	return ProviderSignals{
		ProviderID:  providerID,
		HealthScore: profile.HealthScore,
	}
}
