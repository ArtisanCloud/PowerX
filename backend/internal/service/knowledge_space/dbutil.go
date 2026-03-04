package knowledge_space

import "strings"

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined_table") ||
		strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "unknown table")
}

func isUndefinedTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !isMissingTableError(err) {
		return false
	}
	// best-effort match for Postgres missing relation errors.
	return strings.Contains(msg, "knowledge_chunks") || strings.Contains(msg, "knowledge_chunk_links")
}
