package catalog

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ---------- 配置结构（用于主 YAML 反序列化） ----------

type CatalogConfig struct {
	Dirs            []string `yaml:"dirs"             mapstructure:"dirs"`
	IncludeEmbedded bool     `yaml:"include_embedded" mapstructure:"include_embedded"`
	FailIfEmpty     bool     `yaml:"fail_if_empty"    mapstructure:"fail_if_empty"`
	HotReload       bool     `yaml:"hot_reload"       mapstructure:"hot_reload"`
}

// ---------- Manifest 模型 ----------

type Manifest struct {
	ID         string                      `yaml:"id"   json:"id"`
	Name       string                      `yaml:"name" json:"name"`
	Aliases    []string                    `yaml:"aliases,omitempty"  json:"aliases,omitempty"`
	Drivers    map[string]string           `yaml:"drivers,omitempty"  json:"drivers,omitempty"` // modality -> driverKey
	Auth       AuthSpec                    `yaml:"auth"               json:"auth"`
	Modalities map[string]ModalityManifest `yaml:"modalities"         json:"modalities"`
}

type AuthSpec struct {
	Scheme string   `yaml:"scheme" json:"scheme"`                     // bearer|aksk|oauth2|...
	Fields []string `yaml:"fields,omitempty" json:"fields,omitempty"` // ["api_key","base_url",...]
}

type ModalityManifest struct {
	Models []ModelManifest `yaml:"models" json:"models"`
}

type ModelManifest struct {
	ID       string         `yaml:"id"                 json:"id"`
	Label    string         `yaml:"label"              json:"label"`
	Tags     []string       `yaml:"tags,omitempty"     json:"tags,omitempty"`
	Defaults map[string]any `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Limits   map[string]int `yaml:"limits,omitempty"   json:"limits,omitempty"`
}

// ---------- Registry ----------

type ProviderItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Registry struct {
	mu        sync.RWMutex
	providers map[string]*Manifest // canonical id -> manifest
	alias     map[string]string    // alias(lower) -> canonical id
}

func NewRegistry() *Registry {
	return &Registry{
		providers: map[string]*Manifest{},
		alias:     map[string]string{},
	}
}

func canonical(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "-")
}

func (r *Registry) addManifestBytes(b []byte) error {
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.ID == "" || m.Name == "" {
		return errors.New("invalid manifest: missing id/name")
	}
	id := canonical(m.ID)
	r.mu.Lock()
	r.providers[id] = &m
	// 建立别名映射
	r.alias[strings.ToLower(m.Name)] = id
	r.alias[strings.ToLower(m.ID)] = id
	for _, a := range m.Aliases {
		r.alias[strings.ToLower(a)] = id
	}
	r.mu.Unlock()
	return nil
}

func (r *Registry) loadDir(dir string) {
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		low := strings.ToLower(d.Name())
		if !strings.HasSuffix(low, ".yaml") && !strings.HasSuffix(low, ".yml") {
			return nil
		}
		if b, e := os.ReadFile(path); e == nil {
			_ = r.addManifestBytes(b) // 忽略单文件错误
		}
		return nil
	})
}

// LoadByConfig：只按主配置加载（不读 env，不用默认目录）
func (r *Registry) LoadByConfig(cfg CatalogConfig, embedded fs.FS) error {
	// 1) 可选：先加载内置（若你准备了 go:embed，可以在 embedded 中遍历并 add）
	if cfg.IncludeEmbedded && embedded != nil {
		// 示例：遍历 embedded 根目录下 *.yaml（如果你有的话）
		fs.WalkDir(embedded, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			low := strings.ToLower(d.Name())
			if !strings.HasSuffix(low, ".yaml") && !strings.HasSuffix(low, ".yml") {
				return nil
			}
			if b, e := fs.ReadFile(embedded, path); e == nil {
				_ = r.addManifestBytes(b)
			}
			return nil
		})
	}

	// 2) 按顺序加载外部目录（后者覆盖前者）
	seen := map[string]struct{}{}
	for _, raw := range cfg.Dirs {
		dir := expandPath(raw)
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		r.loadDir(dir)
	}

	if len(r.providers) == 0 && cfg.FailIfEmpty {
		return errors.New("no provider manifests loaded from dirs")
	}
	return nil
}

// Providers: 按模态列出可用 Provider（有该模态且模型非空）
func (r *Registry) Providers(modality string) []ProviderItem {
	if r == nil {
		return nil
	}
	mod := strings.ToLower(strings.TrimSpace(modality))
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ProviderItem, 0, len(r.providers))
	for id, m := range r.providers {
		if mm, ok := m.Modalities[mod]; ok && len(mm.Models) > 0 {
			out = append(out, ProviderItem{ID: id, Name: m.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) CanonicalProvider(nameOrAlias string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.alias[strings.ToLower(strings.TrimSpace(nameOrAlias))]
}

func (r *Registry) Models(modality, provider string) ([]ModelManifest, error) {
	if r == nil {
		return nil, errors.New("catalog not initialized")
	}
	mod := strings.ToLower(strings.TrimSpace(modality))
	pid := r.CanonicalProvider(provider)
	if pid == "" {
		return nil, errors.New("unknown provider: " + provider)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := r.providers[pid]
	if m == nil {
		return nil, errors.New("provider not found: " + pid)
	}
	mm, ok := m.Modalities[mod]
	if !ok {
		return []ModelManifest{}, nil
	}
	return mm.Models, nil
}

// Global：全局注册表（启动时初始化）
var GlobalAIRegister *Registry

// Get: 永远返回非 nil 的 *Registry，防止调用方 panic
func GetGlobalAIRegister() *Registry {
	if GlobalAIRegister == nil {
		return NewRegistry() // 空注册表（返回空结果，而不是崩溃）
	}
	return GlobalAIRegister
}

// InitFromAppConfig：由你的主配置调用
func InitFromAppConfig(cfg CatalogConfig, embedded fs.FS) error {
	reg := NewRegistry()
	if err := reg.LoadByConfig(cfg, embedded); err != nil {
		return err
	}
	GlobalAIRegister = reg

	// 可选：热更新
	// if cfg.HotReload { WatchDirs(cfg.Dirs...) }

	return nil
}

// ---------- 小工具 ----------

func expandPath(p string) string {
	if p == "" {
		return ""
	}
	// ~ 展开
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	// 环境变量展开
	p = os.ExpandEnv(p)
	// 转绝对路径（可选）
	return p
}
