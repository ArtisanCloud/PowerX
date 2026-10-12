package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DirectGrantError struct{ Status int }

func (e *DirectGrantError) HTTPStatus() int { return e.Status }
func (e *DirectGrantError) Error() string {
	switch e.Status {
	case 401:
		return "CAPABILITY_UNAUTHORIZED"
	case 403:
		return "CAPABILITY_FORBIDDEN"
	default:
		return "CAPABILITY_UPSTREAM_DEPENDENCY"
	}
}

type DirectGrantService struct {
	repo *repository.DirectGrantRepository
}

func NewDirectGrantService(db *gorm.DB) *DirectGrantService {
	if db == nil {
		return &DirectGrantService{}
	}
	return &DirectGrantService{repo: repository.NewDirectGrantRepository(db)}
}
func (s *DirectGrantService) AuthorizeSTS(ctx context.Context, capabilityID string) error {
	if s == nil || s.repo == nil {
		return &DirectGrantError{503}
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.PluginID == "" || claims.Subject == "" || claims.ExpiresAt == nil || !time.Now().Before(claims.ExpiresAt.Time) {
		return &DirectGrantError{401}
	}
	audience := false
	for _, v := range claims.Audience {
		if v == "powerx:api" {
			audience = true
		}
	}
	tenant, err := uuid.Parse(claims.TenantUUID)
	if !audience || err != nil || tenant == uuid.Nil || reqctx.GetTenantUUID(ctx) != claims.TenantUUID {
		return &DirectGrantError{401}
	}
	eligible, raw, err := s.repo.Facts(ctx, tenant.String(), claims.PluginID, capabilityID)
	if errors.Is(err, gorm.ErrRecordNotFound) || !eligible && err == nil {
		return &DirectGrantError{403}
	}
	if err != nil {
		return &DirectGrantError{503}
	}
	var credential struct {
		ClientID string   `json:"client_id"`
		Allowed  []string `json:"allowed_capabilities"`
	}
	if json.Unmarshal(raw, &credential) != nil {
		return &DirectGrantError{503}
	}
	if credential.ClientID == "" || claims.Subject != "client:"+credential.ClientID {
		return &DirectGrantError{401}
	}
	for _, allowed := range credential.Allowed {
		if allowed == capabilityID {
			return nil
		}
	}
	return &DirectGrantError{403}
}
