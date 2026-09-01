package plugin_dev

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
	ID              string                 `json:"id"`
	Version         string                 `json:"version"`
	Descriptor      string                 `json:"descriptor"`
	Title           string                 `json:"title"`
	TitleI18n       map[string]string      `json:"title_i18n"`
	Description     string                 `json:"description"`
	DescriptionI18n map[string]string      `json:"description_i18n"`
	Schemas         map[string]string      `json:"schemas"`
	Protocols       map[string]interface{} `json:"protocols"`
	Tags            []string               `json:"tags"`
	Execution       map[string]interface{} `json:"execution"`
}

type catalogExposureDoc struct {
	Exposure struct {
		Channels []catalogExposureChannel `yaml:"channels"`
	} `yaml:"exposure"`
}

type catalogExposureChannel struct {
	Capability string                 `yaml:"capability"`
	RBAC       string                 `yaml:"rbac"`
	Security   map[string]interface{} `yaml:"security"`
}

type catalogDescriptorMetadata struct {
	Title           string
	TitleI18n       map[string]string
	Description     string
	DescriptionI18n map[string]string
	PermissionCodes []string
	Permissions     []catalogPluginPermissionDeclaration
	RiskLevel       string
	AgentUsable     *bool
}

type catalogDescriptorDoc struct {
	Title           string            `yaml:"title"`
	TitleI18n       map[string]string `yaml:"title_i18n"`
	Description     string            `yaml:"description"`
	DescriptionI18n map[string]string `yaml:"description_i18n"`
	Security        struct {
		PermissionCode string `yaml:"permission_code"`
		RiskLevel      string `yaml:"risk_level"`
	} `yaml:"security"`
	Agent struct {
		Usable    *bool  `yaml:"usable"`
		RiskLevel string `yaml:"risk_level"`
	} `yaml:"agent"`
	RBAC struct {
		Resource string   `yaml:"resource"`
		Actions  []string `yaml:"actions"`
	} `yaml:"rbac"`
	Permissions []catalogPluginPermissionDeclaration `yaml:"permissions"`
}

type catalogPluginPermissionDeclaration struct {
	Type                   string                             `json:"type" yaml:"type"`
	PermissionCode         string                             `json:"permission_code" yaml:"permission_code"`
	Module                 string                             `json:"module,omitempty" yaml:"module"`
	TitleI18n              map[string]string                  `json:"title_i18n" yaml:"title_i18n"`
	DescriptionI18n        map[string]string                  `json:"description_i18n" yaml:"description_i18n"`
	RiskLevel              string                             `json:"risk_level" yaml:"risk_level"`
	DataScope              string                             `json:"data_scope,omitempty" yaml:"data_scope"`
	DefaultRoleGrants      []string                           `json:"default_role_grants,omitempty" yaml:"default_role_grants"`
	BusinessPermissionCode string                             `json:"business_permission_code,omitempty" yaml:"business_permission_code"`
	Independent            bool                               `json:"independent,omitempty" yaml:"independent"`
	ProtocolBindings       []catalogPermissionProtocolBinding `json:"protocol_bindings,omitempty" yaml:"protocol_bindings"`
}

type catalogPermissionProtocolBinding struct {
	Channel       string `json:"channel" yaml:"channel"`
	Method        string `json:"method" yaml:"method"`
	Path          string `json:"path" yaml:"path"`
	ActorContext  string `json:"actor_context" yaml:"actor_context"`
	ResourceScope string `json:"resource_scope" yaml:"resource_scope"`
}

