package taskqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type RuntimeInvoker struct {
	db      *gorm.DB
	service *RuntimeService
}

func NewRuntimeInvoker(db *gorm.DB, driver *StreamDriver) *RuntimeInvoker {
	return &RuntimeInvoker{db: db, service: NewRuntimeService(driver)}
}
func DecodeRuntimeRequest(body any) (RuntimeRequest, error) {
	var out RuntimeRequest
	raw, err := json.Marshal(body)
	if err != nil {
		return out, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(&struct{}{}) != io.EOF {
		return out, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	return out, nil
}
func (i *RuntimeInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	if in.CapabilityID != EnqueueCapability && in.CapabilityID != ConsumeCapability {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	if i == nil || i.db == nil || i.service == nil {
		return nil, runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
	}
	if in.Method != "INVOKE" || in.Endpoint != RuntimeEndpoint || len(in.Query) > 0 || len(in.Context) > 0 {
		return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
	}
	for key := range in.Payload {
		if key != "body" {
			return nil, runtimeError(400, "TASK_QUEUE_INVALID_ARGUMENT")
		}
	}
	op, err := DecodeRuntimeRequest(in.Body)
	if err != nil {
		return nil, err
	}
	if (op.Operation == "enqueue") != (in.CapabilityID == EnqueueCapability) {
		return nil, runtimeError(403, "TASK_QUEUE_OPERATION_FORBIDDEN")
	}
	if !cap.ServiceCredential(ctx) {
		return nil, runtimeError(401, "TASK_QUEUE_SERVICE_REQUIRED")
	}
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil || tenant != in.TenantUUID {
		return nil, runtimeError(403, "TASK_QUEUE_SCOPE_FORBIDDEN")
	}
	access, err := cap.NewGrantStatusService(i.db).CurrentAccess(ctx)
	if err != nil {
		return nil, runtimeGrantError(err)
	}
	if err = access.Require(in.CapabilityID); err != nil {
		return nil, runtimeGrantError(err)
	}
	if strings.TrimSpace(access.Subject) == "" {
		return nil, runtimeError(401, "TASK_QUEUE_SERVICE_REQUIRED")
	}
	if op.Operation == "enqueue" && op.Message != nil {
		pluginID := op.Message.Metadata["plugin_id"]
		claims := reqctx.GetClaims(ctx)
		if claims.Issuer == "powerx-sts" && pluginID != claims.PluginID {
			return nil, runtimeError(403, "TASK_QUEUE_SCOPE_FORBIDDEN")
		}
		if pluginID != "" {
			if err := pluginservice.NewPluginDrainJobService(i.db).EnsurePluginAcceptsNewUsage(ctx, pluginID); err != nil {
				return nil, err
			}
		}
	}
	return i.service.Apply(ctx, RuntimeScope{TenantUUID: tenant, SourceService: access.Subject}, op)
}

func runtimeGrantError(err error) error {
	var direct *cap.DirectGrantError
	if errors.As(err, &direct) {
		return runtimeError(direct.HTTPStatus(), "TASK_QUEUE_GRANT_REQUIRED")
	}
	return runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
}

// ApplyAdmin uses a user-owned namespace and live RBAC. Service credentials
// cannot reach this plane or select another service's queue namespace.
func (i *RuntimeInvoker) ApplyAdmin(ctx context.Context, in RuntimeRequest) (map[string]any, error) {
	if i == nil || i.db == nil {
		return nil, runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
	}
	claims := reqctx.GetClaims(ctx)
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil || claims == nil || cap.ServiceCredential(ctx) || reqctx.GetUserID(ctx) == 0 || claims.UserUUID == "" {
		return nil, runtimeError(403, "TASK_QUEUE_ADMIN_REQUIRED")
	}
	if !reqctx.IsRoot(ctx) {
		allowed, err := iamrepo.NewPermissionRepository(i.db).MemberHasLivePlatformPermission(ctx, tenant, reqctx.GetMemberUUID(ctx), reqctx.GetMemberID(ctx), "runtime_taskqueue", "admin_runtime_taskqueue_v2", "invoke")
		if err != nil {
			return nil, runtimeError(503, "TASK_QUEUE_UNAVAILABLE")
		}
		if !allowed {
			return nil, runtimeError(403, "TASK_QUEUE_ADMIN_FORBIDDEN")
		}
	}
	if in.Operation == "enqueue" && in.Message != nil && in.Message.Metadata["plugin_id"] != "" {
		if err := pluginservice.NewPluginDrainJobService(i.db).EnsurePluginAcceptsNewUsage(ctx, in.Message.Metadata["plugin_id"]); err != nil {
			return nil, err
		}
	}
	return i.service.Apply(ctx, RuntimeScope{TenantUUID: tenant, SourceService: "admin:" + claims.UserUUID}, in)
}
