package manager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallFromURL: 下载 ZIP/PXP → 校验 SHA256 → 安全解压 → 共用 InstallFromFile。
func (m *managerImpl) InstallFromURL(ctx context.Context, url, sha256sum, signature string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	if opts.Origin.Type == "" {
		opts.Origin = plugin_mgr.InstallOrigin{Type: "local", Channel: "url"}
	}
	if strings.TrimSpace(url) == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("install_url"), plugin_mgr.WithMsg("empty url"))
	}
	if opts.VerifyChecksum && len(strings.TrimSpace(sha256sum)) != 64 {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("a valid SHA256 is required"))
	}

	// 1) 下载到临时文件
	tmpFile, err := m.downloadToTemp(ctx, url)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	defer os.Remove(tmpFile)
	if opts.ExpectedSize > 0 {
		info, err := os.Stat(tmpFile)
		if err != nil || info.Size() != opts.ExpectedSize {
			return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeChecksumMismatch, plugin_mgr.WithMsg("download size differs from selected artifact"))
		}
	}

	// 2) 校验 SHA256（如果提供了 sha256 或 opts.VerifyChecksum=true 都进行校验）
	actualSHA256 := ""
	if sha256sum != "" || opts.VerifyChecksum {
		actualSHA256, err = verifySHA256File(tmpFile, sha256sum)
		if err != nil {
			return plugin_mgr.Plugin{}, err
		}
		opts.Origin.SHA256 = strings.ToLower(strings.TrimSpace(sha256sum))
	}
	if opts.DownloadVerified != nil {
		info, err := os.Stat(tmpFile)
		if err != nil {
			return plugin_mgr.Plugin{}, err
		}
		if err := opts.DownloadVerified(ctx, plugin_mgr.DownloadVerification{SHA256: actualSHA256, Size: info.Size()}); err != nil {
			return plugin_mgr.Plugin{}, err
		}
	}

	// 3) （可选）签名校验：如 opts.VerifySignature && signature != ""，此处留空实现/返回未实现
	if opts.VerifySignature && signature != "" {
		// 你可以在这里接入 ed25519/rsa 公钥验签；暂时给个明确错误占位
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(
			plugin_mgr.CodeSignatureInvalid,
			plugin_mgr.WithOp("install_url"),
			plugin_mgr.WithMsg("signature verification not implemented"),
		)
	}

	// 4) 解压到临时目录
	tmpDir, err := os.MkdirTemp("", "px-plugin-")
	if err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.mktmp"))
	}
	// 如果失败也希望尽量清理
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractRemotePackage(tmpFile, tmpDir); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.unzip"))
	}

	// 5) 在解压目录内查找包含 plugin.yaml 的“版本根目录”
	srcDir, man, err := findManifestDir(tmpDir)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	if (opts.ExpectedPluginID != "" && man.ID != opts.ExpectedPluginID) || (opts.ExpectedVersion != "" && man.Version != opts.ExpectedVersion) {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInvalidManifest, plugin_mgr.WithMsg("package identity does not match selected plugin and version"))
	}

	// 6) 交给已实现的 InstallFromFile（会复制到 InstalledRoot/<id>/<version> 并登记）
	p, err := m.InstallFromFile(ctx, srcDir, opts)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// AutoEnable 已由共用安装器处理；HTTP 层继续执行租户启用收尾。
	return p, nil
}

// ---- 内部工具 ----

func (m *managerImpl) downloadToTemp(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.req"))
	}
	cli := &http.Client{Timeout: 60 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.get"))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", plugin_mgr.NewError(plugin_mgr.CodeIOError, plugin_mgr.WithOp("install_url.get"), plugin_mgr.WithMsg("bad http status: %s", resp.Status))
	}

	f, err := os.CreateTemp("", "px-plugin-*.zip")
	if err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.tmp"))
	}
	defer f.Close()
	saved := false
	defer func() {
		if !saved {
			_ = os.Remove(f.Name())
		}
	}()

	const maxDownloadBytes = 2 << 30
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.save"))
	}
	if n > maxDownloadBytes {
		return "", plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithMsg("plugin package exceeds 2 GiB"))
	}
	saved = true
	return f.Name(), nil
}

