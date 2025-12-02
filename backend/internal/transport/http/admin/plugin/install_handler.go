package plugin

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// POST /api/admin/plugins/install/local
// body: {"src_dir": "/absolute/or/relative/path", "enable": false}
type installLocalReq struct {
	SrcDir   string                     `json:"src_dir" binding:"required"`
	Enable   bool                       `json:"enable"`
	Force    bool                       `json:"force"`
	Metadata plugin_mgr.InstallMetadata `json:"metadata"`
}

func PluginInstallLocalHandler(c *gin.Context) {
	var req installLocalReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}

	// 解析 src_dir：支持直接传 tar.gz 包，或传入包含 package.tar.gz 的 build 目录，或包含 plugin.yaml 的目录
	srcDir, cleanup, err := resolveInstallSource(req.SrcDir)
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	mgr := mgrimpl.GetPluginManager() // 你走“实现包全局”
	meta := coalesceInstallMetadata(c, req.Metadata)
	p, err := mgr.InstallFromFile(c, srcDir, plugin_mgr.InstallOptions{
		AutoEnable: req.Enable,
		Force:      req.Force,
		Metadata:   meta,
	})
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}
	// 安装接口返回“刚安装的版本”（已在 InstallFromFile 内保证）
	dtoRequest.ResponseSuccess(c, gin.H{
		"installed": gin.H{
			"id":      p.ID,
			"version": p.Version,
			"state":   p.State,
		},
		"metadata": meta,
	})
}

// resolveInstallSource 尝试将传入路径解析为可安装目录：
// - 若是目录且包含 plugin.yaml：直接使用
// - 若是目录且包含 payload/plugin.yaml：使用 payload 子目录
// - 若目录下存在 package.tar.gz：解压到临时目录后使用 payload/ 或根目录
// - 若直接传入 .tar.gz 文件：解压到临时目录后使用
func resolveInstallSource(src string) (string, func(), error) {
	if strings.TrimSpace(src) == "" {
		return "", nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("srcDir is empty"))
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("resolve_src"))
	}
	stat, err := os.Stat(abs)
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("resolve_src"))
	}

	isDir := stat.IsDir()
	if isDir {
		// 目录直接包含 manifest
		if hasManifest(abs) {
			_ = ensureBackendBinsExecutable(abs)
			return abs, nil, nil
		}
		if payload := filepath.Join(abs, "payload"); hasManifest(payload) {
			_ = ensureBackendBinsExecutable(payload)
			return payload, nil, nil
		}
		// 目录下寻找 package.tar.gz
		if pkgPath := findPackage(abs); pkgPath != "" {
			return untarToTemp(pkgPath)
		}
		return "", nil, plugin_mgr.NewError(plugin_mgr.CodeMissingFile, plugin_mgr.WithMsg("plugin.yaml not found in srcDir"))
	}

	// 文件：若是 tar.gz 则解压
	if strings.HasSuffix(strings.ToLower(abs), ".tar.gz") {
		return untarToTemp(abs)
	}

	return "", nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("srcDir must be a directory with plugin.yaml or a package.tar.gz"))
}

func hasManifest(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "plugin.yaml"))
	return err == nil
}

func findPackage(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".tar.gz") && strings.Contains(name, "package") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func untarToTemp(pkgPath string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "px-plugin-install-*")
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_temp"))
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	f, err := os.Open(pkgPath)
	if err != nil {
		cleanup()
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_open"))
	}
	defer f.Close()
	gzr, err := gzip.NewReader(f)
	if err != nil {
		cleanup()
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeInvalidManifest, err, plugin_mgr.WithOp("untar_gzip"))
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_read"))
		}
		target := filepath.Join(tmpDir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				cleanup()
				return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_mkdir"))
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				cleanup()
				return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_mkdir_file"))
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				cleanup()
				return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_create"))
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				cleanup()
				return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("untar_copy"))
			}
			out.Close()
		}
	}

	// 选择 payload/plugin.yaml 或根目录 plugin.yaml
	payloadDir := filepath.Join(tmpDir, "payload")
	if hasManifest(payloadDir) {
		_ = ensureBackendBinsExecutable(payloadDir)
		return payloadDir, cleanup, nil
	}
	if hasManifest(tmpDir) {
		_ = ensureBackendBinsExecutable(tmpDir)
		return tmpDir, cleanup, nil
	}
	return "", cleanup, plugin_mgr.NewError(plugin_mgr.CodeInvalidManifest, plugin_mgr.WithMsg(fmt.Sprintf("plugin.yaml not found in package %s", pkgPath)))
}

// ensureBackendBinsExecutable makes sure backend/bin/* files are executable after extraction.
func ensureBackendBinsExecutable(root string) error {
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // ignore individual path errors
		}
		if d.Type().IsRegular() && strings.Contains(path, string(filepath.Separator)+"backend"+string(filepath.Separator)+"bin"+string(filepath.Separator)) {
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			_ = os.Chmod(path, info.Mode()|0o755)
		}
		return nil
	})
}

// --- Switch Version ---

