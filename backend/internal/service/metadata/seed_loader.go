package metadata

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const DefaultSeedPath = "config/metadata_governance/seed.yaml"

var ErrCanonicalSeedMissing = errors.New("metadata.canonical_seed_missing")

func LoadSeedFile(path string) (SeedFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SeedFile{}, fmt.Errorf("metadata seed path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return SeedFile{}, err
	}
	var seed SeedFile
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&seed); err != nil {
		return SeedFile{}, err
	}
	if err := ValidateSeedFile(seed); err != nil {
		return SeedFile{}, err
	}
	return seed, nil
}

func ValidateCanonicalSeedDefinitions(seed SeedFile) error {
	if err := ValidateSeedFile(seed); err != nil {
		return err
	}
	if len(seed.ResourceTypes) == 0 {
		return fmt.Errorf("%w: resource_types", ErrCanonicalSeedMissing)
	}
	if len(seed.Dictionaries) == 0 && len(seed.Taxonomies) == 0 && len(seed.Tags) == 0 {
		return fmt.Errorf("%w: dictionaries/taxonomies/tags", ErrCanonicalSeedMissing)
	}
	return nil
}
