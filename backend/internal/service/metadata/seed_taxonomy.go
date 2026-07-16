package metadata

import (
	"context"
	"fmt"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
	"github.com/google/uuid"
)

func (s *Service) SeedTaxonomies(ctx context.Context, tenantUUID string, seed SeedFile) (SeedResult, error) {
	canonicalTenant, err := canonicalTenant(tenantUUID)
	if err != nil {
		return SeedResult{}, err
	}
	if err := ValidateSeedFile(seed); err != nil {
		return SeedResult{}, err
	}
	result := SeedResult{}
	repo := metarepo.NewTaxonomyRepository(s.deps.DB)
	for i := range seed.Taxonomies {
		taxonomy := seed.Taxonomies[i]
		module := strings.TrimSpace(taxonomy.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		taxonomyStatus := strings.TrimSpace(taxonomy.Status)
		if taxonomyStatus == "" {
			taxonomyStatus = model.StatusEnabled
		}
		taxonomyRow, err := repo.UpsertTaxonomyByNamespace(ctx, &model.Taxonomy{
			TenantUUID:      canonicalTenant,
			Namespace:       strings.TrimSpace(taxonomy.Namespace),
			Module:          module,
			NameI18n:        mustJSON(taxonomy.NameI18n),
			DescriptionI18n: mustJSON(taxonomy.DescriptionI18n),
			MaxDepth:        taxonomy.MaxDepth,
			Status:          taxonomyStatus,
		})
		if err != nil {
			return result, err
		}
		result.Taxonomies++

		codeToNode := map[string]*model.TaxonomyNode{}
		for j := range taxonomy.Nodes {
			node := taxonomy.Nodes[j]
			parentCode := strings.TrimSpace(node.ParentCode)
			parent := codeToNode[parentCode]
			if parentCode != "" && parent == nil {
				return result, fmt.Errorf("%w: taxonomy=%s node=%s parent_code=%s", ErrInvalidParentReference, strings.TrimSpace(taxonomy.Namespace), strings.TrimSpace(node.Code), parentCode)
			}
			depth := 1
			var parentUUID *string
			if parent != nil {
				depth = parent.Depth + 1
				value := parent.UUID.String()
				parentUUID = &value
			}
			if depth > taxonomyRow.MaxDepth {
				return result, ErrInvalidDepth
			}
			nodeStatus := strings.TrimSpace(node.Status)
			if nodeStatus == "" {
				nodeStatus = model.StatusEnabled
			}
			nodeUUID := uuid.New()
			path := taxonomyNodePath(taxonomyRow.UUID.String(), nodeUUID.String(), parent)
			nodeRow, err := repo.UpsertNodeByCode(ctx, &model.TaxonomyNode{
				PowerUUIDModel:  coremodel.PowerUUIDModel{UUID: nodeUUID},
				TenantUUID:      canonicalTenant,
				TaxonomyUUID:    taxonomyRow.UUID.String(),
				ParentUUID:      parentUUID,
				Code:            strings.TrimSpace(node.Code),
				LabelI18n:       mustJSON(node.LabelI18n),
				DescriptionI18n: mustJSON(node.DescriptionI18n),
				Path:            path,
				Depth:           depth,
				SortOrder:       node.SortOrder,
				Status:          nodeStatus,
				Version:         1,
			})
			if err != nil {
				return result, err
			}
			codeToNode[nodeRow.Code] = nodeRow
			result.TaxonomyNodes++
		}
	}
	return result, nil
}
