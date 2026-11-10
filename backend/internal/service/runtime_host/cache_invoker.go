package runtime_host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const CacheServiceContract = "cache.service.v1"
const CacheServiceEndpoint = "core://runtime/cache"
const CacheServiceRead = "com.corex.runtime.cache.service_read"
const CacheServiceManage = "com.corex.runtime.cache.service_manage"

type CacheOperation struct {
	ContractVersion string  `json:"contract_version"`
	TenantKey       string  `json:"tenant_key"`
	Operation       string  `json:"operation"`
	Namespace       string  `json:"namespace"`
	Key             string  `json:"key"`
	Value           *string `json:"value_base64,omitempty"`
	TTL             *int64  `json:"ttl_ms,omitempty"`
}

type CacheInvoker struct {
	db      *gorm.DB
	service *Service
}

func NewCacheInvoker(db *gorm.DB, service *Service) *CacheInvoker { return &CacheInvoker{db, service} }

func cacheInvokeError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}

// InvokeCoreCapability accepts only the published typed operation. The caller
// identity and live grant are resolved again; body/header fields cannot pick it.
func (i *CacheInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	if in.CapabilityID != CacheServiceRead && in.CapabilityID != CacheServiceManage {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if i == nil || i.db == nil || i.service == nil {
		return nil, cacheInvokeError(503, "CACHE_UPSTREAM_DEPENDENCY")
	}
	if in.Method != "INVOKE" || in.Endpoint != CacheServiceEndpoint || len(in.Query) != 0 || len(in.Context) != 0 {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	for k := range in.Payload {
		if k != "body" {
			return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil || len(raw) > 2<<20 {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	var op CacheOperation
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&op) != nil || d.Decode(&struct{}{}) != io.EOF || op.ContractVersion != CacheServiceContract {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	if (op.Operation != "get" && op.Operation != "set" && op.Operation != "delete") || (op.Operation == "set" && (op.Value == nil || op.TTL == nil)) || (op.Operation != "set" && (op.Value != nil || op.TTL != nil)) {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	if op.Operation != "set" && (fields["value_base64"] != nil || fields["ttl_ms"] != nil) {
		return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
	}
	if (op.Operation == "get") != (in.CapabilityID == CacheServiceRead) {
		return nil, cacheInvokeError(403, "CACHE_OPERATION_FORBIDDEN")
	}
	if !cap.ServiceCredential(ctx) {
		return nil, cacheInvokeError(401, "CACHE_SERVICE_REQUIRED")
	}
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil || tenant != in.TenantUUID || op.TenantKey != tenant {
		return nil, cacheInvokeError(403, "CACHE_SCOPE_FORBIDDEN")
	}
	access, err := cap.NewGrantStatusService(i.db).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(in.CapabilityID)
	}
	if err != nil {
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			return nil, cacheInvokeError(grant.HTTPStatus(), "CACHE_GRANT_REQUIRED")
		}
		return nil, cacheInvokeError(503, "CACHE_UPSTREAM_DEPENDENCY")
	}
	if access.Subject == "" {
		return nil, cacheInvokeError(401, "CACHE_SERVICE_REQUIRED")
	}
	identity, _ := json.Marshal([]string{CacheServiceContract, access.Subject})
	subject, err := i.service.repo.Subject(ctx, uuid.MustParse(tenant), string(identity))
	if err != nil || subject == uuid.Nil {
		return nil, cacheInvokeError(503, "CACHE_UPSTREAM_DEPENDENCY")
	}
	sc := scope{uuid.MustParse(tenant), subject}
	out := map[string]any{"contract_version": CacheServiceContract, "operation": op.Operation, "namespace": op.Namespace, "key": op.Key}
	switch op.Operation {
	case "get":
		var entry *CacheEntry
		entry, err = i.service.getCache(ctx, sc, op.Namespace, op.Key)
		if err == nil {
			out["found"], out["value_base64"], out["expires_at"] = entry.Found, entry.ValueBase64, entry.ExpiresAt
		}
	case "set":
		err = i.service.setCache(ctx, sc, op.Namespace, op.Key, *op.Value, *op.TTL)
	case "delete":
		err = i.service.deleteCache(ctx, sc, op.Namespace, op.Key)
	}
	if err != nil {
		var typed *Error
		if errors.As(err, &typed) && typed.Kind == "invalid_argument" {
			return nil, cacheInvokeError(400, "CACHE_INVALID_ARGUMENT")
		}
		return nil, cacheInvokeError(503, "CACHE_UPSTREAM_DEPENDENCY")
	}
	return out, nil
}
