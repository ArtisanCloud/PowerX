package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

const PluginManifestFile = "plugin.yaml"

type Descriptor struct {
	Manifest plugin_mgr.Manifest
	Paths    plugin_mgr.InstalledPaths
}

type Loader interface {
	Discover(ctx context.Context, installedRoot string) ([]Descriptor, error)
	LoadDescriptor(ctx context.Context, root string) (Descriptor, error)
	Validate(ctx context.Context, m plugin_mgr.Manifest, root string) error
}

type FSLoader struct{}

func NewFSLoader() *FSLoader { return &FSLoader{} }

func (l *FSLoader) Discover(ctx context.Context, installedRoot string) ([]Descriptor, error) {
	out := make([]Descriptor, 0, 8)

	// 如果目录不存在，视为空
	if fi, err := os.Stat(installedRoot); err != nil || !fi.IsDir() {
		return out, nil
	}

	idDirs, _ := os.ReadDir(installedRoot)
	for _, idEntry := range idDirs {
		if !idEntry.IsDir() {
			continue
		}
		idPath := filepath.Join(installedRoot, idEntry.Name())
		verDirs, _ := os.ReadDir(idPath)
		for _, verEntry := range verDirs {
			if !verEntry.IsDir() {
				continue
			}
			root := filepath.Join(idPath, verEntry.Name())
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			default:
			}
			desc, err := l.LoadDescriptor(ctx, root)
			if err != nil {
				// 跳过坏包；你也可以改为返回错误
				continue
			}
			out = append(out, desc)
		}
	}
	return out, nil
}

func (l *FSLoader) LoadDescriptor(ctx context.Context, root string) (Descriptor, error) {
	absRoot, _ := filepath.Abs(root) // ★ 保证根目录是绝对路径
	path := filepath.Join(absRoot, PluginManifestFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Descriptor{}, plugin_mgr.Wrap(plugin_mgr.CodeMissingFile, err,
			plugin_mgr.WithOp("load_descriptor"), plugin_mgr.WithPath(path))
	}

	var m plugin_mgr.Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Descriptor{}, plugin_mgr.Wrap(plugin_mgr.CodeInvalidManifest, err,
			plugin_mgr.WithOp("load_descriptor"), plugin_mgr.WithPath(path))
	}

	// 组装 Paths（把相对路径转为绝对/规范路径）
	p := plugin_mgr.InstalledPaths{
		Root: absRoot, // ★ 存绝对
	}
	if m.Frontend.Admin.StaticDir != "" {
		p.FrontendAdminDir = filepath.Clean(filepath.Join(root, m.Frontend.Admin.StaticDir))
	}
	if m.Runtime.Entry != "" {
		p.Entry = filepath.Clean(filepath.Join(root, m.Runtime.Entry))
	}
	if m.Migrations != nil && m.Migrations.Dir != "" {
		p.MigrationsDir = filepath.Clean(filepath.Join(root, m.Migrations.Dir))
	}
	// 默认 contracts 位置（有就填）
	openapi := filepath.Join(root, "contracts", "openapi.yaml")
	if fi, err := os.Stat(openapi); err == nil && !fi.IsDir() {
		p.ContractsOpenAPI = openapi
	}
	protoDir := filepath.Join(root, "contracts", "proto")
	if fi, err := os.Stat(protoDir); err == nil && fi.IsDir() {
		p.ContractsProtoDir = protoDir
	}
	pub := filepath.Join(root, "public")
	if fi, err := os.Stat(pub); err == nil && fi.IsDir() {
		p.PublicDir = pub
	}

	// 静态校验
	if err := l.Validate(ctx, m, root); err != nil {
		return Descriptor{}, err
	}

	return Descriptor{Manifest: m, Paths: p}, nil
}

func (l *FSLoader) Validate(ctx context.Context, m plugin_mgr.Manifest, root string) error {
	var ferrs []plugin_mgr.FieldError

	req := func(field, val string) {
		if strings.TrimSpace(val) == "" {
			ferrs = append(ferrs, plugin_mgr.FieldError{Field: field, Reason: "required"})
		}
	}

	req("id", m.ID)
	req("version", m.Version)
	switch m.Runtime.Kind {
	case plugin_mgr.RuntimeKindProcess, plugin_mgr.RuntimeKindStatic, plugin_mgr.RuntimeKindRemote:
	default:
		ferrs = append(ferrs, plugin_mgr.FieldError{Field: "runtime.kind", Reason: "unsupported"})
	}

	// process 特有：entry 必须存在
	if m.Runtime.Kind == plugin_mgr.RuntimeKindProcess {
		req("runtime.entry", m.Runtime.Entry)
		if m.Runtime.Entry != "" {
			entry := filepath.Join(root, m.Runtime.Entry)
			if fi, err := os.Stat(entry); err != nil || fi.IsDir() {
				ferrs = append(ferrs, plugin_mgr.FieldError{Field: "runtime.entry", Reason: "not found"})
			}
		}
		// 建议有 http_base_path
		if strings.TrimSpace(m.Endpoints.HTTPBasePath) == "" {
			ferrs = append(ferrs, plugin_mgr.FieldError{Field: "endpoints.http_base_path", Reason: "required"})
		}
	}

	// admin static
	if m.Frontend.Admin.Kind == plugin_mgr.FrontendKindStatic {
		req("frontend.admin.static_dir", m.Frontend.Admin.StaticDir)
		if m.Frontend.Admin.StaticDir != "" {
			static := filepath.Join(root, m.Frontend.Admin.StaticDir)
			if fi, err := os.Stat(static); err != nil || !fi.IsDir() {
				ferrs = append(ferrs, plugin_mgr.FieldError{Field: "frontend.admin.static_dir", Reason: "not found"})
			}
		}
	}

	// http_base_path 格式
	if m.Endpoints.HTTPBasePath != "" && !strings.HasPrefix(m.Endpoints.HTTPBasePath, "/") {
		ferrs = append(ferrs, plugin_mgr.FieldError{Field: "endpoints.http_base_path", Reason: "must start with '/'"})
	}

	// 解析 duration
	parseDur := func(s string) error {
		if strings.TrimSpace(s) == "" {
			return nil
		}
		_, err := time.ParseDuration(s)
		return err
	}
	if err := parseDur(m.Runtime.Health.Interval); err != nil {
		ferrs = append(ferrs, plugin_mgr.FieldError{Field: "runtime.health.interval", Reason: "invalid duration"})
	}
	if err := parseDur(m.Runtime.Health.Timeout); err != nil {
		ferrs = append(ferrs, plugin_mgr.FieldError{Field: "runtime.health.timeout", Reason: "invalid duration"})
	}

	// migrations 目录存在
	if m.Migrations != nil && m.Migrations.Dir != "" {
		mig := filepath.Join(root, m.Migrations.Dir)
		if fi, err := os.Stat(mig); err != nil || !fi.IsDir() {
			ferrs = append(ferrs, plugin_mgr.FieldError{Field: "migrations.dir", Reason: "not found"})
		}
	}

	if len(ferrs) > 0 {
		ve := &plugin_mgr.ValidationError{
			Op:     "validate_manifest",
			Fields: ferrs,
			// 可选填充 ID/Version，便于日志
			PluginID: m.ID,
			Version:  m.Version,
		}
		return ve.ToManagerError()
	}
	return nil
}
