package plugin_mgr

import "slices"

// DatasetObservationSpec pins the declaration asset and its observable relations.
// Relations are scoped to the installed plugin's managed schema by Core.
type DatasetObservationSpec struct {
	SHA256 string   `yaml:"sha256" json:"sha256"`
	Tables []string `yaml:"observation_tables" json:"observation_tables"`
}

func CloneDatasetObservationSpecs(in map[string]DatasetObservationSpec) map[string]DatasetObservationSpec {
	if in == nil {
		return nil
	}
	out := make(map[string]DatasetObservationSpec, len(in))
	for key, item := range in {
		item.Tables = slices.Clone(item.Tables)
		out[key] = item
	}
	return out
}
