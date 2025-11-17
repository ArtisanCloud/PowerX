package toolchain

import (
	"fmt"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// Metadata describes a resolvable tool for QA orchestration.
type Metadata struct {
	ToolID   string `json:"toolId"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Endpoint string `json:"endpoint"`
}

// Registry emits static tool metadata derived from knowledge space features.
type Registry struct{}

// NewRegistry builds a registry instance.
func NewRegistry() *Registry {
	return &Registry{}
}

// Resolve returns predictable tool metadata for the supplied space.
func (r *Registry) Resolve(space *models.KnowledgeSpace) []Metadata {
	if space == nil {
		return nil
	}
	base := strings.ReplaceAll(strings.ToLower(space.SpaceName), " ", "-")
	return []Metadata{
		{
			ToolID:   fmt.Sprintf("%s-sql", base),
			Name:     fmt.Sprintf("%s SQL Lens", space.SpaceName),
			Category: "sql",
			Endpoint: fmt.Sprintf("tool://sql/%s", base),
		},
		{
			ToolID:   fmt.Sprintf("%s-rest", base),
			Name:     fmt.Sprintf("%s Realtime API", space.SpaceName),
			Category: "rest",
			Endpoint: fmt.Sprintf("tool://rest/%s", base),
		},
	}
}
