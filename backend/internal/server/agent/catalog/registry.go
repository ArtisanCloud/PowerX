package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
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
	Apps       map[string]AppManifest      `yaml:"apps,omitempty"     json:"apps,omitempty"`
}

type AuthSpec struct {
	Scheme   string            `yaml:"scheme" json:"scheme"`                         // bearer|aksk|oauth2|tc3|...
	Fields   []string          `yaml:"fields,omitempty" json:"fields,omitempty"`     // ["api_key","base_url",...]
	Defaults map[string]string `yaml:"defaults,omitempty" json:"defaults,omitempty"` // e.g. {"base_url": "https://api.openai.com"}
	Modes    []AuthModeSpec    `yaml:"modes,omitempty" json:"modes,omitempty"`       // optional multi-mode auth
}

type AuthModeSpec struct {
	ID       string            `yaml:"id" json:"id"`
	Label    string            `yaml:"label,omitempty" json:"label,omitempty"`
	Scheme   string            `yaml:"scheme" json:"scheme"`
	Fields   []string          `yaml:"fields,omitempty" json:"fields,omitempty"`
	Defaults map[string]string `yaml:"defaults,omitempty" json:"defaults,omitempty"`
}

type ModalityManifest struct {
	Models []ModelManifest `yaml:"models" json:"models"`
}

