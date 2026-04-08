package backup_ops

import "strings"

func normalizeOperator(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "system"
	}
	return v
}
