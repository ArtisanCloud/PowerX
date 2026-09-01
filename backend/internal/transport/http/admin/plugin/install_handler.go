package plugin

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pmimplnotify "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/notify"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// POST /api/admin/plugins/install/local
// body: {"src_dir": "/absolute/or/relative/path", "enable": false}
type installLocalReq struct {
	SrcDir   string                     `json:"src_dir"`
	Enable   bool                       `json:"enable"`
	Force    bool                       `json:"force"`
	Metadata plugin_mgr.InstallMetadata `json:"metadata"`
}

func PluginInstallLocalHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var (
			req     installLocalReq
			srcPath string
			cleanup func()
			err     error
		)

		// multipart 优先：文件选择器上传
		if strings.HasPrefix(strings.ToLower(c.ContentType()), "multipart/form-data") {
			req.Enable = parseBool(c.PostForm("enable"))
			req.Force = parseBool(c.PostForm("force"))

			var uploadedPath string
			fileHeader, ferr := c.FormFile("file")
			if ferr == nil && fileHeader != nil {
				uploadedPath, cleanup, err = saveUploadedFileToTemp(fileHeader)
			} else {
				var formErr error
				var form *multipart.Form
				form, formErr = c.MultipartForm()
				if formErr != nil || form == nil || len(form.File["files"]) == 0 {
					dtoRequest.ResponseError(c, 400, "安装失败", plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("file or files is required")))
					return
				}
				relPaths := uploadedDirRelPaths(form)
				logLocalInstallUpload(c, "multipart_received", "", form.File["files"], relPaths, nil)
				uploadedPath, cleanup, err = saveUploadedDirToTemp(form.File["files"], relPaths)
			}
			if err != nil {
				dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
				return
			}
			logLocalInstallResolve(c, "uploaded_source_saved", uploadedPath)
			var resolveCleanup func()
			srcPath, resolveCleanup, err = resolveInstallSource(uploadedPath)
			if err != nil {
				logLocalInstallResolve(c, "uploaded_source_resolve_failed", uploadedPath)
				if cleanup != nil {
					cleanup()
					cleanup = nil
				}
				dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
				return
			}
			if resolveCleanup != nil {
				prevCleanup := cleanup
				cleanup = func() {
					resolveCleanup()
					if prevCleanup != nil {
						prevCleanup()
					}
				}
			}
		} else {
			// JSON 兼容：服务端本地路径安装
			if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
				dtoRequest.ResponseValidationError(c, err)
				return
			}
			srcPath, cleanup, err = resolveInstallSource(req.SrcDir)
			if err != nil {
				dtoRequest.ResponseError(c, plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err)), "安装失败", err)
				return
			}
		}
		if cleanup != nil {
			defer cleanup()
		}

		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		ctx := c.Request.Context()
		meta, err := coalesceInstallMetadata(c, req.Metadata)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "PLUGIN_INSTALL_METADATA_INVALID", err)
			return
		}
		hostSeed, err := installTenantHostConfigSeed(c)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "PLUGIN_INSTALL_TENANT_CONTEXT_REQUIRED", err)
			return
		}
		p, err := mgr.InstallFromFile(ctx, srcPath, plugin_mgr.InstallOptions{
			AutoEnable:     req.Enable,
			Force:          req.Force,
			HostConfigSeed: hostSeed,
			Metadata:       meta,
		})
		if err != nil {
			respondPluginInstallError(c, "安装失败", err)
			return
		}
		seeded := 0
		var tenantInstance any
		if req.Enable {
			if err := completeReadyDrainJobsAfterRuntimeEnabled(c, deps, p.ID); err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：Drain 状态恢复失败", err)
				return
			}
			tenantInstance, err = enableInstalledPluginForCurrentTenant(c, deps, p)
			if err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：租户插件启用失败", err)
				return
			}
			seeded, err = ensureEnabledTenantEventFabricTopics(c, deps, p.ID)
			if err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：Topic 注册失败", err)
				return
			}
		}
		// 安装接口返回“刚安装的版本”（已在 InstallFromFile 内保证）
		dtoRequest.ResponseSuccess(c, gin.H{
			"installed": gin.H{
				"id":      p.ID,
				"version": p.Version,
				"state":   p.State,
			},
			"enabled":                     req.Enable,
			"tenant_instance":             tenantInstance,
			"event_fabric_seeded_tenants": seeded,
			"metadata":                    meta,
		})
	}
}

