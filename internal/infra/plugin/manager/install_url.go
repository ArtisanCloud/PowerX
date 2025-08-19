package manager

import (
	"archive/zip"
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

// InstallFromURL: 下载 zip 包 -> 可选校验 SHA256 -> 解压到临时目录 -> 找到包含 plugin.yaml 的目录 -> 走 InstallFromFile
func (m *managerImpl) InstallFromURL(ctx context.Context, url, sha256sum, signature string, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	if strings.TrimSpace(url) == "" {
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeInvalidArg, plugin_mgr.WithOp("install_url"), plugin_mgr.WithMsg("empty url"))
	}

	// 1) 下载到临时文件
	tmpFile, err := m.downloadToTemp(ctx, url)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	defer os.Remove(tmpFile)

	// 2) 校验 SHA256（如果提供了 sha256 或 opts.VerifyChecksum=true 都进行校验）
	if sha256sum != "" || opts.VerifyChecksum {
		if err := verifySHA256(tmpFile, sha256sum); err != nil {
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

	if err := unzipSafe(tmpFile, tmpDir); err != nil {
		return plugin_mgr.Plugin{}, plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.unzip"))
	}

	// 5) 在解压目录内查找包含 plugin.yaml 的“版本根目录”
	srcDir, man, err := findManifestDir(tmpDir)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// 6) 交给已实现的 InstallFromFile（会复制到 InstalledRoot/<id>/<version> 并登记）
	p, err := m.InstallFromFile(ctx, srcDir, opts)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}

	// 7) 安装成功后可选择立即启用：这里不自动启用，交给外层 handler 调用 SwitchVersion(enable=true)
	_ = man // 只是说明我们已读过 manifest；真正返回的是 registry 里的视图 p
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

	// 可加大小限制：io.CopyN
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("install_url.save"))
	}
	return f.Name(), nil
}

func verifySHA256(path string, expectedHex string) error {
	f, err := os.Open(path)
	if err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("sha256.open"), plugin_mgr.WithPath(path))
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("sha256.read"), plugin_mgr.WithPath(path))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if expectedHex != "" && !strings.EqualFold(sum, expectedHex) {
		return plugin_mgr.NewError(plugin_mgr.CodeChecksumMismatch, plugin_mgr.WithOp("sha256.verify"),
			plugin_mgr.WithMsg("mismatch: expect %s got %s", expectedHex, sum))
	}
	return nil
}

// 解压 zip（路径穿越保护）
func unzipSafe(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.Contains(f.Name, "..") { // 简单保护
			return errors.New("zip contains path traversal")
		}
		target := filepath.Join(dest, f.Name)
		// 进一步保护：必须在 dest 内
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), filepath.Clean(dest)+string(os.PathSeparator)) {
			return errors.New("zip entry escapes dest")
		}
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
