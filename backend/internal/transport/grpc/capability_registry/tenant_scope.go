package capability_registry

import (
	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func tenantUUIDFromScopedID(id *capabilityRegistryPB.TenantScopedId) (string, error) {
	if id == nil {
		return "", status.Error(codes.InvalidArgument, "tenant scoped id required")
	}
	return canonicalTenantUUID(id.GetTenantUuid())
}

func canonicalTenantUUID(value string) (string, error) {
	canonical, err := reqctx.CanonicalTenantUUID(value)
	if err != nil {
		return "", status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return canonical, nil
}