func logLocalInstallUpload(c *gin.Context, stage string, uploadedPath string, files []*multipart.FileHeader, relPaths []string, err error) {
	pxlog.Info(c.Request.Context(), "plugin.local_install.upload",
		zap.String("stage", stage),
		zap.String("content_type", c.ContentType()),
		zap.Int("files_count", len(files)),
		zap.Int("file_paths_count", len(relPaths)),
		zap.Strings("sample_file_names", sampleUploadFileNames(files, 8)),
		zap.Strings("sample_file_paths", sampleStrings(relPaths, 8)),
		zap.String("uploaded_path", uploadedPath),
		zap.Error(err),
	)
}

func uploadedDirRelPaths(form *multipart.Form) []string {
	if form == nil {
		return nil
	}
	if relPaths := form.Value["file_paths"]; len(relPaths) > 0 {
		return relPaths
	}
	raw := strings.TrimSpace(firstString(form.Value["file_paths_json"]))
	if raw == "" {
		return nil
	}
	var relPaths []string
	if err := json.Unmarshal([]byte(raw), &relPaths); err != nil {
		return nil
	}
	return relPaths
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func logLocalInstallResolve(c *gin.Context, stage string, src string) {
	fields := []zap.Field{
		zap.String("stage", stage),
		zap.String("src", src),
	}
	if abs, err := filepath.Abs(src); err == nil {
		fields = append(fields, zap.String("abs", abs))
		if st, statErr := os.Stat(abs); statErr == nil {
			fields = append(fields, zap.Bool("src_exists", true), zap.Bool("src_is_dir", st.IsDir()))
		} else {
			fields = append(fields, zap.Bool("src_exists", false), zap.String("src_stat_error", statErr.Error()))
		}
		manifestPath := filepath.Join(abs, "plugin.yaml")
		if st, statErr := os.Stat(manifestPath); statErr == nil {
			fields = append(fields, zap.Bool("manifest_exists", true), zap.Int64("manifest_size", st.Size()))
		} else {
			fields = append(fields, zap.Bool("manifest_exists", false), zap.String("manifest_stat_error", statErr.Error()))
		}
		payloadManifestPath := filepath.Join(abs, "payload", "plugin.yaml")
		if st, statErr := os.Stat(payloadManifestPath); statErr == nil {
			fields = append(fields, zap.Bool("payload_manifest_exists", true), zap.Int64("payload_manifest_size", st.Size()))
		} else {
			fields = append(fields, zap.Bool("payload_manifest_exists", false), zap.String("payload_manifest_stat_error", statErr.Error()))
		}
	}
	pxlog.Info(c.Request.Context(), "plugin.local_install.resolve", fields...)
}

func sampleUploadFileNames(files []*multipart.FileHeader, limit int) []string {
	if limit <= 0 || len(files) == 0 {
		return nil
	}
	if len(files) < limit {
		limit = len(files)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if files[i] == nil {
			out = append(out, "")
			continue
		}
		out = append(out, files[i].Filename)
	}
	return out
}

func sampleStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) < limit {
		limit = len(values)
	}
	out := make([]string, limit)
	copy(out, values[:limit])
	return out
}

func parseBool(v string) bool {
	ok, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false
	}
	return ok
}

func saveUploadedFileToTemp(fileHeader *multipart.FileHeader) (string, func(), error) {
	in, err := fileHeader.Open()
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_open"))
	}
	defer in.Close()

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".tar.gz") {
		ext = ".tar.gz"
	}
	tmpFile, err := os.CreateTemp("", "px-plugin-upload-*"+ext)
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_temp"))
	}
	defer tmpFile.Close()
	if _, err := io.Copy(tmpFile, in); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_copy"))
	}
	return tmpFile.Name(), func() {
		_ = os.Remove(tmpFile.Name())
	}, nil
}