type localPermissionSnapshot struct {
	PluginID        string   `json:"plugin_id"`
	Source          string   `json:"source"`
	PermissionCodes []string `json:"permission_codes"`
	PermsHash       string   `json:"perms_hash"`
	PolicyVersion   string   `json:"policy_version"`
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
	localSnapshot, err := localPermissionSnapshotFromCatalog(req)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
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
		"plugin_id":                 req.Catalog.PluginID,
		"manifest_version":          req.Catalog.ManifestVersion,
		"capability_count":          len(req.Catalog.Entries),
		"local_permission_snapshot": localSnapshot,
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
	exposurePermissions, err := permissionCodesFromCatalogAssets(req.Catalog.PluginID, req.Assets)
	if err != nil {
		return err
	}
	descriptorMetadata, err := descriptorMetadataFromCatalogAssets(req.Catalog.PluginID, req.Catalog.Entries, req.Assets)
	if err != nil {
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
		"capabilities": buildSyncCapabilities(req.Catalog, exposurePermissions, descriptorMetadata),
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

func buildSyncCapabilities(catalog capabilityCatalogSnapshot, exposurePermissions map[string][]string, descriptors map[string]catalogDescriptorMetadata) []map[string]interface{} {
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]map[string]interface{}, 0, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		id := strings.TrimSpace(entry.ID)
		if id == "" {
			continue
		}
		descriptor := descriptors[id]
		titleI18n := firstCatalogLocaleMap(entry.TitleI18n, descriptor.TitleI18n)
		descriptionI18n := firstCatalogLocaleMap(entry.DescriptionI18n, descriptor.DescriptionI18n)
		permissionCodes := dedupeCatalogStrings(append(append(permissionCodesFromEntry(catalog.PluginID, entry), exposurePermissions[id]...), descriptor.PermissionCodes...))
		annotations := map[string]interface{}{
			"descriptor":       strings.TrimSpace(entry.Descriptor),
			"version":          strings.TrimSpace(entry.Version),
			"permission_codes": permissionCodes,
			"agent_usable":     firstCatalogBool(descriptor.AgentUsable, true),
			"title_i18n":       titleI18n,
			"description_i18n": descriptionI18n,
		}
		if risk := strings.TrimSpace(descriptor.RiskLevel); risk != "" {
			annotations["risk_level"] = risk
		}
		permissions := normalizeCatalogPluginPermissions(descriptor.Permissions)
		if len(permissions) > 0 {
			annotations["permissions"] = permissions
		}
		out = append(out, map[string]interface{}{
			"id":           id,
			"title":        firstNonEmpty(strings.TrimSpace(entry.Title), strings.TrimSpace(descriptor.Title), firstCatalogLocaleValue(titleI18n), id),
			"description":  firstNonEmpty(strings.TrimSpace(entry.Description), strings.TrimSpace(descriptor.Description), firstCatalogLocaleValue(descriptionI18n)),
			"categories":   entry.Tags,
			"tool_scope":   permissionCodes,
			"protocols":    protocolsForSync(entry),
			"permissions":  permissions,
			"annotations":  annotations,
			"status":       "published",
			"published_at": now,
		})
	}
	return out
}

func descriptorMetadataFromCatalogAssets(pluginID string, entries []capabilityCatalogEntry, assets []capabilityCatalogAsset) (map[string]catalogDescriptorMetadata, error) {
	assetByPath := make(map[string]capabilityCatalogAsset, len(assets))
	for _, asset := range assets {
		rel := cleanCatalogAssetPath(asset.Path)
		if rel == "" {
			continue
		}
		assetByPath[rel] = asset
	}
	out := make(map[string]catalogDescriptorMetadata)
	for _, entry := range entries {
		capabilityID := strings.TrimSpace(entry.ID)
		descriptorPath := cleanCatalogAssetPath(entry.Descriptor)
		if capabilityID == "" || descriptorPath == "" {
			continue
		}
		asset, ok := assetByPath[descriptorPath]
		if !ok {
			return nil, fmt.Errorf("capability descriptor asset missing: capability=%s descriptor=%s", capabilityID, descriptorPath)
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(asset.Content))
		if err != nil {
			return nil, fmt.Errorf("decode capability descriptor %s: %w", descriptorPath, err)
		}
		var doc catalogDescriptorDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse capability descriptor %s: %w", descriptorPath, err)
		}
		out[capabilityID] = catalogDescriptorMetadata{
			Title:           strings.TrimSpace(doc.Title),
			TitleI18n:       cleanCatalogLocaleMap(doc.TitleI18n),
			Description:     strings.TrimSpace(doc.Description),
			DescriptionI18n: cleanCatalogLocaleMap(doc.DescriptionI18n),
			PermissionCodes: permissionCodesFromDescriptor(pluginID, doc),
			Permissions:     normalizeCatalogPluginPermissions(doc.Permissions),
			RiskLevel:       firstNonEmpty(strings.TrimSpace(doc.Agent.RiskLevel), strings.TrimSpace(doc.Security.RiskLevel)),
			AgentUsable:     doc.Agent.Usable,
		}
	}
	return out, nil
}

