package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func tenantUUIDFromContext(ctx context.Context, rc *commonv1.RequestContext) (string, error) {
	candidate := ""
	if rc != nil {
		candidate = strings.TrimSpace(rc.GetTenantUuid())
		if candidate == "" {
			if attrs := rc.GetAttributes(); attrs != nil {
				candidate = strings.TrimSpace(attrs["tenant_uuid"])
			}
		}
	}
	if candidate == "" {
		candidate = strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	}
	if candidate == "" {
		return "", errors.New("tenant_uuid required")
	}
	canonical, err := reqctx.CanonicalTenantUUID(candidate)
	if err != nil {
		return "", fmt.Errorf("tenant uuid invalid: %w", err)
	}
	return canonical, nil
}

func tenantRepoFromDeps(deps *shared.Deps) *tenantrepo.TenantRepository {
	if deps == nil {
		return nil
	}
	if deps.TenantSvc != nil && deps.TenantSvc.Repo != nil {
		return deps.TenantSvc.Repo
	}
	if deps.DB != nil {
		return tenantrepo.NewTenantRepository(deps.DB)
	}
	return nil
}
