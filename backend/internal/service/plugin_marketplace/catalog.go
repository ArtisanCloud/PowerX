package plugin_marketplace

import (
	"context"
	"runtime"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type Installation struct {
	Provider          plugin_mgr.ProviderIdentity `json:"provider"`
	InstalledProvider plugin_mgr.ProviderIdentity `json:"installed_provider"`
	Revision          string                      `json:"revision,omitempty"`
	CanReplace        bool                        `json:"can_replace"`
	Reason            string                      `json:"reason,omitempty"`
	Status            string                      `json:"status"` // installed / not_installed / source_conflict / unknown
	Version           string                      `json:"version,omitempty"`
	Origin            plugin_mgr.InstallOrigin    `json:"origin"`
}

type CatalogPlugin struct {
	market.Plugin
	Installation Installation `json:"installation"`
}

type Catalog struct {
	Connection     Status          `json:"connection"`
	InventoryReady bool            `json:"inventory_ready"`
	Items          []CatalogPlugin `json:"items"`
	Total          int64           `json:"total"`
	Page           int             `json:"page"`
	Size           int             `json:"size"`
}

func (s *Service) Origin() plugin_mgr.InstallOrigin {
	return plugin_mgr.InstallOrigin{Type: "marketplace", SourceID: s.client.SourceID(), Channel: "catalog", OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// InstallationFor 按发布者核对同 ID 安装，来源差异仅用于展示。旧身份可由 Root 明确确认后绑定。
func (s *Service) InstallationFor(id string, inventory []plugin_mgr.Plugin, ready bool, ownerDevID string) Installation {
	result := Installation{Provider: plugin_mgr.ProviderIdentity{ID: ownerDevID, Authority: s.client.SourceID()}.Normalized(), Status: "not_installed", Origin: plugin_mgr.InstallOrigin{Type: "unknown"}}
	if !ready {
		result.Status = "unknown"
		return result
	}
	for _, p := range inventory {
		if p.ID != id {
			continue
		}
		result.Version, result.Origin = p.Version, p.InstallOrigin.Normalized()
		result.Revision, result.InstalledProvider = p.Revision, p.Provider
		if p.Provider.SameProvider(result.Provider) {
			result.Status, result.CanReplace = "installed", true
		} else {
			result.Status = "source_conflict"
			result.Reason = "provider_mismatch"
			if !p.Provider.Known() || !result.Provider.Known() {
				result.Reason = "provider_unconfirmed"
				result.CanReplace = !p.Provider.Known() && result.Provider.Known()
			}
		}
		return result
	}
	return result
}

// Catalog 按市场来源筛选安装状态。筛选结果先汇总后分页，不能只过滤远端当前页。
func (s *Service) Catalog(ctx context.Context, page, size int, filter string) (Catalog, error) {
	out := Catalog{Connection: s.Status(), Items: []CatalogPlugin{}, Page: page, Size: size}
	if filter == "" {
		filter = "all"
	}
	if filter != "all" && filter != "installed" && filter != "not_installed" && filter != "source_conflict" {
		return out, invalid("安装状态筛选无效")
	}
	if page < 1 || size < 1 || size > 100 {
		return out, invalid("分页参数无效")
	}
	if !out.Connection.Configured {
		return out, nil
	}
	var inventory []plugin_mgr.Plugin
	if s.inventory != nil {
		var err error
		inventory, err = s.inventory(ctx)
		out.InventoryReady = err == nil
	}
	if !out.InventoryReady && filter != "all" {
		return out, &market.Error{Status: 503, Code: "PLUGIN_INVENTORY_UNAVAILABLE", Message: "无法读取宿主安装来源，请稍后重试"}
	}
	remotePage, remoteSize := page, size
	if filter != "all" {
		remotePage, remoteSize = 1, 100
	}
	for {
		remote, err := s.List(ctx, remotePage, remoteSize)
		if err != nil {
			return out, err
		}
		for _, p := range remote.Items {
			installation := s.InstallationFor(p.ID, inventory, out.InventoryReady, p.OwnerDevID)
			if filter == "all" || installation.Status == filter {
				out.Items = append(out.Items, CatalogPlugin{Plugin: p, Installation: installation})
			}
		}
		if filter == "all" {
			out.Total = remote.Total
			return out, nil
		}
		if int64(remotePage*remoteSize) >= remote.Total || len(remote.Items) == 0 {
			break
		}
		if remotePage >= 100 {
			return out, &market.Error{Status: 502, Code: "MARKETPLACE_CATALOG_LIMIT", Message: "目录过大，无法完整筛选安装状态"}
		}
		remotePage++
	}
	out.Total = int64(len(out.Items))
	if int64(page-1) >= (out.Total+int64(size)-1)/int64(size) {
		out.Items = []CatalogPlugin{}
		return out, nil
	}
	start := int64(page-1) * int64(size)
	end := start + int64(size)
	if end > out.Total {
		end = out.Total
	}
	out.Items = out.Items[start:end]
	return out, nil
}
