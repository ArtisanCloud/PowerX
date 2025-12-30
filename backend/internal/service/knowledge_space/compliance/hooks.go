package compliance

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// Guard enforces lightweight IAM + sensitivity gates for QA bridge.
type Guard struct{}

// NewGuard constructs a guard instance.
func NewGuard() *Guard {
	return &Guard{}
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
