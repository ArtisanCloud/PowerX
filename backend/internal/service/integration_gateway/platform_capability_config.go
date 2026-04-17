package integration_gateway

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gopkg.in/yaml.v3"
)

const (
	platformCapabilitiesDirEnv  = "PLATFORM_CAPABILITIES_DIR"
	defaultPlatformCapabilities = "backend/config/platform_capabilities"
)

type capabilityConfigFile struct {
	Capabilities []capabilityConfigEntry `yaml:"capabilities"`
}

type capabilityConfigEntry struct {
	CapabilityID string                    `yaml:"capability_id"`
	Title        string                    `yaml:"title"`
	Description  string                    `yaml:"description"`
	Module       string                    `yaml:"module"`
	Categories   []string                  `yaml:"categories"`
	Intents      []string                  `yaml:"intents"`
	ToolScopes   []string                  `yaml:"tool_scopes"`
	Policy       capabilityPolicy          `yaml:"policy"`
	Protocols    []capabilityProtocolEntry `yaml:"protocols"`
	Docs         []string                  `yaml:"docs"`
}

type capabilityProtocolEntry struct {
	Channel      string `yaml:"channel"`
	Endpoint     string `yaml:"endpoint"`
	Method       string `yaml:"method"`
	RPC          string `yaml:"rpc"`
	SchemaRef    string `yaml:"schema_ref"`
	ToolRef      string `yaml:"tool_ref"`
	ToolScope    string `yaml:"tool_scope"`
	AuthType     string `yaml:"auth_type"`
	HealthState  string `yaml:"health_state"`
	HealthReason string `yaml:"health_reason"`
}

func (entry capabilityConfigEntry) toDefinition() platformCapabilityDefinition {
	bindings := make([]models.ProtocolBinding, 0, len(entry.Protocols))
	for _, protocol := range entry.Protocols {
		bindings = append(bindings, models.ProtocolBinding{
			Channel:     protocol.Channel,
			Endpoint:    protocol.Endpoint,
			Method:      protocol.Method,
			RPC:         protocol.RPC,
			SchemaRef:   protocol.SchemaRef,
			ToolRef:     protocol.ToolRef,
			ToolScope:   protocol.ToolScope,
			AuthType:    protocol.AuthType,
			HealthState: protocol.HealthState,
		})
	}
	return platformCapabilityDefinition{
		CapabilityID: strings.TrimSpace(entry.CapabilityID),
		Title:        strings.TrimSpace(entry.Title),
		Description:  entry.Description,
		Module:       entry.Module,
		Categories:   entry.Categories,
		Intents:      entry.Intents,
		ToolScopes:   entry.ToolScopes,
		Policy:       entry.Policy,
		Protocols:    bindings,
		Docs:         entry.Docs,
	}
}

func loadPlatformCapabilityDefinitionsFromConfig() ([]platformCapabilityDefinition, error) {
	dir, err := resolvePlatformCapabilitiesDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read platform capability dir failed: %w", err)
	}
	var (
		defs     []platformCapabilityDefinition
		seen     = map[string]struct{}{}
		loadErrs []error
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".yaml") && !strings.HasSuffix(low, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		fileDefs, err := parsePlatformCapabilityFile(path)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		for _, def := range fileDefs {
			if def.CapabilityID == "" {
				loadErrs = append(loadErrs, fmt.Errorf("%s: capability_id is required", path))
				continue
			}
			if _, exists := seen[def.CapabilityID]; exists {
				loadErrs = append(loadErrs, fmt.Errorf("duplicate capability_id %s in %s", def.CapabilityID, path))
				continue
			}
			seen[def.CapabilityID] = struct{}{}
			defs = append(defs, def)
		}
	}
	if len(loadErrs) > 0 {
		return nil, errors.Join(loadErrs...)
	}
	return defs, nil
}

func parsePlatformCapabilityFile(path string) ([]platformCapabilityDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg capabilityConfigFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	defs := make([]platformCapabilityDefinition, 0, len(cfg.Capabilities))
	for _, entry := range cfg.Capabilities {
		defs = append(defs, entry.toDefinition())
	}
	return defs, nil
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
	seen := map[string]struct{}{}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		abs, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs, nil
		}
	}
	return "", fmt.Errorf("platform capabilities directory not found; tried %v", candidates)
}

func logPlatformCapabilityError(err error) {
	if err == nil {
		return
	}
	logger.WarnF(context.Background(), "[integration_gateway] load platform capabilities config failed: %v", err)
}
