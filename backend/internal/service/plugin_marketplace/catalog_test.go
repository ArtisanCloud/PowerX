package plugin_marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestInstallationMatchesPublisherRegardlessOfSource(t *testing.T) {
	s := NewService(market.NewClient("https://market.example/api/v1/", "", http.DefaultClient), "1.0.0", nil)
	provider := plugin_mgr.ProviderIdentity{ID: "developer", Authority: "https://market.example/api/v1"}
	for _, tc := range []struct {
		name       string
		origin     plugin_mgr.InstallOrigin
		provider   plugin_mgr.ProviderIdentity
		want       string
		canReplace bool
	}{
		{"same publisher local", plugin_mgr.InstallOrigin{Type: "local"}, provider, "installed", true},
		{"same publisher market", s.Origin(), provider, "installed", true},
		{"other publisher", s.Origin(), plugin_mgr.ProviderIdentity{ID: "other", Authority: provider.Authority}, "source_conflict", false},
		{"legacy unknown", plugin_mgr.InstallOrigin{}, plugin_mgr.ProviderIdentity{}, "source_conflict", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inventory := []plugin_mgr.Plugin{{ID: "com.test.demo", Version: "1.2.0", Revision: "revision", Provider: tc.provider, InstallOrigin: tc.origin}}
			result := s.InstallationFor("com.test.demo", inventory, true, "developer")
			require.Equal(t, tc.want, result.Status)
			require.Equal(t, tc.canReplace, result.CanReplace)
			require.Equal(t, "not_installed", s.InstallationFor("com.test.other", inventory, true, "developer").Status)
			require.Equal(t, "unknown", s.InstallationFor("com.test.demo", inventory, false, "developer").Status)
		})
	}
}

func TestCatalogFiltersAcrossRemotePages(t *testing.T) {
	var pages []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pages = append(pages, page)
		items := []market.Plugin{}
		for i := (page - 1) * 100; i < page*100 && i < 101; i++ {
			items = append(items, market.Plugin{ID: fmt.Sprintf("com.test.%d", i), OwnerDevID: "developer"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": market.Page[market.Plugin]{Items: items, Total: 101, Page: page, Size: 100}})
	}))
	defer server.Close()
	client := market.NewClient(server.URL, "", server.Client())
	s := NewService(client, "1.0.0", func(context.Context) ([]plugin_mgr.Plugin, error) {
		return []plugin_mgr.Plugin{
			{ID: "com.test.0", InstallOrigin: plugin_mgr.InstallOrigin{Type: "local"}},
			{ID: "com.test.100", Version: "1.0.0", Provider: plugin_mgr.ProviderIdentity{ID: "developer", Authority: client.SourceID()}, InstallOrigin: plugin_mgr.InstallOrigin{Type: "marketplace", SourceID: client.SourceID()}},
		}, nil
	})
	result, err := s.Catalog(context.Background(), 1, 12, "installed")
	require.NoError(t, err)
	require.Equal(t, []int{1, 2}, pages)
	require.EqualValues(t, 1, result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "com.test.100", result.Items[0].ID)
	require.Equal(t, "installed", result.Items[0].Installation.Status)
	result, err = s.Catalog(context.Background(), 2, 12, "installed")
	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.EqualValues(t, 1, result.Total)
	_, err = s.Catalog(context.Background(), 1, 12, "invalid")
	require.Error(t, err)
}

func TestUnavailableInventoryNeverReportsNotInstalled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"com.test.demo"}],"total":1,"page":1,"size":12}}`))
	}))
	defer server.Close()
	s := NewService(market.NewClient(server.URL, "", server.Client()), "1.0.0", func(context.Context) ([]plugin_mgr.Plugin, error) { return nil, fmt.Errorf("private registry path") })
	result, err := s.Catalog(context.Background(), 1, 12, "all")
	require.NoError(t, err)
	require.False(t, result.InventoryReady)
	require.Equal(t, "unknown", result.Items[0].Installation.Status)
	_, err = s.Catalog(context.Background(), 1, 12, "installed")
	require.ErrorContains(t, err, "无法读取")
	require.NotContains(t, err.Error(), "private")
}
