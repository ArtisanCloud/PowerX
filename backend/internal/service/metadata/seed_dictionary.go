package metadata

import (
	"context"
	"fmt"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/metadata"
	metarepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/metadata"
)

type SeedFile struct {
	Version       int                       `yaml:"version"`
	Module        string                    `yaml:"module"`
	Dictionaries  []DictionaryNamespaceSeed `yaml:"dictionaries"`
	Taxonomies    []TaxonomySeed            `yaml:"taxonomies"`
	ResourceTypes []ResourceTypeSeed        `yaml:"resource_types"`
	Tags          []TagSeed                 `yaml:"tags"`
}

type DictionaryNamespaceSeed struct {
	Namespace       string               `yaml:"namespace"`
	Module          string               `yaml:"module"`
	NameI18n        map[string]string    `yaml:"name_i18n"`
	DescriptionI18n map[string]string    `yaml:"description_i18n"`
	Status          string               `yaml:"status"`
	Items           []DictionaryItemSeed `yaml:"items"`
}

type DictionaryItemSeed struct {
	Code            string            `yaml:"code"`
	LabelI18n       map[string]string `yaml:"label_i18n"`
	DescriptionI18n map[string]string `yaml:"description_i18n"`
	SortOrder       int               `yaml:"sort_order"`
	Status          string            `yaml:"status"`
	Metadata        map[string]any    `yaml:"metadata"`
}

type TaxonomySeed struct {
	Namespace       string             `yaml:"namespace"`
	Module          string             `yaml:"module"`
	NameI18n        map[string]string  `yaml:"name_i18n"`
	DescriptionI18n map[string]string  `yaml:"description_i18n"`
	MaxDepth        int                `yaml:"max_depth"`
	Status          string             `yaml:"status"`
	Nodes           []TaxonomyNodeSeed `yaml:"nodes"`
}

type TaxonomyNodeSeed struct {
	ParentCode      string            `yaml:"parent_code"`
	Code            string            `yaml:"code"`
	LabelI18n       map[string]string `yaml:"label_i18n"`
	DescriptionI18n map[string]string `yaml:"description_i18n"`
	SortOrder       int               `yaml:"sort_order"`
	Status          string            `yaml:"status"`
}

type TagSeed struct {
	Namespace       string            `yaml:"namespace"`
	ResourceType    string            `yaml:"resource_type"`
	Code            string            `yaml:"code"`
	LabelI18n       map[string]string `yaml:"label_i18n"`
	DescriptionI18n map[string]string `yaml:"description_i18n"`
	Color           string            `yaml:"color"`
	Status          string            `yaml:"status"`
	Source          string            `yaml:"source"`
}

type ResourceTypeSeed struct {
	ResourceType    string            `yaml:"resource_type"`
	Module          string            `yaml:"module"`
	NameI18n        map[string]string `yaml:"name_i18n"`
	DescriptionI18n map[string]string `yaml:"description_i18n"`
	ValidatorKey    string            `yaml:"validator_key"`
	BindingEnabled  bool              `yaml:"binding_enabled"`
	Status          string            `yaml:"status"`
}

type SeedResult struct {
	DictionaryNamespaces int
	DictionaryItems      int
	Taxonomies           int
	TaxonomyNodes        int
	ResourceTypes        int
	Tags                 int
}

