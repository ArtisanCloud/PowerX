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
	CapabilityID string                           `yaml:"capability_id"`
	Module       string                           `yaml:"module"`
	Title        string                           `yaml:"title"`
	Protocols    []platformCapabilityProtocolItem `yaml:"protocols"`
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
	seen := make(map[string]struct{}, 1024)
	for _, path := range paths {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read %s failed: %w", path, readErr)
		}
		var file platformCapabilityFile
		if unmarshalErr := yaml.Unmarshal(raw, &file); unmarshalErr != nil {
			return nil, fmt.Errorf("parse %s failed: %w", path, unmarshalErr)
		}
		for _, capItem := range file.Capabilities {
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
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}

				meta := map[string]any{
					"type":          "api",
					"module":        module,
					"label":         strings.TrimSpace(capItem.Title),
					"http_method":   method,
					"api_endpoint":  endpoint,
					"capability_id": strings.TrimSpace(capItem.CapabilityID),
					"source":        "platform_capabilities",
				}
				if strings.TrimSpace(anyToString(meta["label"])) == "" {
					meta["label"] = method + " " + endpoint
				}
				metaBytes, _ := json.Marshal(meta)

				permission := modelsiam.Permission{
					Module:      module,
					Resource:    resource,
					Action:      action,
					Effect:      "allow",
					Description: method + " " + endpoint,
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
