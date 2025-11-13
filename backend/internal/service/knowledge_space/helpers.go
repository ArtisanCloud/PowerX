package knowledge_space

import (
	"encoding/json"
	"strconv"
	"strings"

	"gorm.io/datatypes"
)

// FeatureFlagsFromJSON decodes JSON blob into string slice.
func FeatureFlagsFromJSON(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var flags []string
	if err := json.Unmarshal(raw, &flags); err != nil {
		return nil
	}
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// PolicyIDString converts uint64 id to API string.
func PolicyIDString(id uint64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatUint(id, 10)
}

// EncodeConcurrencyFlag ensures ingestion concurrency flag is embedded.
func EncodeConcurrencyFlag(flags []string, concurrency int) []string {
	if concurrency <= 0 {
		concurrency = 1
	}
	const prefix = "quota.ingestion:"
	out := make([]string, 0, len(flags)+1)
	for _, f := range flags {
		if strings.HasPrefix(strings.ToLower(f), prefix) {
			continue
		}
		out = append(out, f)
	}
	out = append(out, prefix+strconv.Itoa(concurrency))
	return out
}

// ExtractConcurrencyFlag reads concurrency from flags, defaults to 1.
func ExtractConcurrencyFlag(flags []string) int {
	const prefix = "quota.ingestion:"
	for _, f := range flags {
		if strings.HasPrefix(strings.ToLower(f), prefix) {
			val := strings.TrimSpace(strings.TrimPrefix(f, prefix))
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				return v
			}
		}
	}
	return 1
}
