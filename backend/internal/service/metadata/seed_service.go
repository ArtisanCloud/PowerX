package metadata

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

type SeedService struct {
	metadata *Service
	seedPath string
}

type SeedServiceOptions struct {
	DB                *gorm.DB
	SeedPath          string
	ValidatorRegistry ResourceValidatorRegistry
}

type SeedExecutionInput struct {
	TenantUUID                  string
	SeedPath                    string
	DryRun                      bool
	RequireCanonicalDefinitions bool
}

func NewSeedService(opts SeedServiceOptions) (*SeedService, error) {
	metadataSvc, err := NewService(Deps{DB: opts.DB, ValidatorRegistry: opts.ValidatorRegistry})
	if err != nil {
		return nil, err
	}
	seedPath := strings.TrimSpace(opts.SeedPath)
	if seedPath == "" {
		seedPath = DefaultSeedPath
	}
	return &SeedService{metadata: metadataSvc, seedPath: seedPath}, nil
}

func (s *SeedService) Execute(ctx context.Context, in SeedExecutionInput) (SeedResult, SeedFile, error) {
	if s == nil || s.metadata == nil {
		return SeedResult{}, SeedFile{}, ErrMissingDB
	}
	tenantUUID, err := canonicalTenant(in.TenantUUID)
	if err != nil {
		return SeedResult{}, SeedFile{}, err
	}
	seedPath := strings.TrimSpace(in.SeedPath)
	if seedPath == "" {
		seedPath = s.seedPath
	}
	seed, err := LoadSeedFile(seedPath)
	if err != nil {
		return SeedResult{}, SeedFile{}, err
	}
	if in.RequireCanonicalDefinitions {
		if err := ValidateCanonicalSeedDefinitions(seed); err != nil {
			return SeedResult{}, SeedFile{}, err
		}
	}
	if in.DryRun {
		return SeedResult{}, seed, nil
	}

	var result SeedResult
	dictionaryResult, err := s.metadata.SeedDictionaries(ctx, tenantUUID, seed)
	if err != nil {
		return result, seed, err
	}
	result.DictionaryNamespaces = dictionaryResult.DictionaryNamespaces
	result.DictionaryItems = dictionaryResult.DictionaryItems

	taxonomyResult, err := s.metadata.SeedTaxonomies(ctx, tenantUUID, seed)
	if err != nil {
		return result, seed, err
	}
	result.Taxonomies = taxonomyResult.Taxonomies
	result.TaxonomyNodes = taxonomyResult.TaxonomyNodes

	resourceTypeResult, err := s.metadata.SeedResourceTypes(ctx, tenantUUID, seed)
	if err != nil {
		return result, seed, err
	}
	result.ResourceTypes = resourceTypeResult.ResourceTypes

	tagResult, err := s.metadata.SeedTags(ctx, tenantUUID, seed)
	if err != nil {
		return result, seed, err
	}
	result.Tags = tagResult.Tags
	return result, seed, nil
}

func (s *SeedService) BootstrapTenantMetadata(ctx context.Context, tenantUUID string) error {
	_, _, err := s.Execute(ctx, SeedExecutionInput{
		TenantUUID:                  tenantUUID,
		RequireCanonicalDefinitions: true,
	})
	return err
}