func saveUploadedDirToTemp(files []*multipart.FileHeader, relPathsInput []string) (string, func(), error) {
	tmpRoot, err := os.MkdirTemp("", "px-plugin-upload-dir-*")
	if err != nil {
		return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_dir_temp"))
	}
	cleanup := func() {
		_ = os.RemoveAll(tmpRoot)
	}
	relPaths := make([]string, 0, len(files))
	hasPathHints := len(relPathsInput) == len(files)
	for _, fh := range files {
		relRaw := strings.TrimSpace(fh.Filename)
		if hasPathHints {
			relRaw = strings.TrimSpace(relPathsInput[len(relPaths)])
		}
		rel := filepath.Clean(relRaw)
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "..") {
			cleanup()
			return "", nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("invalid uploaded directory entry"))
		}
		target := filepath.Join(tmpRoot, rel)
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget, tmpRoot+string(filepath.Separator)) && cleanTarget != tmpRoot {
			cleanup()
			return "", nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("invalid uploaded directory path"))
		}

		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			cleanup()
			return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_dir_mkdir"))
		}
		in, err := fh.Open()
		if err != nil {
			cleanup()
			return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_dir_open"))
		}
		out, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			in.Close()
			cleanup()
			return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_dir_create"))
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			in.Close()
			cleanup()
			return "", nil, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("upload_dir_copy"))
		}
		out.Close()
		in.Close()
		relPaths = append(relPaths, rel)
	}
	return detectUploadedRoot(tmpRoot, relPaths), cleanup, nil
}

func detectUploadedRoot(tmpRoot string, relPaths []string) string {
	if len(relPaths) == 0 {
		return tmpRoot
	}
	root := strings.Split(relPaths[0], string(filepath.Separator))[0]
	if root == "" || root == "." {
		return tmpRoot
	}
	for _, rel := range relPaths[1:] {
		seg := strings.Split(rel, string(filepath.Separator))[0]
		if seg != root {
			return tmpRoot
		}
	}
	candidate := filepath.Join(tmpRoot, root)
	if st, err := os.Stat(candidate); err == nil && st.IsDir() {
		return candidate
	}
	return tmpRoot
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
	if isSupportedPackageFile(abs) {
		return untarToTemp(abs)
	}

	return "", nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("srcDir must be a directory with plugin.yaml or a package.tar.gz/.tgz"))
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
		if isSupportedPackageFile(name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func isSupportedPackageFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz")
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
	return "", cleanup, plugin_mgr.NewError(
		plugin_mgr.CodeInvalidManifest,
		plugin_mgr.WithMsg("plugin.yaml not found in package"),
		plugin_mgr.WithPath(pkgPath),
	)
}

// ensureBackendBinsExecutable makes sure packaged backend entries are executable after extraction.
func ensureBackendBinsExecutable(root string) error {
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // ignore individual path errors
		}
		if !d.Type().IsRegular() || !isPluginExecutablePath(root, path) {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		_ = os.Chmod(path, info.Mode()|0o755)
		return nil
	})
}

func isPluginExecutablePath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == filepath.Join("bin", "plugin") || rel == filepath.Join("bin", "migrate") {
		return true
	}
	return strings.HasPrefix(rel, filepath.Join("backend", "bin")+string(filepath.Separator))
}

// --- Switch Version ---

// POST /api/admin/plugins/:id/switch_version
type switchVersionReq struct {
	Version string `json:"version" binding:"required"`
	Enable  bool   `json:"enable"`
}

func PluginSwitchVersionHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		p, err := mgr.SwitchVersion(c.Request.Context(), id, req.Version, req.Enable)
		if err != nil {
			respondPluginInstallError(c, "切换版本失败", err)
			return
		}
		seeded := 0
		if req.Enable {
			seeded, err = ensureEnabledTenantEventFabricTopics(c, deps, id)
			if err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "切换版本失败：Topic 注册失败", err)
				return
			}
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"id":                          p.ID,
			"version":                     p.Version,
			"state":                       p.State,
			"event_fabric_seeded_tenants": seeded,
		})
	}
}