func localPermissionSnapshotFromCatalog(req capabilityCatalogRequest) (localPermissionSnapshot, error) {
	descriptors, err := descriptorMetadataFromCatalogAssets(req.Catalog.PluginID, req.Catalog.Entries, req.Assets)
	if err != nil {
		return localPermissionSnapshot{}, err
	}
	codes := make([]string, 0)
	for _, descriptor := range descriptors {
		for _, permission := range descriptor.Permissions {
			code := strings.TrimSpace(permission.PermissionCode)
			if code == "" {
				continue
			}
			codes = append(codes, code)
		}
	}
	codes = dedupeCatalogStrings(codes)
	sort.Strings(codes)
	hash := localPermissionCodesHash(codes)
	return localPermissionSnapshot{
		PluginID:        strings.TrimSpace(req.Catalog.PluginID),
		Source:          "local_mock",
		PermissionCodes: codes,
		PermsHash:       hash,
		PolicyVersion:   localPermissionPolicyVersion(hash),
	}, nil
}

func normalizeCatalogPluginPermissions(in []catalogPluginPermissionDeclaration) []catalogPluginPermissionDeclaration {
	if len(in) == 0 {
		return nil
	}
	out := make([]catalogPluginPermissionDeclaration, 0, len(in))
	for _, permission := range in {
		permission.Type = strings.TrimSpace(permission.Type)
		permission.PermissionCode = strings.TrimSpace(permission.PermissionCode)
		permission.Module = strings.TrimSpace(permission.Module)
		permission.TitleI18n = cleanCatalogLocaleMap(permission.TitleI18n)
		permission.DescriptionI18n = cleanCatalogLocaleMap(permission.DescriptionI18n)
		permission.RiskLevel = strings.TrimSpace(permission.RiskLevel)
		permission.DataScope = strings.TrimSpace(permission.DataScope)
		permission.BusinessPermissionCode = strings.TrimSpace(permission.BusinessPermissionCode)
		permission.DefaultRoleGrants = dedupeCatalogStrings(permission.DefaultRoleGrants)
		for idx, binding := range permission.ProtocolBindings {
			permission.ProtocolBindings[idx] = catalogPermissionProtocolBinding{
				Channel:       strings.TrimSpace(binding.Channel),
				Method:        strings.ToUpper(strings.TrimSpace(binding.Method)),
				Path:          strings.TrimSpace(binding.Path),
				ActorContext:  strings.TrimSpace(binding.ActorContext),
				ResourceScope: strings.TrimSpace(binding.ResourceScope),
			}
		}
		out = append(out, permission)
	}
	return out
}

