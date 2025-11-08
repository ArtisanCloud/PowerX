package plugin_sandbox

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Suite describes available sandbox datasets.
type Suite struct {
	Version  string        `yaml:"version"`
	Datasets []DatasetSpec `yaml:"datasets"`
	index    map[string]DatasetSpec
}

// DatasetSpec describes a dataset entry.
type DatasetSpec struct {
	ID             string   `yaml:"id"`
	Description    string   `yaml:"description"`
	DefaultVersion string   `yaml:"default_version"`
	Resource       string   `yaml:"resource"`
	Coverage       float64  `yaml:"coverage"`
	Tags           []string `yaml:"tags"`
}

// LoadSuite loads suite definition from yaml.
func LoadSuite(path string) (*Suite, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, err
	}
	suite.buildIndex()
	return &suite, nil
}

func (s *Suite) buildIndex() {
	if s == nil {
		return
	}
	s.index = make(map[string]DatasetSpec)
	for _, dataset := range s.Datasets {
		s.index[dataset.ID] = dataset
	}
}

// Lookup returns dataset spec by id.
func (s *Suite) Lookup(id string) (DatasetSpec, bool) {
	if s == nil || s.index == nil {
		return DatasetSpec{}, false
	}
	spec, ok := s.index[id]
	return spec, ok
}
