package runtime_identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	settings "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type Request struct {
	Operation string `json:"operation"`
	PluginID  string `json:"plugin_id"`
}
type Invoker struct {
	db      *gorm.DB
	service *Service
}

func NewInvoker(db *gorm.DB, info CoreInfo) *Invoker {
	return &Invoker{db: db, service: NewService(info)}
}
func (i *Invoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != CapabilityID {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	invalid := func() (map[string]interface{}, error) {
		return nil, Error(http.StatusBadRequest, "RUNTIME_IDENTITY_INVALID_ARGUMENT")
	}
	if in.Method != "INVOKE" || in.Endpoint != "core://runtime/identity" || len(in.Query) != 0 {
		return invalid()
	}
	for key := range in.Payload {
		if key != "body" && key != "method" && key != "endpoint" {
			return invalid()
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil {
		return invalid()
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || request.Operation != "get" || !ValidPluginID(request.PluginID) {
		return invalid()
	}
	tenant, err := i.authorize(ctx, request.PluginID)
	if err != nil {
		return nil, err
	}
	if tenant != in.TenantUUID {
		return nil, Error(http.StatusForbidden, "RUNTIME_IDENTITY_FORBIDDEN")
	}
	item, err := i.service.Lookup(ctx, request.PluginID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"item": item}, nil
}
func (i *Invoker) authorize(ctx context.Context, pluginID string) (string, error) {
	forbidden := func() (string, error) { return "", Error(http.StatusForbidden, "RUNTIME_IDENTITY_FORBIDDEN") }
	upstream := func() (string, error) {
		return "", Error(http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE")
	}
	if i.db == nil {
		return upstream()
	}
	tenant, err := reqctx.RequireTenantUUID(ctx)
	if err != nil {
		return "", Error(http.StatusUnauthorized, "RUNTIME_IDENTITY_UNAUTHORIZED")
	}
	tenant, err = reqctx.CanonicalTenantUUID(tenant)
	if err != nil {
		return "", Error(http.StatusUnauthorized, "RUNTIME_IDENTITY_UNAUTHORIZED")
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil {
		return "", Error(http.StatusUnauthorized, "RUNTIME_IDENTITY_UNAUTHORIZED")
	}
	var record capmodel.CapabilityRecord
	if err = i.db.WithContext(ctx).Where("capability_id = ? AND status = ?", CapabilityID, "published").First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden()
		}
		return upstream()
	}
	var registration capmodel.CapabilityRegistration
	if err = i.db.WithContext(ctx).Where("capability_id = ? AND tenant_uuid = ?", CapabilityID, tenant).Order("version DESC").First(&registration).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden()
		}
		return upstream()
	}
	if registration.Status != "published" {
		return forbidden()
	}
	if contains(claims.Platforms, "api_key") {
		hash := reqctx.AuthenticatedAPIKeyHash(ctx)
		if hash == "" {
			return "", Error(http.StatusUnauthorized, "RUNTIME_IDENTITY_UNAUTHORIZED")
		}
		key, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(i.db).FindActiveByHash(ctx, tenant, hash)
		if err != nil {
			return upstream()
		}
		if key == nil {
			return "", Error(http.StatusUnauthorized, "RUNTIME_IDENTITY_UNAUTHORIZED")
		}
		// A blank or wildcard plugin binding must never authorize plugin enumeration.
		var permissions []gwmodel.IntegrationGatewayAPIKeyPermission
		if err = i.db.WithContext(ctx).Where("api_key_uuid = ? AND plugin_id = ?", key.UUID, pluginID).Find(&permissions).Error; err != nil {
			return upstream()
		}
		if !gwrepo.APIKeyPermissionGranted(permissions, APIKeyScope, "read", "capability", "runtime_identity_read") {
			return forbidden()
		}
		return tenant, nil
	}
	if claims.Issuer != "powerx-sts" || !contains(claims.Audience, "powerx:api") || claims.PluginID != pluginID {
		return forbidden()
	}
	var credential settings.PluginInstanceConfig
	if err = i.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenant, pluginID, "auth.credentials", true).First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return forbidden()
		}
		return upstream()
	}
	var grant struct {
		Allowed []string `json:"allowed_capabilities"`
	}
	if json.Unmarshal(credential.ValueJSON, &grant) != nil {
		return upstream()
	}
	if !contains(grant.Allowed, CapabilityID) {
		return forbidden()
	}
	return tenant, nil
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}
