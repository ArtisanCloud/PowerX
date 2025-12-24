package capability_registry

import (
	"encoding/json"
	"errors"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
)

const (
	CapabilitySourceCoreX  = "corex"
	CapabilitySourcePlugin = "plugin"
)

var (
	ErrInvalidCapabilitySource = errors.New("invalid capability source")
	validSourceAliases         = map[string]string{
		"corex":    CapabilitySourceCoreX,
		"platform": CapabilitySourceCoreX,
		"plugin":   CapabilitySourcePlugin,
	}
)

// CapabilitySource returns the canonical source of a capability record.
func CapabilitySource(record *models.CapabilityRecord) string {
	if record == nil {
		return CapabilitySourcePlugin
	}
	if source := sourceFromAnnotations(record.Annotations); source != "" {
		return source
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record.PluginID)), "corex.") {
		return CapabilitySourceCoreX
	}
	return CapabilitySourcePlugin
}

// NormalizeCapabilitySource validates and normalizes a source filter.
func NormalizeCapabilitySource(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "all" || value == "any" {
		return "", nil
	}
	if normalized, ok := validSourceAliases[value]; ok {
		return normalized, nil
	}
	return "", ErrInvalidCapabilitySource
}

func sourceFromAnnotations(raw []byte) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var annotations map[string]interface{}
	if err := json.Unmarshal(raw, &annotations); err != nil {
		return ""
	}
	if rawSource, ok := annotations["source"]; ok {
		switch typed := rawSource.(type) {
		case string:
			if canonical, ok := validSourceAliases[strings.ToLower(strings.TrimSpace(typed))]; ok {
				return canonical
			}
			return strings.TrimSpace(strings.ToLower(typed))
		}
	}
	return ""
}