// POST /api/admin/plugins/:id/switch_version
type switchVersionReq struct {
	Version string `json:"version" binding:"required"`
	Enable  bool   `json:"enable"`
}

func PluginSwitchVersionHandler(c *gin.Context) {
	var req switchVersionReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	id := c.Param("id")
	if id == "" {
		dtoRequest.ResponseError(c, 400, "缺少插件ID", nil)
		return
	}

	mgr := mgrimpl.GetPluginManager() // 走实现包的全局
	p, err := mgr.SwitchVersion(c, id, req.Version, req.Enable)
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "切换版本失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"id":      p.ID,
		"version": p.Version,
		"state":   p.State,
	})
}

// POST /api/admin/plugins/install/url
// body: {"url":"https://.../com.powerx.demo.hello_world-0.1.2.zip","sha256":"...","enable":true}
type installURLReq struct {
	URL      string                     `json:"url"     validate:"required,url"`
	SHA256   string                     `json:"sha256"`
	Enable   bool                       `json:"enable"`
	Sign     string                     `json:"signature"`
	Metadata plugin_mgr.InstallMetadata `json:"metadata"`
}

func PluginInstallURLHandler(c *gin.Context) {
	var req installURLReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	mgr := mgrimpl.GetPluginManager()

	// 安装（只登记、不自动启用）
	meta := coalesceInstallMetadata(c, req.Metadata)
	p, err := mgr.InstallFromURL(c, req.URL, req.SHA256, req.Sign, plugin_mgr.InstallOptions{
		VerifyChecksum:  req.SHA256 != "", // 传了就校验
		VerifySignature: false,            // 先关；后续接公钥再开
		AutoEnable:      req.Enable,
		Metadata:        meta,
	})
	if err != nil {
		dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
		return
	}

	// 可选：安装完切换并启用该版本
	if req.Enable {
		if _, err := mgr.SwitchVersion(c, p.ID, p.Version, true); err != nil {
			dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装成功但启用失败", err)
			return
		}
	}

	dtoRequest.ResponseSuccess(c, gin.H{
		"installed": gin.H{
			"id":      p.ID,
			"version": p.Version,
			"state":   string(p.State),
		},
		"enabled":  req.Enable,
		"metadata": meta,
	})
}

func coalesceInstallMetadata(c *gin.Context, body plugin_mgr.InstallMetadata) plugin_mgr.InstallMetadata {
	fromForm := plugin_mgr.InstallMetadata{}
	if raw := strings.TrimSpace(c.PostForm("metadata")); raw != "" {
		var parsed plugin_mgr.InstallMetadata
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			fromForm = parsed
		}
	}
	meta := mergeInstallMetadata(body, fromForm)
	return normalizeInstallMetadata(meta)
}

func mergeInstallMetadata(primary, secondary plugin_mgr.InstallMetadata) plugin_mgr.InstallMetadata {
	if isZeroMetadata(primary) {
		return secondary
	}
	if isZeroMetadata(secondary) {
		return primary
	}
	meta := primary
	if meta.Scope == "" {
		meta.Scope = secondary.Scope
	}
	if meta.Namespace == "" {
		meta.Namespace = secondary.Namespace
	}
	if meta.Environment == "" {
		meta.Environment = secondary.Environment
	}
	if !meta.AutoUpdate && secondary.AutoUpdate {
		meta.AutoUpdate = true
	}
	if !meta.Permissions.Network && secondary.Permissions.Network {
		meta.Permissions.Network = true
	}
	if !meta.Permissions.Storage && secondary.Permissions.Storage {
		meta.Permissions.Storage = true
	}
	if !meta.Permissions.Files && secondary.Permissions.Files {
		meta.Permissions.Files = true
	}
	if meta.Notes == "" {
		meta.Notes = secondary.Notes
	}
	return meta
}

func normalizeInstallMetadata(meta plugin_mgr.InstallMetadata) plugin_mgr.InstallMetadata {
	meta.Scope = normalizeScope(meta.Scope)
	if meta.Environment == "" {
		meta.Environment = "default"
	} else {
		meta.Environment = strings.ToLower(strings.TrimSpace(meta.Environment))
	}
	meta.Namespace = strings.TrimSpace(meta.Namespace)
	meta.Notes = strings.TrimSpace(meta.Notes)
	return meta
}

func isZeroMetadata(meta plugin_mgr.InstallMetadata) bool {
	return strings.TrimSpace(meta.Scope) == "" &&
		strings.TrimSpace(meta.Namespace) == "" &&
		strings.TrimSpace(meta.Environment) == "" &&
		!meta.AutoUpdate &&
		!meta.Permissions.Network &&
		!meta.Permissions.Storage &&
		!meta.Permissions.Files &&
		strings.TrimSpace(meta.Notes) == ""
}

func normalizeScope(scope string) string {
	s := strings.TrimSpace(strings.ToLower(scope))
	switch s {
	case "", "system", "systems", "sys", "system级", "系统级":
		return "system"
	case "user", "users", "user级", "用户级":
		return "user"
	case "org", "organisation", "organization", "tenant", "组织级", "tenant级":
		return "org"
	default:
		return s
	}
}
