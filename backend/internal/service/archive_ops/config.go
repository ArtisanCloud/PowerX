package archive_ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadConfig 只解析运维配置，不加载认证初始化、不依赖业务数据库可连接。
func LoadConfig(path string) (Config, error) {
	var c Config
	raw, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	d := yaml.NewDecoder(strings.NewReader(string(raw)))
	d.KnownFields(true)
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	return c, nil
}
func DefaultDirectory() string {
	if root := os.Getenv("POWERX_LINKS_ROOT"); root != "" {
		return filepath.Join(root, "storage", "archives")
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".powerx", "storage", "archives")
}
func validateConfig(c *Config) error {
	if c.Directory == "" {
		c.Directory = DefaultDirectory()
	}
	if !filepath.IsAbs(c.Directory) {
		return fmt.Errorf("archive directory must be absolute")
	}
	if c.Instance == "" {
		c.Instance = os.Getenv("POWERX_ENV")
		if c.Instance == "" {
			c.Instance = "local"
		}
	}
	if !labelPattern.MatchString(c.Instance) {
		return fmt.Errorf("invalid archive instance identifier")
	}
	if c.ExecutionNodeID == "" {
		c.ExecutionNodeID = c.Instance
	}
	if !labelPattern.MatchString(c.ExecutionNodeID) {
		return fmt.Errorf("invalid archive execution node")
	}
	seen := map[string]bool{}
	for _, source := range c.Sources {
		if !labelPattern.MatchString(source.ID) || seen[source.ID] {
			return fmt.Errorf("invalid or duplicate source identifier")
		}
		seen[source.ID] = true
		if source.Kind != "media" && !isLogSource(source.Kind) && source.Kind != "database_files" {
			return fmt.Errorf("unsupported archive source kind")
		}
		if isLogSource(source.Kind) && source.Kind != "postgresql_logs" && source.RotationFormat == "" {
			return fmt.Errorf("sealed log rotation format required")
		}
		if source.RotationFormat != "" {
			if !isLogSource(source.Kind) || (source.RotationFormat != "numeric" && source.RotationFormat != "lumberjack" && source.RotationFormat != "dated" && source.RotationFormat != "journal" && source.RotationFormat != "postgresql") {
				return fmt.Errorf("sealed log rotation format invalid")
			}
			if source.RotationFormat == "postgresql" && (source.Kind != "postgresql_logs" || !filepath.IsAbs(source.CurrentLogfiles)) {
				return fmt.Errorf("native PostgreSQL logs require an absolute current_logfiles manifest")
			}
			if len(source.ActiveFiles) > 16 || (source.RotationFormat != "journal" && len(source.ActiveFiles) == 0) {
				return fmt.Errorf("sealed log active files required")
			}
			for _, name := range source.ActiveFiles {
				if name == "" || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
					return fmt.Errorf("sealed log active name invalid")
				}
			}
		}
		if !filepath.IsAbs(source.Directory) {
			return fmt.Errorf("source directory must be absolute")
		}
		if source.CurrentLogfiles != "" && !filepath.IsAbs(source.CurrentLogfiles) {
			return fmt.Errorf("current_logfiles must be absolute")
		}
	}
	for ref, cred := range c.Credentials {
		if !labelPattern.MatchString(ref) || !filepath.IsAbs(cred.KeyFile) || !filepath.IsAbs(cred.KnownHostsFile) {
			return fmt.Errorf("invalid SSH credential reference")
		}
	}
	return nil
}
func marshalConfig(c Config) ([]byte, error) { return yaml.Marshal(c) }