// POST /api/admin/plugins/install/url
// body: {"url":"https://.../com.powerx.demo.hello_world-0.1.2.zip","sha256":"...","enable":true}
type installURLReq struct {
	URL      string                     `json:"url"     validate:"required,url"`
	SHA256   string                     `json:"sha256"`
	Enable   bool                       `json:"enable"`
	Force    bool                       `json:"force"`
	Sign     string                     `json:"signature"`
	Metadata plugin_mgr.InstallMetadata `json:"metadata"`
}

func PluginInstallURLHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req installURLReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		mgr, err := tryGetPluginManager()
		if err != nil {
			respondPluginRuntimeUnavailable(c, err)
			return
		}
		ctx := c.Request.Context()

		// 安装（只登记、不自动启用）
		meta, err := coalesceInstallMetadata(c, req.Metadata)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "PLUGIN_INSTALL_METADATA_INVALID", err)
			return
		}
		hostSeed, err := installTenantHostConfigSeed(c)
		if err != nil {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "PLUGIN_INSTALL_TENANT_CONTEXT_REQUIRED", err)
			return
		}
		p, err := mgr.InstallFromURL(ctx, req.URL, req.SHA256, req.Sign, plugin_mgr.InstallOptions{
			VerifyChecksum:  req.SHA256 != "", // 传了就校验
			VerifySignature: false,            // 先关；后续接公钥再开
			AutoEnable:      req.Enable,
			Force:           req.Force,
			HostConfigSeed:  hostSeed,
			Metadata:        meta,
		})
		if err != nil {
			respondPluginInstallError(c, "安装失败", err)
			return
		}

		// 可选：安装完切换并启用该版本
		if req.Enable {
			if _, err := mgr.SwitchVersion(ctx, p.ID, p.Version, true); err != nil {
				respondPluginInstallError(c, "安装成功但启用失败", err)
				return
			}
		}
		seeded := 0
		var tenantInstance any
		if req.Enable {
			if err := completeReadyDrainJobsAfterRuntimeEnabled(c, deps, p.ID); err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：Drain 状态恢复失败", err)
				return
			}
			tenantInstance, err = enableInstalledPluginForCurrentTenant(c, deps, p)
			if err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：租户插件启用失败", err)
				return
			}
			seeded, err = ensureEnabledTenantEventFabricTopics(c, deps, p.ID)
			if err != nil {
				dtoRequest.ResponseError(c, http.StatusInternalServerError, "安装失败：Topic 注册失败", err)
				return
			}
		}

		dtoRequest.ResponseSuccess(c, gin.H{
			"installed": gin.H{
				"id":      p.ID,
				"version": p.Version,
				"state":   string(p.State),
			},
			"enabled":                     req.Enable,
			"tenant_instance":             tenantInstance,
			"event_fabric_seeded_tenants": seeded,
			"metadata":                    meta,
		})
	}
}

func completeReadyDrainJobsAfterRuntimeEnabled(c *gin.Context, deps *shared.Deps, pluginID string) error {
	if deps == nil || deps.DB == nil {
		return plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("database dependency missing"))
	}
	_, err := reposetting.NewPluginDrainJobRepository(deps.DB).MarkReadyJobsCompletedByPlugin(c.Request.Context(), pluginID, time.Now().UTC())
	return err
}

func enableInstalledPluginForCurrentTenant(c *gin.Context, deps *shared.Deps, p plugin_mgr.Plugin) (any, error) {
	if deps == nil || deps.DB == nil {
		return nil, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("database dependency missing"))
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		return nil, err
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	svc := pluginservice.NewTenantPluginInstanceService(deps.DB)
	instance, clientID, clientSecret, err := svc.Enable(c.Request.Context(), tenantUUID, p, nil)
	if err != nil {
		return nil, err
	}
	if clientSecret != "" {
		_ = pmimplnotify.PushTenantCredentials(c, p.ID, tenantUUID, clientID, clientSecret)
	}
	return instance, nil
}

