package metadata

import (
	"context"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
)

func (s *Service) SeedResourceTypes(ctx context.Context, tenantUUID string, seed SeedFile) (SeedResult, error) {
	canonicalTenant, err := canonicalTenant(tenantUUID)
	if err != nil {
		return SeedResult{}, err
	}
	if err := ValidateSeedFile(seed); err != nil {
		return SeedResult{}, err
	}
	result := SeedResult{}
	repo := s.resourceTypeRepo()
	for i := range seed.ResourceTypes {
		resourceType := seed.ResourceTypes[i]
		module := strings.TrimSpace(resourceType.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		status := strings.TrimSpace(resourceType.Status)
		if status == "" {
			status = model.StatusEnabled
		}
		if _, err := repo.UpsertByResourceType(ctx, &model.ResourceType{
			TenantUUID:      canonicalTenant,
			ResourceType:    strings.TrimSpace(resourceType.ResourceType),
			Module:          module,
			NameI18n:        mustJSON(resourceType.NameI18n),
			DescriptionI18n: mustJSON(resourceType.DescriptionI18n),
			ValidatorKey:    strings.TrimSpace(resourceType.ValidatorKey),
			BindingEnabled:  resourceType.BindingEnabled,
			Status:          status,
		}); err != nil {
			return result, err
		}
		result.ResourceTypes++
	}
	return result, nil
}
