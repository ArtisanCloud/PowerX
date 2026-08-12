package apikeypermissions

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"gopkg.in/yaml.v3"
)

const (
	platformCapabilitiesDirEnv  = "PLATFORM_CAPABILITIES_DIR"
	defaultPlatformCapabilities = "backend/config/platform_capabilities"
	platformPermissionSource    = "platform_capability"
)

var (
	platformPermissionOnce sync.Once
	platformPermissionRows []modelsiam.Permission
	platformPermissionErr  error
)

type platformCapabilityFile struct {
	Capabilities []platformCapabilityEntry `yaml:"capabilities"`
}

type platformCapabilityEntry struct {
	CapabilityID    string                           `yaml:"capability_id"`
	Module          string                           `yaml:"module"`
	Title           string                           `yaml:"title"`
	Description     string                           `yaml:"description"`
	PermissionCode  string                           `yaml:"permission_code"`
	TitleI18n       map[string]string                `yaml:"title_i18n"`
	DescriptionI18n map[string]string                `yaml:"description_i18n"`
	Protocols       []platformCapabilityProtocolItem `yaml:"protocols"`
}

type platformCapabilityProtocolItem struct {
	Channel  string `yaml:"channel"`
	Endpoint string `yaml:"endpoint"`
	Method   string `yaml:"method"`
}

func BuildPlatformCapabilityPermissions() ([]modelsiam.Permission, error) {
	platformPermissionOnce.Do(func() {
		platformPermissionRows, platformPermissionErr = loadPlatformCapabilityPermissions()
	})
	if platformPermissionErr != nil {
		return nil, platformPermissionErr
	}
	out := make([]modelsiam.Permission, len(platformPermissionRows))
	copy(out, platformPermissionRows)
	return out, nil
}

func RESTPermissionTriple(moduleHint, method, endpoint string) (module, resource, action string, ok bool) {
	endpoint = canonicalEndpoint(endpoint)
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" || endpoint == "" {
		return "", "", "", false
	}
	module = strings.TrimSpace(moduleHint)
	if module == "" {
		module = moduleFromEndpoint(endpoint)
	}
	action = actionFromHTTPMethod(method, endpoint)
	resource = resourceFromEndpoint(endpoint)
	if module == "" || resource == "" || action == "" {
		return "", "", "", false
	}
	return module, resource, action, true
}

func loadPlatformCapabilityPermissions() ([]modelsiam.Permission, error) {
	dir, err := resolvePlatformCapabilitiesDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read platform capabilities dir failed: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)

	out := make([]modelsiam.Permission, 0, 512)
	seenOperations := make(map[string]struct{}, 512)
	seenAPIs := make(map[string]struct{}, 1024)
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s failed: %w", path, readErr)
		}
		var file platformCapabilityFile
		if unmarshalErr := yaml.Unmarshal(raw, &file); unmarshalErr != nil {
			return nil, fmt.Errorf("parse %s failed: %w", path, unmarshalErr)
		}
		generatedCandidateFile := isGeneratedPlatformCapabilityFile(path)
		for _, capItem := range file.Capabilities {
			if !generatedCandidateFile {
				if err := validateFormalPlatformCapabilityPermissionMeta(path, capItem); err != nil {
					return nil, err
				}
				if operation, ok := platformCapabilityOperationPermission(capItem); ok {
					key := operation.Module + "|" + operation.Resource + "|" + operation.Action
					if _, exists := seenOperations[key]; !exists {
						seenOperations[key] = struct{}{}
						out = append(out, operation)
					}
				}
			}
			for _, proto := range capItem.Protocols {
				if !strings.EqualFold(strings.TrimSpace(proto.Channel), "rest") {
					continue
				}
				method := strings.ToUpper(strings.TrimSpace(proto.Method))
				endpoint := canonicalEndpoint(proto.Endpoint)
				if method == "" || endpoint == "" {
					continue
				}

				module, resource, action, ok := RESTPermissionTriple(capItem.Module, method, endpoint)
				if !ok {
					continue
				}

				key := module + "|" + resource + "|" + action
				if _, ok := seenAPIs[key]; ok {
					continue
				}
				seenAPIs[key] = struct{}{}

				if generatedCandidateFile {
					out = append(out, generatedPlatformCapabilityCandidate(module, resource, action, method, endpoint, capItem))
					continue
				}

				titleI18n := normalizedLocaleMap(capItem.TitleI18n)
				descriptionI18n := normalizedLocaleMap(capItem.DescriptionI18n)
				meta := map[string]any{
					"type":             "api",
					"module":           module,
					"label":            preferredLocaleText(titleI18n),
					"title_i18n":       titleI18n,
					"description_i18n": descriptionI18n,
					"http_method":      method,
					"api_endpoint":     endpoint,
					"capability_id":    strings.TrimSpace(capItem.CapabilityID),
					"permission_code":  strings.TrimSpace(capItem.PermissionCode),
					"source":           "platform_capabilities",
				}
				metaBytes, _ := json.Marshal(meta)

				permission := modelsiam.Permission{
					Module:      module,
					Resource:    resource,
					Action:      action,
					Effect:      "allow",
					Description: preferredLocaleText(descriptionI18n),
					Meta:        metaBytes,
					Status:      modelsiam.PermissionStatusActive,
					Source:      platformPermissionSource,
					Introduced:  IntroducedVersion(),
				}
				permission.AllowAPIKey = DefaultAllowAPIKey(permission)
				if permission.AllowAPIKey {
					if apiMeta := BuildAPIKeyMeta(permission); len(apiMeta) > 0 {
						meta["api_key"] = apiMeta
					}
				}
				metaBytes, _ = json.Marshal(meta)
				permission.Meta = metaBytes

				out = append(out, permission)
			}
		}
	}
	return out, nil
}

