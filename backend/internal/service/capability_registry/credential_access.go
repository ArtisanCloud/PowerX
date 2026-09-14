package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

// ServiceCredential identifies only already authenticated service contexts.
// User/admin authorization remains a separate policy, not an STS impersonation.
func ServiceCredential(ctx context.Context) bool {
	c := reqctx.GetClaims(ctx)
	return c != nil && (c.Issuer == "powerx-sts" || grantStatusContains(c.Platforms, "api_key"))
}

type CredentialAccess struct {
	Subject string
	Granted map[string]bool
}

type callerContextKey struct{}

// CurrentAccess is a live database evaluation, deliberately uncached so removed
// grants and registrations also affect previously issued STS tokens.
func (s *GrantStatusService) CurrentAccess(ctx context.Context) (CredentialAccess, error) {
	result := CredentialAccess{Granted: map[string]bool{}}
	if s == nil || s.db == nil {
		return result, &DirectGrantError{503}
	}
	c := reqctx.GetClaims(ctx)
	tenant, e := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if e != nil || c == nil || c.TenantUUID != tenant {
		return result, &DirectGrantError{401}
	}
	var allowed map[string]bool
	if grantStatusContains(c.Platforms, "api_key") {
		if reqctx.AuthenticatedAPIKeyHash(ctx) == "" {
			return result, &DirectGrantError{401}
		}
		key, e := gwrepo.NewIntegrationGatewayAPIKeyRepository(s.db).FindActiveByHash(ctx, tenant, reqctx.AuthenticatedAPIKeyHash(ctx))
		if errors.Is(e, gorm.ErrRecordNotFound) || e == nil && key == nil {
			return result, &DirectGrantError{401}
		}
		if e != nil {
			return result, &DirectGrantError{503}
		}
		result.Subject = "api_key:" + key.UUID.String()
		allowed, _, e = s.allowedForGatewayAPIKey(ctx, tenant)
		if e != nil {
			return result, &DirectGrantError{503}
		}
	} else {
		if c.Issuer != "powerx-sts" || !grantStatusContains(c.Audience, "powerx:api") || c.PluginID == "" || c.Subject == "" || c.ExpiresAt == nil || !time.Now().Before(c.ExpiresAt.Time) {
			return result, &DirectGrantError{401}
		}
		var row setting.PluginInstanceConfig
		e = s.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenant, c.PluginID, "auth.credentials", true).First(&row).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return result, &DirectGrantError{403}
		}
		if e != nil {
			return result, &DirectGrantError{503}
		}
		var credential struct {
			ClientID string   `json:"client_id"`
			Allowed  []string `json:"allowed_capabilities"`
		}
		if json.Unmarshal(row.ValueJSON, &credential) != nil {
			return result, &DirectGrantError{503}
		}
		if credential.ClientID == "" || c.Subject != "client:"+credential.ClientID {
			return result, &DirectGrantError{401}
		}
		result.Subject = "sts:" + c.PluginID + ":" + c.Subject
		allowed = map[string]bool{}
		for _, id := range credential.Allowed {
			allowed[id] = true
		}
	}
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return result, nil
	}
	facts, e := s.publishedAndRegistered(ctx, tenant, ids)
	if e != nil {
		return result, &DirectGrantError{503}
	}
	for _, id := range ids {
		result.Granted[id] = facts.published[id] && facts.registered[id]
	}
	return result, nil
}
func (a CredentialAccess) Require(id string) error {
	if !a.Granted[id] {
		return &DirectGrantError{403}
	}
	return nil
}
func callerSubject(ctx context.Context) string {
	v, _ := ctx.Value(callerContextKey{}).(string)
	return v
}
