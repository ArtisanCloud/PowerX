package plugin_bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

type templateRegistry struct {
	path string

	mu    sync.RWMutex
	index TemplateIndex
}

func newTemplateRegistry(path string) (*templateRegistry, error) {
	reg := &templateRegistry{path: filepath.Clean(path)}
	if err := reg.reload(); err != nil {
		return nil, err
	}
	return reg, nil
}

func (r *templateRegistry) reload() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	var idx TemplateIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.index = idx
	return nil
}

func (r *templateRegistry) find(id string) (*TemplateSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := range r.index.Templates {
		if r.index.Templates[i].ID == id {
			spec := r.index.Templates[i]
			return &spec, true
		}
	}
	return nil, false
}

func (r *templateRegistry) list() []TemplateSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cloned := make([]TemplateSpec, len(r.index.Templates))
	copy(cloned, r.index.Templates)
	return cloned
}

var errTemplateNotFound = errors.New("template not found")
