package capability_registry

import (
	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func tenantUUIDFromScopedID(id *capabilityRegistryPB.TenantScopedId) (string, error) {
	if id == nil {
		return "", capability_registrydto.ToGRPCError(capability_registrydto.ErrInvalidRequest.WithHint("tenant scoped id required"), nil)
	}
	return canonicalTenantUUID(id.GetTenantUuid())
}

func canonicalTenantUUID(value string) (string, error) {
	canonical, err := reqctx.CanonicalTenantUUID(value)
	if err != nil {
		return "", capability_registrydto.ToGRPCError(capability_registrydto.ErrTenantUUIDInvalid, err)
	}
	return canonical, nil
}