// Marketplace 制品可能是 ZIP 或 gzip TAR（包括 .pxp），按内容识别，不依赖预签名 URL 后缀。
func extractRemotePackage(source, dest string) error {
	f, err := os.Open(source)
	if err != nil {
		return err
	}
	var magic [2]byte
	_, err = io.ReadFull(f, magic[:])
	_ = f.Close()
	if err != nil {
		return err
	}
	if magic != [2]byte{0x1f, 0x8b} {
		return unzipSafe(source, dest)
	}
	f, err = os.Open(source)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var total int64
	for count := 0; ; count++ {
		h, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if count > 100000 || h.Size < 0 || h.Size > (4<<30)-total {
			return errors.New("plugin archive exceeds extraction limit")
		}
		target, err := safeArchivePath(dest, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(out, reader, h.Size)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("plugin archive contains unsupported link or special entry")
		}
	}
}

func safeArchivePath(dest, name string) (string, error) {
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" || strings.Contains(name, "\\") {
		return "", errors.New("plugin archive contains unsafe path")
	}
	target := filepath.Join(dest, name)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("plugin archive entry escapes destination")
	}
	return target, nil
}

func verifySHA256(path string, expectedHex string) error {
	_, err := verifySHA256File(path, expectedHex)
	return err
}
func verifySHA256File(path string, expectedHex string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("sha256.open"), plugin_mgr.WithPath(path))
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("sha256.read"), plugin_mgr.WithPath(path))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if expectedHex != "" && !strings.EqualFold(sum, expectedHex) {
		return "", plugin_mgr.NewError(plugin_mgr.CodeChecksumMismatch, plugin_mgr.WithOp("sha256.verify"),
			plugin_mgr.WithMsg("mismatch: expect %s got %s", expectedHex, sum))
	}
	return sum, nil
}

// 解压 zip（路径穿越保护）
func unzipSafe(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	var total uint64
	if len(r.File) > 100000 {
		return errors.New("plugin archive has too many entries")
	}
	for _, f := range r.File {
		target, err := safeArchivePath(dest, f.Name)
		if err != nil {
			return err
		}
		if !f.FileInfo().IsDir() && !f.Mode().IsRegular() {
			return errors.New("plugin archive contains link or special entry")
		}
		if f.UncompressedSize64 > (4<<30)-total {
			return errors.New("plugin archive exceeds extraction limit")
		}
		total += f.UncompressedSize64
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}

// 在解压根目录里寻找包含 plugin.yaml 的目录，返回该目录及解析出的 manifest
func findManifestDir(root string) (string, plugin_mgr.Manifest, error) {
	var foundDir string
	var man plugin_mgr.Manifest

	// 优先：root 下直接有 plugin.yaml
	try := filepath.Join(root, "plugin.yaml")
	if b, err := os.ReadFile(try); err == nil {
		if yaml.Unmarshal(b, &man) == nil && man.ID != "" && man.Version != "" {
			return root, man, nil
		}
	}

	// 其次：root 下的第一级子目录中找
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name(), "plugin.yaml")
		if b, err := os.ReadFile(p); err == nil {
			if yaml.Unmarshal(b, &man) == nil && man.ID != "" && man.Version != "" {
				foundDir = filepath.Join(root, e.Name())
				return foundDir, man, nil
			}
		}
	}
	return "", plugin_mgr.Manifest{}, plugin_mgr.NewError(
		plugin_mgr.CodeInvalidManifest,
		plugin_mgr.WithOp("install_url.find_manifest"),
		plugin_mgr.WithMsg("plugin.yaml not found or invalid"),
	)
}