func localPermissionCodesHash(items []string) string {
	codes := dedupeCatalogStrings(items)
	sort.Strings(codes)
	sum := sha256.Sum256([]byte(strings.Join(codes, "\n")))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func localPermissionPolicyVersion(permsHash string) string {
	permsHash = strings.TrimSpace(permsHash)
	if permsHash == "" {
		return ""
	}
	return "iam:" + permsHash
}

func permissionCodesFromDescriptor(pluginID string, doc catalogDescriptorDoc) []string {
	codes := make([]string, 0)
	if code := strings.TrimSpace(doc.Security.PermissionCode); code != "" {
		codes = append(codes, code)
	}
	resource := strings.TrimSpace(doc.RBAC.Resource)
	for _, action := range doc.RBAC.Actions {
		action = strings.TrimSpace(action)
		if resource == "" || action == "" {
			continue
		}
		if code := permissionCodeFromCatalogRBAC(pluginID, resource+":"+action); code != "" {
			codes = append(codes, code)
		}
	}
	return dedupeCatalogStrings(codes)
}

func cleanCatalogAssetPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

func firstCatalogLocaleMap(values ...map[string]string) map[string]string {
	for _, value := range values {
		if cleaned := cleanCatalogLocaleMap(value); len(cleaned) > 0 {
			return cleaned
		}
	}
	return nil
}

func firstCatalogBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func cleanCatalogLocaleMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for locale, value := range in {
		locale = strings.TrimSpace(locale)
		value = strings.TrimSpace(value)
		if locale == "" || value == "" {
			continue
		}
		out[locale] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstCatalogLocaleValue(in map[string]string) string {
	cleaned := cleanCatalogLocaleMap(in)
	for _, locale := range []string{"zh-CN", "zh", "en", "en-US", "ja", "ko"} {
		if value := strings.TrimSpace(cleaned[locale]); value != "" {
			return value
		}
	}
	for _, value := range cleaned {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func permissionCodesFromCatalogAssets(pluginID string, assets []capabilityCatalogAsset) (map[string][]string, error) {
	out := map[string][]string{}
	for _, asset := range assets {
		rel := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(asset.Path))))
		if rel != "plugin.d/exposure.yaml" && rel != "plugin.d/exposure.yml" && rel != "plugin.merged.yaml" && rel != "plugin.merged.yml" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(asset.Content))
		if err != nil {
			return nil, fmt.Errorf("decode capability asset %s: %w", asset.Path, err)
		}
		var doc catalogExposureDoc
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse exposure asset %s: %w", asset.Path, err)
		}
		for _, channel := range doc.Exposure.Channels {
			capabilityID := strings.TrimSpace(channel.Capability)
			if capabilityID == "" {
				continue
			}
			code := permissionCodeFromCatalogExposure(pluginID, channel)
			if code == "" {
				continue
			}
			out[capabilityID] = append(out[capabilityID], code)
		}
	}
	for capabilityID, codes := range out {
		out[capabilityID] = dedupeCatalogStrings(codes)
	}
	return out, nil
}

func permissionCodesFromEntry(pluginID string, entry capabilityCatalogEntry) []string {
	codes := make([]string, 0)
	for _, raw := range entry.Protocols {
		for _, item := range normalizeProtocolPayload(raw) {
			if code := strings.TrimSpace(firstNonEmpty(stringFromAny(item["permission_code"]), stringFromAny(item["permissionCode"]))); code != "" {
				codes = append(codes, code)
			}
			if code := permissionCodeFromCatalogRBAC(pluginID, stringFromAny(item["rbac"])); code != "" {
				codes = append(codes, code)
			}
			if code := strings.TrimSpace(firstNonEmpty(stringFromAny(item["tool_scope"]), stringFromAny(item["toolScope"]))); code != "" {
				codes = append(codes, code)
			}
		}
	}
	return codes
}

func permissionCodeFromCatalogExposure(pluginID string, channel catalogExposureChannel) string {
	if channel.Security != nil {
		if code := strings.TrimSpace(fmt.Sprint(channel.Security["permission_code"])); code != "" && code != "<nil>" {
			return code
		}
		if code := strings.TrimSpace(fmt.Sprint(channel.Security["permissionCode"])); code != "" && code != "<nil>" {
			return code
		}
	}
	return permissionCodeFromCatalogRBAC(pluginID, channel.RBAC)
}

func permissionCodeFromCatalogRBAC(pluginID, rbac string) string {
	resource, action, ok := strings.Cut(strings.TrimSpace(rbac), ":")
	if !ok {
		return ""
	}
	resource = strings.TrimSpace(resource)
	action = strings.TrimSpace(action)
	pluginID = strings.TrimSpace(pluginID)
	if resource == "" || action == "" || pluginID == "" {
		return ""
	}
	return pluginID + "." + resource + ":" + action
}

func dedupeCatalogStrings(items []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(items))
	for _, item := range items {
		normalized := strings.TrimSpace(item)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
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
