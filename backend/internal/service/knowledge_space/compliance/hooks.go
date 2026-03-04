package compliance

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// Guard enforces lightweight IAM + sensitivity gates for QA bridge.
type Guard struct {
	MustCiteSources   bool
	MinEvidenceChunks int
}

// NewGuard constructs a guard instance.
func NewGuard() *Guard {
	return &Guard{
		MustCiteSources:   true,
		MinEvidenceChunks: 1,
	}
}

// Evaluate returns a degrade reason if the request should not access the space.
func (g *Guard) Evaluate(tenant uuid.UUID, space *models.KnowledgeSpace) string {
	if space == nil {
		return "space_not_found"
	}
	if !strings.EqualFold(strings.TrimSpace(space.TenantUUID), strings.TrimSpace(tenant.String())) {
		return "tenant_mismatch"
	}
	if space.Status != models.KnowledgeSpaceStatusActive {
		return fmt.Sprintf("status=%s", space.Status)
	}
	return ""
}

// CheckGuardrails validates citation/evidence requirements.
func (g *Guard) CheckGuardrails(citationChunks int) string {
	if g == nil {
		return ""
	}
	if g.MustCiteSources && citationChunks <= 0 {
		return "must_cite_sources"
	}
	if g.MinEvidenceChunks > 0 && citationChunks < g.MinEvidenceChunks {
		return fmt.Sprintf("min_evidence_chunks=%d", g.MinEvidenceChunks)
	}
	return ""
}
