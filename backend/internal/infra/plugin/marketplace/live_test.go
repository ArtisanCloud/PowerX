package marketplace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"runtime"
	"testing"
	"time"
)

// 本机真实合同探测：显式启用后才申请下载授权，会留下市场普通下载审计。
// 不安装、不启用插件，不将凭证或预签名 URL 写入日志。
func TestLiveMarketplaceCatalogDownload(t *testing.T) {
	if os.Getenv("POWERX_MARKETPLACE_LIVE_TEST") != "1" {
		t.Skip("需要显式启用本机 Marketplace 合同探测")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	httpClient := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	client := NewClient(os.Getenv("POWERX_MARKETPLACE_API_ROOT"), os.Getenv("POWERX_MARKETPLACE_DOWNLOAD_TOKEN"), httpClient)
	pluginID := os.Getenv("POWERX_MARKETPLACE_TEST_PLUGIN_ID")
	if pluginID == "" {
		t.Fatal("测试需要指定插件 ID")
	}
	plugins, err := client.List(ctx, runtime.GOOS, runtime.GOARCH, 1, 100)
	if err != nil {
		t.Fatal("真实目录接口失败")
	}
	found := false
	for _, p := range plugins.Items {
		if p.ID == pluginID {
			found = true
		}
	}
	if !found {
		t.Fatal("目标插件不在当前平台的目录中")
	}
	versions, err := client.Versions(ctx, pluginID, runtime.GOOS, runtime.GOARCH, 1, 20)
	if err != nil || len(versions.Items) == 0 {
		t.Fatal("真实版本接口失败或没有当前平台版本")
	}
	selected := versions.Items[0]
	download, err := client.Download(ctx, pluginID, selected.Semver, runtime.GOOS, runtime.GOARCH, "")
	if err != nil {
		t.Fatal("真实下载授权失败")
	}
	if download.SHA256 != selected.SHA256 || download.Size != selected.Size || download.Size <= 0 || download.Size > 2<<30 || download.OS != runtime.GOOS || download.Arch != runtime.GOARCH {
		t.Fatal("下载授权与所选平台版本不一致")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, download.SignedURL, nil)
	if err != nil {
		t.Fatal("下载地址无效")
	}
	response, err := httpClient.Do(req)
	if err != nil {
		t.Fatal("对象存储连接失败")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("对象存储下载状态 %d", response.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(response.Body, download.Size+1))
	if err != nil || size != download.Size || hex.EncodeToString(hash.Sum(nil)) != download.SHA256 {
		t.Fatal("真实制品 SHA256 或大小校验失败")
	}
	t.Logf("已验证 %s@%s %s/%s bytes=%d sha256=%s min_core=%s；未安装或启用", pluginID, selected.Semver, runtime.GOOS, runtime.GOARCH, size, download.SHA256, selected.MinCore)
}