func platformCapabilityOperationPermission(capItem platformCapabilityEntry) (modelsiam.Permission, bool) {
	module, resource, action, ok := operationTripleFromPermissionCode(capItem.Module, capItem.PermissionCode)
	if !ok {
		return modelsiam.Permission{}, false
	}
	titleI18n := normalizedLocaleMap(capItem.TitleI18n)
	descriptionI18n := normalizedLocaleMap(capItem.DescriptionI18n)
	meta := map[string]any{
		"type":             "action",
		"module":           module,
		"label":            preferredLocaleText(titleI18n),
		"title_i18n":       titleI18n,
		"description_i18n": descriptionI18n,
		"capability_id":    strings.TrimSpace(capItem.CapabilityID),
		"permission_code":  strings.TrimSpace(capItem.PermissionCode),
		"source":           "platform_capabilities",
	}
	metaBytes, _ := json.Marshal(meta)
	return modelsiam.Permission{
		Module:      module,
		Resource:    resource,
		Action:      action,
		Effect:      "allow",
		Description: preferredLocaleText(descriptionI18n),
		Meta:        metaBytes,
		Status:      modelsiam.PermissionStatusActive,
		Source:      platformPermissionSource,
		Introduced:  IntroducedVersion(),
		AllowAPIKey: false,
	}, true
}

func operationTripleFromPermissionCode(moduleHint, permissionCode string) (module, resource, action string, ok bool) {
	module = sanitizeToken(moduleHint)
	code := strings.TrimSpace(permissionCode)
	parts := strings.Split(code, ":")
	if len(parts) != 2 {
		return "", "", "", false
	}
	left := strings.TrimSpace(parts[0])
	action = sanitizeToken(parts[1])
	if strings.HasPrefix(left, "corex.") {
		left = strings.TrimPrefix(left, "corex.")
	}
	if module != "" {
		prefix := module + "."
		if strings.HasPrefix(left, prefix) {
			left = strings.TrimPrefix(left, prefix)
		}
	}
	resource = sanitizeToken(left)
	if resource == "" && module != "" {
		resource = module
	}
	return module, resource, action, module != "" && resource != "" && action != ""
}

func validateFormalPlatformCapabilityPermissionMeta(path string, capItem platformCapabilityEntry) error {
	capabilityID := strings.TrimSpace(capItem.CapabilityID)
	if capabilityID == "" {
		return fmt.Errorf("%s: capability_id is required", path)
	}
	if strings.TrimSpace(capItem.PermissionCode) == "" {
		return fmt.Errorf("%s: capability %s permission_code is required", path, capabilityID)
	}
	if preferredLocaleText(normalizedLocaleMap(capItem.TitleI18n)) == "" {
		return fmt.Errorf("%s: capability %s title_i18n is required for formal api permission", path, capabilityID)
	}
	if preferredLocaleText(normalizedLocaleMap(capItem.DescriptionI18n)) == "" {
		return fmt.Errorf("%s: capability %s description_i18n is required for formal api permission", path, capabilityID)
	}
	return nil
}

func isGeneratedPlatformCapabilityFile(path string) bool {
	return strings.EqualFold(filepath.Base(path), "generated.auto.yaml")
}