type AppManifest struct {
	Name       string                      `yaml:"name,omitempty"     json:"name,omitempty"`
	Modalities map[string]ModalityManifest `yaml:"modalities"          json:"modalities"`
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

type AppItem struct {
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

func (r *Registry) Manifest(provider string) (*Manifest, bool) {
	pid := r.CanonicalProvider(provider)
	if pid == "" {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.providers[pid]
	return m, ok
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

func (r *Registry) loadDir(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		low := strings.ToLower(d.Name())
		if !strings.HasSuffix(low, ".yaml") && !strings.HasSuffix(low, ".yml") {
			return nil
		}
		if b, e := os.ReadFile(path); e == nil {
			if r.addManifestBytes(b) == nil {
				count++
			}
		}
		return nil
	})
	return count
}

func (r *Registry) LoadByConfig(cfg CatalogConfig, embedded fs.FS) error {
	//（省略 embedded）
	total := 0
	absDirs := make([]string, 0, len(cfg.Dirs))
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
		absDirs = append(absDirs, dir)
		total += r.loadDir(dir)
	}
	if total == 0 && cfg.FailIfEmpty {
		wd, _ := os.Getwd()
		return fmt.Errorf("no provider manifests loaded from dirs=%v (wd=%s)", absDirs, wd)
	}
	log.Printf("[ai.catalog] loaded %d provider manifests from dirs=%v", total, absDirs)
	return nil
}

// 规范化模态名：支持中文/别名/大小写
func NormalizeModality(s string) string {
	ls := strings.TrimSpace(strings.ToLower(s))
	switch ls {
	case "", "default": // 留给调用方决定是否默认 llm；这里不替换
		return ""
	// LLM
	case "llm", "text", "chat", "completion", "文本", "llm 文本", "llm文本":
		return "llm"
	// Image
	case "image", "vision", "img", "图像", "图像生成", "图片":
		return "image"
	// Embedding
	case "embedding", "embed", "vector", "向量", "嵌入", "向量嵌入":
		return "embedding"
	// Video
	case "video", "vid", "视频", "视频生成":
		return "video"
	// 3D Model Generation
	case "model3d", "3d", "3d生成", "3d 生成", "3d模型", "3d模型生成", "模型3d", "模型3d生成":
		return "model3d"
	// Audio（预留）
	case "audio", "speech", "声音", "语音":
		return "audio"
	}
	return ls
}

func (r *Registry) Providers(modality string) []ProviderItem {
	if r == nil {
		return nil
	}
	mod := NormalizeModality(modality)

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ProviderItem, 0, len(r.providers))
	// 支持 "*" 或空：返回所有有任一模态模型的 Provider（含 apps）
	if mod == "" || mod == "*" || mod == "any" {
		for id, m := range r.providers {
			if hasAnyModels(m) {
				out = append(out, ProviderItem{ID: id, Name: m.Name})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}

	for id, m := range r.providers {
		if hasModelsForModality(m, mod) {
			out = append(out, ProviderItem{ID: id, Name: m.Name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (r *Registry) Models(modality, provider string) ([]ModelManifest, error) {
	return r.ModelsByApp(modality, provider, "")
}

func (r *Registry) ModelsByApp(modality, provider, app string) ([]ModelManifest, error) {
	if r == nil {
		return nil, errors.New("catalog not initialized")
	}
	mod := NormalizeModality(modality)
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
	app = strings.TrimSpace(strings.ToLower(app))
	// 有 app：只在 app 内查
	if app != "" && m.Apps != nil {
		if am, ok := m.Apps[app]; ok {
			if mod == "" || mod == "*" || mod == "any" {
				var all []ModelManifest
				for _, mm := range am.Modalities {
					all = append(all, mm.Models...)
				}
				return all, nil
			}
			if mm, ok := am.Modalities[mod]; ok {
				return mm.Models, nil
			}
			return []ModelManifest{}, nil
		}
	}
	// 无 app：聚合 base + apps
	if mod == "" || mod == "*" || mod == "any" {
		var all []ModelManifest
		for _, mm := range m.Modalities {
			all = append(all, mm.Models...)
		}
		for _, am := range m.Apps {
			for _, mm := range am.Modalities {
				all = append(all, mm.Models...)
			}
		}
		return all, nil
	}
	var out []ModelManifest
	if mm, ok := m.Modalities[mod]; ok {
		out = append(out, mm.Models...)
	}
	for _, am := range m.Apps {
		if mm, ok := am.Modalities[mod]; ok {
			out = append(out, mm.Models...)
		}
	}
	return out, nil
}

func (r *Registry) Apps(provider, modality string) []AppItem {
	if r == nil {
		return nil
	}
	mod := NormalizeModality(modality)
	pid := r.CanonicalProvider(provider)
	if pid == "" {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := r.providers[pid]
	if m == nil || len(m.Apps) == 0 {
		return nil
	}
	out := make([]AppItem, 0, len(m.Apps))
	for id, app := range m.Apps {
		if mod == "" || mod == "*" || mod == "any" || hasAppModelsForModality(&app, mod) {
			name := app.Name
			if strings.TrimSpace(name) == "" {
				name = id
			}
			out = append(out, AppItem{ID: id, Name: name})
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

func hasAnyModels(m *Manifest) bool {
	if m == nil {
		return false
	}
	for _, mm := range m.Modalities {
		if len(mm.Models) > 0 {
			return true
		}
	}
	for _, am := range m.Apps {
		for _, mm := range am.Modalities {
			if len(mm.Models) > 0 {
				return true
			}
		}
	}
	return false
}

func hasModelsForModality(m *Manifest, modality string) bool {
	if m == nil {
		return false
	}
	if mm, ok := m.Modalities[modality]; ok && len(mm.Models) > 0 {
		return true
	}
	for _, am := range m.Apps {
		if mm, ok := am.Modalities[modality]; ok && len(mm.Models) > 0 {
			return true
		}
	}
	return false
}

func hasAppModelsForModality(a *AppManifest, modality string) bool {
	if a == nil {
		return false
	}
	if mm, ok := a.Modalities[modality]; ok && len(mm.Models) > 0 {
		return true
	}
	return false
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
	if len(cfg.Dirs) == 0 {
		cfg.Dirs = []string{"./config/agents/providers.d"}
	}
	log.Printf("[ai.catalog] initializing registry, dirs=%v include_embedded=%v", cfg.Dirs, cfg.IncludeEmbedded)
	reg := NewRegistry()
	if err := reg.LoadByConfig(cfg, embedded); err != nil {
		return err
	}
	if len(reg.providers) == 0 {
		wd, _ := os.Getwd()
		log.Printf("[ai.catalog] warning: registry empty after load (wd=%s)", wd)
	}
	GlobalAIRegister = reg

	// 可选：热更新
	// if cfg.HotReload { WatchDirs(cfg.Dirs...) }

	return nil
}

// ---------- 小工具 ----------

// internal/server/agent/catalog/registry.go
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
	p = os.ExpandEnv(p)
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd, _ := os.Getwd()
	trim := strings.TrimPrefix(p, "."+string(filepath.Separator))
	candidates := make([]string, 0, 6)
	if wd != "" {
		candidates = append(candidates, filepath.Clean(filepath.Join(wd, p)))
	}
	if linksRoot := strings.TrimSpace(os.Getenv("POWERX_LINKS_ROOT")); linksRoot != "" {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(linksRoot, trim)),
			filepath.Clean(filepath.Join(linksRoot, "backend", trim)),
		)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Clean(filepath.Join(exeDir, trim)),
			filepath.Clean(filepath.Join(filepath.Dir(exeDir), trim)),
		)
	}
	for _, candidate := range candidates {
		if pathExists(candidate) {
			return candidate
		}
	}
	clean := filepath.Clean(filepath.Join(wd, p))
	if root := findRepoRoot(wd); root != "" {
		if candidate := filepath.Clean(filepath.Join(root, trim)); pathExists(candidate) {
			return candidate
		}
		if candidate := filepath.Clean(filepath.Join(root, "backend", trim)); pathExists(candidate) {
			return candidate
		}
	}
	return clean
}

func pathExists(p string) bool {
	if p == "" {
		return false
	}
	if _, err := os.Stat(p); err == nil {
		return true
	}
	return false
}

func findRepoRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if dir == "" || dir == string(filepath.Separator) || dir == "." {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, ".specify")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
}
