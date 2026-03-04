package agent

import (
	"encoding/json"
	"strconv"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
)

// ResolveEmbeddingDimensions returns embedding dimensions from profile defaults/cap_cache.
func ResolveEmbeddingDimensions(profile *dbmodel.AIModelProfile) int {
	if profile == nil {
		return 0
	}
	if dim := readDimensionFromMap(profile.Defaults); dim > 0 {
		return dim
	}
	return readDimensionFromMap(profile.CapCache)
}

// EmbeddingProfileProbed reports whether profile has a valid probe stamp.
func EmbeddingProfileProbed(profile *dbmodel.AIModelProfile) bool {
	if profile == nil || profile.CapCache == nil {
		return false
	}
	val, ok := profile.CapCache["probed_at"]
	if !ok {
		return false
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return v != nil
	}
}

// EmbeddingProfileReady checks both dimension and probe stamp.
func EmbeddingProfileReady(profile *dbmodel.AIModelProfile) bool {
	return ResolveEmbeddingDimensions(profile) > 0 && EmbeddingProfileProbed(profile)
}

func readDimensionFromMap(values map[string]any) int {
	if len(values) == 0 {
		return 0
	}
	if v, ok := values["dimensions"]; ok {
		if d := parseDimension(v); d > 0 {
			return d
		}
	}
	if v, ok := values["dim"]; ok {
		if d := parseDimension(v); d > 0 {
			return d
		}
	}
	return 0
}

func parseDimension(v any) int {
	switch val := v.(type) {
	case float64:
		if int(val) > 0 {
			return int(val)
		}
	case float32:
		if int(val) > 0 {
			return int(val)
		}
	case int:
		if val > 0 {
			return val
		}
	case int32:
		if val > 0 {
			return int(val)
		}
	case int64:
		if val > 0 {
			return int(val)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && parsed > 0 {
			return parsed
		}
	case json.Number:
		if parsed, err := val.Int64(); err == nil && parsed > 0 {
			return int(parsed)
		}
		if parsed, err := strconv.Atoi(strings.TrimSpace(val.String())); err == nil && parsed > 0 {
			return parsed
		}
	}
	return 0
}