func respondPluginInstallError(c *gin.Context, message string, err error) {
	status := plugin_mgr.HTTPStatusOf(plugin_mgr.CodeOf(err))
	if details := platformPreflightErrorDetails(err); len(details) > 0 {
		dtoRequest.ResponseErrorWithDetails(c, status, "plugin package platform incompatible", nil, details)
		return
	}
	if plugin_mgr.IsCode(err, plugin_mgr.CodeConflict) && strings.Contains(strings.ToLower(err.Error()), "tenant instances") {
		pluginID := ""
		version := ""
		var managerErr *plugin_mgr.ManagerError
		if errors.As(err, &managerErr) && managerErr != nil {
			pluginID = managerErr.PluginID
			version = managerErr.Version
		}
		dtoRequest.ResponseErrorWithDetails(c, status, message, nil, gin.H{
			"code":           "PLUGIN_DRAIN_REQUIRED",
			"plugin_id":      pluginID,
			"version":        version,
			"reason":         "active_tenant_instances",
			"requires_drain": true,
			"hint":           "当前插件仍有租户实例在使用。请先发起并完成 drain，再重新安装或切换版本。",
		})
		return
	}
	dtoRequest.ResponseError(c, status, message, err)
}

func platformPreflightErrorDetails(err error) gin.H {
	var managerErr *plugin_mgr.ManagerError
	if !errors.As(err, &managerErr) || managerErr == nil {
		return nil
	}
	if managerErr.Op != "install_file.platform_preflight" {
		return nil
	}
	return gin.H{
		"code":      "PLUGIN_PACKAGE_PLATFORM_INCOMPATIBLE",
		"plugin_id": managerErr.PluginID,
		"version":   managerErr.Version,
		"path":      managerErr.Path,
		"reason":    managerErr.Msg,
		"hint":      "Select a plugin artifact built for the current PowerX host platform.",
	}
}

func coalesceInstallMetadata(c *gin.Context, body plugin_mgr.InstallMetadata) (plugin_mgr.InstallMetadata, error) {
	if len(body.DeprecatedEnvironment) > 0 {
		return plugin_mgr.InstallMetadata{}, errors.New("PLUGIN_INSTALL_METADATA_ENVIRONMENT_DEPRECATED: use metadata.release_channel")
	}
	fromForm := plugin_mgr.InstallMetadata{}
	if raw := strings.TrimSpace(c.PostForm("metadata")); raw != "" {
		var parsed plugin_mgr.InstallMetadata
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return plugin_mgr.InstallMetadata{}, fmt.Errorf("PLUGIN_INSTALL_METADATA_INVALID_JSON: %w", err)
		}
		if len(parsed.DeprecatedEnvironment) > 0 {
			return plugin_mgr.InstallMetadata{}, errors.New("PLUGIN_INSTALL_METADATA_ENVIRONMENT_DEPRECATED: use metadata.release_channel")
		}
		fromForm = parsed
	}
	meta := mergeInstallMetadata(body, fromForm)
	return normalizeInstallMetadata(meta), nil
}

func installTenantHostConfigSeed(c *gin.Context) (*plugin_mgr.HostConfig, error) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		return nil, err
	}
	tenantUUID, err = reqctx.CanonicalTenantUUID(tenantUUID)
	if err != nil {
		return nil, err
	}
	return &plugin_mgr.HostConfig{
		Values: map[string]string{
			"PLUGIN_IAM_TENANT_KEY":            tenantUUID,
			"POWERX_GRPC_UPSTREAM_TENANT_UUID": tenantUUID,
			"POWERX_INSTALL_TENANT_UUID":       tenantUUID,
		},
	}, nil
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
	if meta.ReleaseChannel == "" {
		meta.ReleaseChannel = secondary.ReleaseChannel
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
	meta.ReleaseChannel = strings.TrimSpace(meta.ReleaseChannel)
	meta.Namespace = strings.TrimSpace(meta.Namespace)
	meta.Notes = strings.TrimSpace(meta.Notes)
	return meta
}

func isZeroMetadata(meta plugin_mgr.InstallMetadata) bool {
	return strings.TrimSpace(meta.Scope) == "" &&
		strings.TrimSpace(meta.Namespace) == "" &&
		strings.TrimSpace(meta.ReleaseChannel) == "" &&
		len(meta.DeprecatedEnvironment) == 0 &&
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
