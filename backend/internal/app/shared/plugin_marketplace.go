package shared

import (
	"context"
	"fmt"
	"net/http"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func newPluginMarketplace(opts *DepsOptions) *marketsvc.Service {
	client := market.NewClient(opts.PluginMarketplaceAPI, opts.PluginMarketplaceToken, &http.Client{Timeout: 20 * time.Second})
	service := marketsvc.NewService(client, opts.RuntimeIdentity.Version, func(ctx context.Context) ([]plugin_mgr.Plugin, error) {
		if opts.RuntimeIdentity.Manager == nil {
			return nil, fmt.Errorf("plugin manager unavailable")
		}
		mgr, err := opts.RuntimeIdentity.Manager()
		if err != nil {
			return nil, err
		}
		return mgr.List(ctx)
	})
	if err := service.ConfigureDeployment(opts.ProductInformation, opts.PluginMarketplacePublicSite); err != nil {
		logger.WarnF(context.Background(), "plugin.marketplace.public_site_url 无效，已暂停网址关联；实例登记与本地安装可继续")
	}
	if client.Configured() {
		store, err := market.OpenDistributionStore(opts.PluginMarketplaceStateDirectory, client.SourceID(), opts.EnvironmentID)
		service.ConfigureDistribution(store, opts.PluginMarketplaceEnrollment, opts.PluginMarketplaceHeartbeat, err)
	}
	return service
}
