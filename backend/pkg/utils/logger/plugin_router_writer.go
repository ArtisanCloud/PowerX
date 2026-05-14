package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	lumberjack "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
	"go.uber.org/zap/zapcore"
)

const (
	defaultPluginLogDir = "logs/plugins"
)

var pluginIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type pluginRouterWriter struct {
	mu       sync.Mutex
	level    string
	baseDir  string
	fileCfg  *logcfg.FileConfig
	writers  map[string]zapcore.WriteSyncer
	enabled  bool
}

func newPluginRouterWriter(level string, fileCfg *logcfg.FileConfig) zapcore.WriteSyncer {
	w := &pluginRouterWriter{
		level:   strings.ToLower(strings.TrimSpace(level)),
		baseDir: resolvePluginLogDir(),
		fileCfg: fileCfg,
		writers: map[string]zapcore.WriteSyncer{},
		enabled: pluginLogIsolationEnabled(),
	}
	if !w.enabled {
		return zapcore.AddSync(nilWriter{})
	}
	return zapcore.AddSync(w)
}

func (w *pluginRouterWriter) Write(p []byte) (int, error) {
	if !w.enabled || len(p) == 0 {
		return len(p), nil
	}
	pluginID := extractPluginIDFromJSONLine(p)
	if pluginID == "" {
		return len(p), nil
	}
	sanitized := sanitizePluginID(pluginID)
	if sanitized == "" {
		return len(p), nil
	}
	ws := w.getWriter(sanitized)
	if ws == nil {
		return len(p), nil
	}
	_, err := ws.Write(p)
	return len(p), err
}

func (w *pluginRouterWriter) Sync() error {
	return nil
}

func (w *pluginRouterWriter) getWriter(pluginID string) zapcore.WriteSyncer {
	w.mu.Lock()
	defer w.mu.Unlock()
	if ws, ok := w.writers[pluginID]; ok {
		return ws
	}
	levelFile := "info.log"
	if w.level == "error" {
		levelFile = "error.log"
	}
	targetFile := filepath.Join(w.baseDir, pluginID, levelFile)
	_ = os.MkdirAll(filepath.Dir(targetFile), 0o755)
	lw := &lumberjack.Logger{
		Filename:   targetFile,
		MaxSize:    coalescePositive(w.fileCfg.MaxSize, 100),
		MaxBackups: coalescePositive(w.fileCfg.MaxBackups, 10),
		MaxAge:     coalescePositive(w.fileCfg.MaxAge, 30),
		Compress:   w.fileCfg.Compress,
	}
	ws := zapcore.AddSync(newDailyRotateWriter(lw, w.fileCfg.RotateDaily))
	w.writers[pluginID] = ws
	return ws
}

func extractPluginIDFromJSONLine(raw []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if value, ok := payload["plugin_id"]; ok {
		return strings.TrimSpace(toString(value))
	}
	if labels, ok := payload["labels"]; ok {
		if text := strings.TrimSpace(toString(labels)); text != "" {
			// labels is often "map[... plugin_id:xxx ...]"; lightweight extraction.
			if idx := strings.Index(text, "plugin_id:"); idx >= 0 {
				rest := text[idx+len("plugin_id:"):]
				end := strings.IndexAny(rest, " ]")
				if end > 0 {
					return strings.TrimSpace(rest[:end])
				}
				return strings.TrimSpace(rest)
			}
		}
	}
	return ""
}

func sanitizePluginID(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = pluginIDSanitizer.ReplaceAllString(v, "_")
	v = strings.Trim(v, "._-")
	return v
}

func resolvePluginLogDir() string {
	if v := strings.TrimSpace(os.Getenv("CORE_X_LOG_PLUGIN_DIR")); v != "" {
		return v
	}
	return defaultPluginLogDir
}

func pluginLogIsolationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("CORE_X_LOG_PLUGIN_ISOLATION_ENABLE"))) {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func coalescePositive(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

type nilWriter struct{}

func (nilWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nilWriter) Sync() error                  { return nil }

