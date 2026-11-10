package plugin_marketplace

import (
	"context"
	"encoding/hex"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"golang.org/x/mod/semver"
)

type Service struct {
	client       *market.Client
	coreVersion  string
	inventory    Inventory
	distribution *distribution
	deployment   *deploymentState
}

// Inventory 读取宿主实际安装记录，不从远程目录或包声明推断来源。
type Inventory func(context.Context) ([]plugin_mgr.Plugin, error)

func NewService(client *market.Client, coreVersion string, inventory Inventory) *Service {
	return &Service{client: client, coreVersion: coreVersion, inventory: inventory, deployment: &deploymentState{info: productinfo.Information{Product: "powerx", State: "development"}, proofs: map[string]storedProof{}, now: time.Now}}
}

type Status struct {
	Configured    bool            `json:"configured"`
	DownloadReady bool            `json:"download_ready"`
	OS            string          `json:"os"`
	Arch          string          `json:"arch"`
	CoreVersion   string          `json:"core_version"`
	SourceID      string          `json:"source_id"`
	Instance      *InstanceStatus `json:"instance,omitempty"`
}

func (s *Service) Status() Status {
	out := Status{Configured: s.client.Configured(), DownloadReady: s.client.DownloadReady(), OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: s.coreVersion, SourceID: s.client.SourceID()}
	if s.distribution != nil {
		out.Instance = s.distribution.status()
		out.DownloadReady = out.DownloadReady && s.distribution.available()
	}
	return out
}
func (s *Service) List(ctx context.Context, page, size int) (market.Page[market.Plugin], error) {
	result, err := s.client.List(ctx, runtime.GOOS, runtime.GOARCH, page, size)
	if s.distribution != nil {
		s.distribution.authenticationError(err)
	}
	return result, err
}

// Provider 以当前配置市场为签发域，使用目录返回的开发者 ID 固定发布者身份。
func (s *Service) Provider(ctx context.Context, id string) (plugin_mgr.ProviderIdentity, error) {
	if !validID.MatchString(id) {
		return plugin_mgr.ProviderIdentity{}, invalid("插件 ID 无效")
	}
	p, err := s.client.Plugin(ctx, id, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		if s.distribution != nil {
			s.distribution.authenticationError(err)
		}
		return plugin_mgr.ProviderIdentity{}, err
	}
	provider := plugin_mgr.ProviderIdentity{ID: p.OwnerDevID, Authority: s.client.SourceID()}.Normalized()
	if p.ID != id || !provider.Known() {
		return plugin_mgr.ProviderIdentity{}, &market.Error{Status: 502, Code: "MARKETPLACE_PROVIDER_INVALID", Message: "市场目录缺少有效的发布者身份"}
	}
	return provider, nil
}
func (s *Service) Versions(ctx context.Context, id string, page, size int) (market.Page[market.Version], error) {
	if !validID.MatchString(id) {
		return market.Page[market.Version]{}, invalid("插件 ID 无效")
	}
	result, err := s.client.Versions(ctx, id, runtime.GOOS, runtime.GOARCH, page, size)
	if s.distribution != nil {
		s.distribution.authenticationError(err)
	}
	return result, err
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func invalid(message string) error {
	return &market.Error{Status: 400, Code: "MARKETPLACE_SELECTION_INVALID", Message: message}
}
func validHash(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32
}

// ResolveDownload 重新核对已发布的版本与平台；浏览器不能提交 URL、校验和或目标平台。
func (s *Service) ResolveDownload(ctx context.Context, id, version, tenant string, attemptIDs ...string) (market.Download, error) {
	var empty market.Download
	if !validID.MatchString(id) || !semver.IsValid("v"+strings.TrimPrefix(version, "v")) || tenant == "" {
		return empty, invalid("请选择明确的插件版本并提供租户上下文")
	}
	if !s.Status().DownloadReady {
		return empty, &market.Error{Status: 503, Code: "MARKETPLACE_DOWNLOAD_AUTH_REQUIRED", Message: "Marketplace 下载凭证未配置"}
	}
	var selected *market.Version
	for page := 1; page <= 100; page++ {
		versions, err := s.Versions(ctx, id, page, 100)
		if err != nil {
			return empty, err
		}
		for _, v := range versions.Items {
			if v.Semver == version && v.PluginID == id {
				copy := v
				selected = &copy
				break
			}
		}
		if selected != nil || len(versions.Items) == 0 || int64(page*100) >= versions.Total {
			break
		}
	}
	if selected == nil {
		return empty, &market.Error{Status: 404, Code: "MARKETPLACE_VERSION_NOT_FOUND", Message: "该版本没有适用于当前 PowerX 平台的已发布安装包"}
	}
	if selected.OS != runtime.GOOS || selected.Arch != runtime.GOARCH || !validHash(selected.SHA256) || selected.Size <= 0 || selected.Size > 2<<30 {
		return empty, invalid("Marketplace 版本缺少有效的平台、SHA256 或安装包大小")
	}
	if selected.MinCore != "" && selected.MinCore != "*" {
		minimum := "v" + strings.TrimPrefix(selected.MinCore, "v")
		current := "v" + strings.TrimPrefix(s.coreVersion, "v")
		if !semver.IsValid(minimum) || !semver.IsValid(current) || semver.Compare(current, minimum) < 0 {
			return empty, invalid("当前 PowerX 版本不满足插件 min_core 要求")
		}
	}
	key := ""
	if s.distribution != nil && s.distribution.registered() {
		if len(attemptIDs) != 1 || attemptIDs[0] == "" {
			return empty, invalid("实例下载需要持久化的安装尝试")
		}
		key = attemptIDs[0]
		if err := s.distribution.prepareDownload(key, id, version, tenant, selected.Target); err != nil {
			return empty, err
		}
	}
	download, err := s.client.DownloadWithKey(ctx, id, version, runtime.GOOS, runtime.GOARCH, tenant, key)
	if err != nil {
		if s.distribution != nil {
			s.distribution.authenticationError(err)
		}
		return empty, err
	}
	u, parseErr := url.Parse(download.SignedURL)
	if parseErr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || !download.ExpiresAt.After(time.Now()) ||
		download.OS != runtime.GOOS || download.Arch != runtime.GOARCH || !strings.EqualFold(download.SHA256, selected.SHA256) || download.Size != selected.Size {
		return empty, &market.Error{Status: 502, Code: "MARKETPLACE_DOWNLOAD_INVALID", Message: "Marketplace 下载响应与所选平台版本不一致，或下载链接已过期"}
	}
	if key != "" {
		if err := s.distribution.saveTicket(key, download); err != nil {
			return empty, err
		}
	}
	return download, nil
}