func ValidateSeedFile(seed SeedFile) error {
	if seed.Version <= 0 {
		return fmt.Errorf("metadata seed requires positive version")
	}
	if strings.TrimSpace(seed.Module) == "" {
		return fmt.Errorf("metadata seed requires module")
	}
	for i := range seed.Dictionaries {
		ns := seed.Dictionaries[i]
		namespace := strings.TrimSpace(ns.Namespace)
		if err := ValidateMachineIdentifier(namespace); err != nil {
			return fmt.Errorf("dictionary namespace %q: %w", namespace, err)
		}
		module := strings.TrimSpace(ns.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		if err := ValidateMachineIdentifier(module); err != nil {
			return fmt.Errorf("dictionary namespace %q module: %w", namespace, err)
		}
		if err := ValidateRequiredI18n(ns.NameI18n, "zh-CN"); err != nil {
			return fmt.Errorf("dictionary namespace %q name_i18n: %w", namespace, err)
		}
		if status := strings.TrimSpace(ns.Status); status != "" {
			if err := ValidateStatus(status); err != nil {
				return fmt.Errorf("dictionary namespace %q status: %w", namespace, err)
			}
		}
		for j := range ns.Items {
			item := ns.Items[j]
			code := strings.TrimSpace(item.Code)
			if err := ValidateMachineIdentifier(code); err != nil {
				return fmt.Errorf("dictionary namespace %q item %q: %w", namespace, code, err)
			}
			if err := ValidateRequiredI18n(item.LabelI18n, "zh-CN"); err != nil {
				return fmt.Errorf("dictionary namespace %q item %q label_i18n: %w", namespace, code, err)
			}
			if status := strings.TrimSpace(item.Status); status != "" {
				if err := ValidateStatus(status); err != nil {
					return fmt.Errorf("dictionary namespace %q item %q status: %w", namespace, code, err)
				}
			}
		}
	}
	for i := range seed.Taxonomies {
		taxonomy := seed.Taxonomies[i]
		namespace := strings.TrimSpace(taxonomy.Namespace)
		if err := ValidateMachineIdentifier(namespace); err != nil {
			return fmt.Errorf("taxonomy %q: %w", namespace, err)
		}
		module := strings.TrimSpace(taxonomy.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		if err := ValidateMachineIdentifier(module); err != nil {
			return fmt.Errorf("taxonomy %q module: %w", namespace, err)
		}
		if err := ValidateRequiredI18n(taxonomy.NameI18n, "zh-CN"); err != nil {
			return fmt.Errorf("taxonomy %q name_i18n: %w", namespace, err)
		}
		if taxonomy.MaxDepth < 1 {
			return fmt.Errorf("taxonomy %q: %w", namespace, ErrInvalidDepth)
		}
		if status := strings.TrimSpace(taxonomy.Status); status != "" {
			if err := ValidateStatus(status); err != nil {
				return fmt.Errorf("taxonomy %q status: %w", namespace, err)
			}
		}
		seenCodes := map[string]struct{}{}
		for j := range taxonomy.Nodes {
			node := taxonomy.Nodes[j]
			code := strings.TrimSpace(node.Code)
			if err := ValidateMachineIdentifier(code); err != nil {
				return fmt.Errorf("taxonomy %q node %q: %w", namespace, code, err)
			}
			if _, ok := seenCodes[code]; ok {
				return fmt.Errorf("taxonomy %q node %q: duplicate code", namespace, code)
			}
			seenCodes[code] = struct{}{}
			if parentCode := strings.TrimSpace(node.ParentCode); parentCode != "" {
				if err := ValidateMachineIdentifier(parentCode); err != nil {
					return fmt.Errorf("taxonomy %q node %q parent_code: %w", namespace, code, err)
				}
			}
			if err := ValidateRequiredI18n(node.LabelI18n, "zh-CN"); err != nil {
				return fmt.Errorf("taxonomy %q node %q label_i18n: %w", namespace, code, err)
			}
			if status := strings.TrimSpace(node.Status); status != "" {
				if err := ValidateStatus(status); err != nil {
					return fmt.Errorf("taxonomy %q node %q status: %w", namespace, code, err)
				}
			}
		}
	}
	for i := range seed.Tags {
		tag := seed.Tags[i]
		namespace := strings.TrimSpace(tag.Namespace)
		if err := ValidateMachineIdentifier(namespace); err != nil {
			return fmt.Errorf("tag %q: %w", namespace, err)
		}
		resourceType := strings.TrimSpace(tag.ResourceType)
		if err := ValidateMachineIdentifier(resourceType); err != nil {
			return fmt.Errorf("tag %q resource_type: %w", namespace, err)
		}
		code := strings.TrimSpace(tag.Code)
		if err := ValidateMachineIdentifier(code); err != nil {
			return fmt.Errorf("tag %q code %q: %w", namespace, code, err)
		}
		if err := ValidateRequiredI18n(tag.LabelI18n, "zh-CN"); err != nil {
			return fmt.Errorf("tag %q code %q label_i18n: %w", namespace, code, err)
		}
		if status := strings.TrimSpace(tag.Status); status != "" {
			if err := ValidateStatus(status); err != nil {
				return fmt.Errorf("tag %q code %q status: %w", namespace, code, err)
			}
		}
	}
	for i := range seed.ResourceTypes {
		resourceType := seed.ResourceTypes[i]
		value := strings.TrimSpace(resourceType.ResourceType)
		if err := ValidateMachineIdentifier(value); err != nil {
			return fmt.Errorf("resource_type %q: %w", value, err)
		}
		module := strings.TrimSpace(resourceType.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		if err := ValidateMachineIdentifier(module); err != nil {
			return fmt.Errorf("resource_type %q module: %w", value, err)
		}
		if err := ValidateRequiredI18n(resourceType.NameI18n, "zh-CN"); err != nil {
			return fmt.Errorf("resource_type %q name_i18n: %w", value, err)
		}
		if resourceType.BindingEnabled && strings.TrimSpace(resourceType.ValidatorKey) == "" {
			return fmt.Errorf("resource_type %q: %w", value, ErrResourceValidatorMissing)
		}
		if status := strings.TrimSpace(resourceType.Status); status != "" {
			if err := ValidateStatus(status); err != nil {
				return fmt.Errorf("resource_type %q status: %w", value, err)
			}
		}
	}
	return nil
}

func (s *Service) SeedDictionaries(ctx context.Context, tenantUUID string, seed SeedFile) (SeedResult, error) {
	canonicalTenant, err := canonicalTenant(tenantUUID)
	if err != nil {
		return SeedResult{}, err
	}
	if err := ValidateSeedFile(seed); err != nil {
		return SeedResult{}, err
	}
	result := SeedResult{}
	repo := metarepo.NewDictionaryRepository(s.deps.DB)
	for i := range seed.Dictionaries {
		ns := seed.Dictionaries[i]
		module := strings.TrimSpace(ns.Module)
		if module == "" {
			module = strings.TrimSpace(seed.Module)
		}
		nsStatus := strings.TrimSpace(ns.Status)
		if nsStatus == "" {
			nsStatus = model.StatusEnabled
		}
		namespaceRow, err := repo.UpsertNamespaceByNamespace(ctx, &model.DictionaryNamespace{
			TenantUUID:      canonicalTenant,
			Namespace:       strings.TrimSpace(ns.Namespace),
			Module:          module,
			NameI18n:        mustJSON(ns.NameI18n),
			DescriptionI18n: mustJSON(ns.DescriptionI18n),
			Status:          nsStatus,
		})
		if err != nil {
			return result, err
		}
		result.DictionaryNamespaces++
		for j := range ns.Items {
			item := ns.Items[j]
			itemStatus := strings.TrimSpace(item.Status)
			if itemStatus == "" {
				itemStatus = model.StatusEnabled
			}
			if _, err := repo.UpsertItemByCode(ctx, &model.DictionaryItem{
				TenantUUID:      canonicalTenant,
				NamespaceUUID:   namespaceRow.UUID.String(),
				Code:            strings.TrimSpace(item.Code),
				LabelI18n:       mustJSON(item.LabelI18n),
				DescriptionI18n: mustJSON(item.DescriptionI18n),
				SortOrder:       item.SortOrder,
				Status:          itemStatus,
				Metadata:        mustJSONAny(item.Metadata),
			}); err != nil {
				return result, err
			}
			result.DictionaryItems++
		}
	}
	return result, nil
}
