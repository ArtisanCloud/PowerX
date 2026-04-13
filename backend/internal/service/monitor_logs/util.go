package monitorlogs

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var tsRegex = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`)

func parseLineToEntry(line string) Entry {
	entry := Entry{Raw: line, Message: strings.TrimSpace(line), Level: "info"}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return entry
	}

	var payload map[string]any
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") && json.Unmarshal([]byte(trimmed), &payload) == nil {
		if v := getString(payload, "time"); v != "" {
			if t, ok := parseFlexibleTime(v); ok {
				entry.Timestamp = t
			}
		}
		if entry.Timestamp.IsZero() {
			if v := getString(payload, "timestamp"); v != "" {
				if t, ok := parseFlexibleTime(v); ok {
					entry.Timestamp = t
				}
			}
		}
		if v := strings.ToLower(getString(payload, "level")); v != "" {
			entry.Level = v
		}
		entry.Module = getString(payload, "component")
		entry.TraceID = getString(payload, "trace_id")
		if entry.TraceID == "" {
			entry.TraceID = getString(payload, "traceId")
		}
		entry.JobID = getUint64(payload, "job_id")
		entry.PolicyID = getUint64(payload, "policy_id")
		if msg := getString(payload, "msg"); msg != "" {
			entry.Message = msg
		}
	}

	if entry.Timestamp.IsZero() {
		if m := tsRegex.FindString(trimmed); m != "" {
			if t, ok := parseFlexibleTime(m); ok {
				entry.Timestamp = t
			}
		}
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, " error ") || strings.Contains(lower, "\"level\":\"error\"") {
		entry.Level = "error"
	} else if strings.Contains(lower, " warn ") || strings.Contains(lower, "\"level\":\"warn\"") {
		entry.Level = "warn"
	} else if strings.Contains(lower, " debug ") || strings.Contains(lower, "\"level\":\"debug\"") {
		entry.Level = "debug"
	}

	return entry
}

func parseFlexibleTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	if len(s) >= 19 {
		if t, err := time.ParseInLocation("2006-01-02 15:04:05", s[:19], time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func applyFilters(items []Entry, req QueryRequest) []Entry {
	filtered := make([]Entry, 0, len(items))
	needle := strings.ToLower(strings.TrimSpace(req.Keyword))
	traceID := strings.TrimSpace(req.TraceID)
	for i := range items {
		it := items[i]
		if traceID != "" && strings.TrimSpace(it.TraceID) != traceID {
			continue
		}
		if req.JobID > 0 && it.JobID != req.JobID {
			continue
		}
		if req.PolicyID > 0 && it.PolicyID != req.PolicyID {
			continue
		}
		if req.From != nil && it.Timestamp.Before(*req.From) {
			continue
		}
		if req.To != nil && it.Timestamp.After(*req.To) {
			continue
		}
		if needle != "" {
			hay := strings.ToLower(it.Raw + " " + it.Message)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		filtered = append(filtered, it)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})
	return filtered
}

func paginate(items []Entry, page, pageSize int) ([]Entry, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return []Entry{}, total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], total
}

func readLinesFromFile(path string, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if maxLines <= 0 {
		maxLines = 50000
	}
	s := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 2*1024*1024)
	lines := make([]string, 0, minInt(maxLines, 1024))
	for s.Scan() {
		lines = append(lines, s.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(path)
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func getUint64(m map[string]any, key string) uint64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	s := strings.TrimSpace(toString(v))
	if s == "" {
		return 0
	}
	u, _ := strconv.ParseUint(s, 10, 64)
	return u
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
