package plugin

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Grant-source data is Core-owned credential metadata, never plugin config.
// Unknown historical grants are retained for review but are not effective.
// independent is loaded from Core approval records, never from doc.
func reconcileManifestCapabilityGrants(doc map[string]any, required []string, independent map[string]string) error {
	read := func(key string) ([]string, error) {
		value, exists := doc[key]
		if !exists {
			return nil, nil
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var values []string
		if err = json.Unmarshal(raw, &values); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", key, err)
		}
		return normalizedRequiredCapabilities(values)
	}
	old, err := read("allowed_capabilities")
	if err != nil {
		return err
	}
	previous, err := read("manifest_capabilities")
	if err != nil {
		return err
	}
	quarantined, err := read("unattributed_capabilities")
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, cap := range previous {
		known[cap] = true
	}
	for _, cap := range required {
		known[cap] = true
	}
	effective := append([]string{}, required...)
	for cap := range independent {
		if _, err := normalizedRequiredCapabilities([]string{cap}); err != nil {
			return err
		}
		known[cap] = true
		effective = append(effective, cap)
	}
	for _, cap := range old {
		if !known[cap] {
			quarantined = append(quarantined, cap)
		}
	}
	effective, err = normalizedRequiredCapabilities(effective)
	if err != nil {
		return err
	}
	quarantined, err = normalizedRequiredCapabilities(quarantined)
	if err != nil {
		return err
	}
	doc["manifest_capabilities"] = append([]string{}, required...)
	if independent == nil {
		independent = map[string]string{}
	}
	doc["independent_capability_grants"] = independent
	sort.Strings(effective)
	sort.Strings(quarantined)
	doc["allowed_capabilities"] = effective
	doc["unattributed_capabilities"] = quarantined
	doc["capability_grant_schema_version"] = 1
	return nil
}