func generatedPlatformCapabilityCandidate(module, resource, action, method, endpoint string, capItem platformCapabilityEntry) modelsiam.Permission {
	meta := map[string]any{
		"type":                "api_candidate",
		"module":              module,
		"http_method":         method,
		"api_endpoint":        endpoint,
		"capability_id":       strings.TrimSpace(capItem.CapabilityID),
		"permission_code":     strings.TrimSpace(capItem.PermissionCode),
		"generated_from":      "platform_capability_generated",
		"registration_status": "invalid",
		"registration_errors": []string{"api_permission_platform_capability_missing", "api_permission_i18n_missing"},
		"title_i18n":          map[string]string{},
		"description_i18n":    map[string]string{},
	}
	metaBytes, _ := json.Marshal(meta)
	return modelsiam.Permission{
		Module:      module,
		Resource:    resource,
		Action:      action,
		Effect:      "allow",
		Meta:        metaBytes,
		Status:      modelsiam.PermissionStatusDeprecated,
		Source:      "platform_capability_generated",
		Introduced:  IntroducedVersion(),
		AllowAPIKey: false,
	}
}

func normalizedLocaleMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for locale, value := range values {
		key := strings.TrimSpace(locale)
		text := strings.TrimSpace(value)
		if key != "" && text != "" {
			out[key] = text
		}
	}
	return out
}

func preferredLocaleText(values map[string]string) string {
	for _, locale := range []string{"zh-CN", "zh", "en", "en-US"} {
		if text := strings.TrimSpace(values[locale]); text != "" {
			return text
		}
	}
	for _, text := range values {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func resolvePlatformCapabilitiesDir() (string, error) {
	candidates := make([]string, 0, 5)
	if custom := strings.TrimSpace(os.Getenv(platformCapabilitiesDirEnv)); custom != "" {
		candidates = append(candidates, custom)
	}
	candidates = append(candidates,
		filepath.Join(".", defaultPlatformCapabilities),
		filepath.Join("..", defaultPlatformCapabilities),
	)
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, defaultPlatformCapabilities),
			filepath.Join(execDir, "..", defaultPlatformCapabilities),
		)
	}
	for _, cand := range candidates {
		if strings.TrimSpace(cand) == "" {
			continue
		}
		abs, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("platform capabilities directory not found")
}

func canonicalEndpoint(endpoint string) string {
	v := strings.TrimSpace(endpoint)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	for strings.Contains(v, "//") {
		v = strings.ReplaceAll(v, "//", "/")
	}
	if len(v) > 1 && strings.HasSuffix(v, "/") {
		v = strings.TrimSuffix(v, "/")
	}
	return v
}

func moduleFromEndpoint(endpoint string) string {
	segs := strings.Split(strings.Trim(endpoint, "/"), "/")
	idx := 0
	if len(segs) > idx && segs[idx] == "api" {
		idx++
	}
	if len(segs) > idx && strings.HasPrefix(strings.ToLower(segs[idx]), "v") {
		idx++
	}
	if len(segs) > idx && segs[idx] == "admin" {
		idx++
	}
	if len(segs) <= idx || strings.TrimSpace(segs[idx]) == "" {
		return "core"
	}
	return sanitizeToken(segs[idx])
}

func actionFromHTTPMethod(method, endpoint string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET":
		if strings.Contains(endpoint, "{") || strings.Contains(endpoint, "}") || strings.Contains(endpoint, ":") {
			return "read"
		}
		return "list"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return strings.ToLower(strings.TrimSpace(method))
	}
}

func resourceFromEndpoint(endpoint string) string {
	trimmed := strings.Trim(endpoint, "/")
	if trimmed == "" {
		return "root"
	}
	sanitized := sanitizeToken(trimmed)
	if strings.HasPrefix(sanitized, "api_v1_") {
		sanitized = strings.TrimPrefix(sanitized, "api_v1_")
	}
	if len(sanitized) == 0 {
		return "misc"
	}
	if len(sanitized) <= 64 {
		return sanitized
	}
	sum := sha1.Sum([]byte(sanitized))
	suffix := hex.EncodeToString(sum[:4])
	head := sanitized[:55]
	return head + "_" + suffix
}

func sanitizeToken(value string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"{", "",
		"}", "",
		":", "",
		"-", "_",
		".", "_",
	)
	v := replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	for strings.Contains(v, "__") {
		v = strings.ReplaceAll(v, "__", "_")
	}
	return strings.Trim(v, "_")
}
