package capability_registry

import (
	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	caperrdto "github.com/ArtisanCloud/PowerX/internal/dto/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func tenantUUIDFromScopedID(id *capabilityRegistryPB.TenantScopedId) (string, error) {
	if id == nil {
		return "", caperrdto.ToGRPCError(caperrdto.ErrInvalidRequest.WithHint("tenant scoped id required"), nil)
	}
	return canonicalTenantUUID(id.GetTenantUuid())
}

func canonicalTenantUUID(value string) (string, error) {
	canonical, err := reqctx.CanonicalTenantUUID(value)
	if err != nil {
		return "", caperrdto.ToGRPCError(caperrdto.ErrTenantUUIDInvalid, err)
	}
	return canonical, nil
}
