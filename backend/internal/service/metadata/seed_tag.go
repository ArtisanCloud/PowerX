package metadata

import (
	"context"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
)

func (s *Service) SeedTags(ctx context.Context, tenantUUID string, seed SeedFile) (SeedResult, error) {
	canonicalTenant, err := canonicalTenant(tenantUUID)
	if err != nil {
		return SeedResult{}, err
	}
	if err := ValidateSeedFile(seed); err != nil {
		return SeedResult{}, err
	}
	result := SeedResult{}
	repo := metarepo.NewTagRepository(s.deps.DB)
	for i := range seed.Tags {
		tag := seed.Tags[i]
		status := strings.TrimSpace(tag.Status)
		if status == "" {
			status = model.StatusEnabled
		}
		source := strings.TrimSpace(tag.Source)
		if source == "" {
			source = model.SourceSeed
		}
		if _, err := repo.UpsertTagByCode(ctx, &model.Tag{
			TenantUUID:      canonicalTenant,
			Namespace:       strings.TrimSpace(tag.Namespace),
			ResourceType:    strings.TrimSpace(tag.ResourceType),
			Code:            strings.TrimSpace(tag.Code),
			LabelI18n:       mustJSON(tag.LabelI18n),
			DescriptionI18n: mustJSON(tag.DescriptionI18n),
			Color:           strings.TrimSpace(tag.Color),
			Source:          source,
			Status:          status,
		}); err != nil {
			return result, err
		}
		result.Tags++
	}
	return result, nil
}
