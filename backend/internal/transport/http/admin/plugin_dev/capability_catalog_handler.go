package plugin_dev

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type capabilityCatalogRequest struct {
	Catalog capabilityCatalogSnapshot `json:"catalog"`
	Assets  []capabilityCatalogAsset  `json:"assets"`
}

type capabilityCatalogSnapshot struct {
	PluginID        string                   `json:"plugin_id"`
	ManifestVersion string                   `json:"manifest_version"`
	GeneratedAt     string                   `json:"generated_at"`
	Entries         []capabilityCatalogEntry `json:"entries"`
}

type capabilityCatalogEntry struct {
	ID          string                 `json:"id"`
	Version     string                 `json:"version"`
	Descriptor  string                 `json:"descriptor"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Schemas     map[string]string      `json:"schemas"`
	Protocols   map[string]interface{} `json:"protocols"`
	Tags        []string               `json:"tags"`
	Execution   map[string]interface{} `json:"execution"`
}

type capabilityCatalogAsset struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
	Content  string `json:"content"`
}

func (h *handler) syncCapabilityCatalog(c *gin.Context) {
	if h.capabilitySync == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "capability sync worker disabled", nil)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "缺少有效租户上下文", err)
		return
	}
	var req capabilityCatalogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid capability catalog payload", err)
		return
	}
	artifactPath, cleanup, err := materializeCapabilityCatalog(req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	defer cleanup()

	worker := h.capabilitySync.WithTenant(tenantUUID)
	if err := worker.ProcessArtifact(c.Request.Context(), artifactPath); err != nil {
		dto.ResponseError(c, http.StatusBadGateway, "capability catalog sync failed", err)
		return
	}
	logger.Info(c.Request.Context(), "plugin capability catalog sync completed",
		zap.String("plugin_id", strings.TrimSpace(req.Catalog.PluginID)),
		zap.String("manifest_version", strings.TrimSpace(req.Catalog.ManifestVersion)),
		zap.Int("capability_count", len(req.Catalog.Entries)),
	)
	dto.ResponseSuccess(c, gin.H{
		"plugin_id":        req.Catalog.PluginID,
		"manifest_version": req.Catalog.ManifestVersion,
		"capability_count": len(req.Catalog.Entries),
	})
}

func materializeCapabilityCatalog(req capabilityCatalogRequest) (string, func(), error) {
	if strings.TrimSpace(req.Catalog.PluginID) == "" {
		return "", nil, fmt.Errorf("catalog.plugin_id is required")
	}
	if len(req.Catalog.Entries) == 0 {
		return "", nil, fmt.Errorf("catalog.entries is required")
	}
	tempDir, err := os.MkdirTemp("", "powerx-plugin-catalog-*")
	if err != nil {
		return "", nil, fmt.Errorf("create catalog temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }
	if err := writeCapabilityArtifact(tempDir, req); err != nil {
		cleanup()
		return "", nil, err
	}
	return tempDir, cleanup, nil
}

func writeCapabilityArtifact(root string, req capabilityCatalogRequest) error {
	if err := os.MkdirAll(filepath.Join(root, "capabilities"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "contracts", "exposure"), 0o755); err != nil {
		return err
	}
	manifest := map[string]interface{}{
		"id":      strings.TrimSpace(req.Catalog.PluginID),
		"name":    strings.TrimSpace(req.Catalog.PluginID),
		"version": firstNonEmpty(strings.TrimSpace(req.Catalog.ManifestVersion), "1.0.0"),
	}
	if err := writeYAML(filepath.Join(root, "plugin.yaml"), manifest); err != nil {
		return err
	}
	catalog := map[string]interface{}{
		"plugin": map[string]interface{}{
			"id":      strings.TrimSpace(req.Catalog.PluginID),
			"name":    strings.TrimSpace(req.Catalog.PluginID),
			"version": firstNonEmpty(strings.TrimSpace(req.Catalog.ManifestVersion), "1.0.0"),
		},
		"capabilities": buildSyncCapabilities(req.Catalog),
	}
	if err := writeJSON(filepath.Join(root, "capabilities", "catalog.json"), catalog); err != nil {
		return err
	}
	for _, asset := range req.Assets {
		if err := writeCapabilityAsset(root, asset); err != nil {
			return err
		}
	}
	return nil
}

func buildSyncCapabilities(catalog capabilityCatalogSnapshot) []map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]map[string]interface{}, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"id":           id,
			"title":        firstNonEmpty(strings.TrimSpace(entry.Title), id),
			"description":  strings.TrimSpace(entry.Description),
			"categories":   entry.Tags,
			"tool_scope":   entry.Tags,
			"protocols":    protocolsForSync(entry),
			"annotations":  map[string]interface{}{"descriptor": strings.TrimSpace(entry.Descriptor), "version": strings.TrimSpace(entry.Version)},
			"status":       "published",
			"published_at": now,
		})
	}
	return out
}

func protocolsForSync(entry capabilityCatalogEntry) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entry.Protocols))
	for channel, raw := range entry.Protocols {
		items := normalizeProtocolPayload(raw)
		for _, item := range items {
			item["channel"] = channel
			if endpoint := firstNonEmpty(stringFromAny(item["endpoint"]), stringFromAny(item["path"])); endpoint != "" {
				item["endpoint"] = endpoint
			}
			if schema := schemaRefForEntry(entry); schema != "" {
				item["schema_ref"] = schema
			}
			out = append(out, item)
		}
	}
	return out
}

func stringFromAny(value interface{}) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func normalizeProtocolPayload(raw interface{}) []map[string]interface{} {
	switch value := raw.(type) {
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(value))
		for _, item := range value {
			if m, ok := item.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	case map[string]interface{}:
		return []map[string]interface{}{value}
	default:
		return nil
	}
}

func schemaRefForEntry(entry capabilityCatalogEntry) string {
	if entry.Schemas == nil {
		return ""
	}
	return firstNonEmpty(entry.Schemas["input"], entry.Schemas["output"])
}

func writeCapabilityAsset(root string, asset capabilityCatalogAsset) error {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(asset.Path)))
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("invalid capability asset path: %s", asset.Path)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(asset.Content))
	if err != nil {
		return fmt.Errorf("decode capability asset %s: %w", asset.Path, err)
	}
	target := filepath.Join(root, rel)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(root)+string(os.PathSeparator)) {
		return fmt.Errorf("capability asset path escapes root: %s", asset.Path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, raw, 0o644)
}

func writeJSON(path string, value interface{}) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func writeYAML(path string, value interface{}) error {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
