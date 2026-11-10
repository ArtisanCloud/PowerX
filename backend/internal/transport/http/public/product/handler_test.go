package product

import (
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/internal/productinfo"
	marketsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_marketplace"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAnonymousProductAndProofPaths(t *testing.T) {
	for _, prefix := range []string{"/api/v1", "/api/v2", "/custom/api"} {
		t.Run(prefix, func(t *testing.T) {
			svc := marketsvc.NewService(market.NewClient("", "", nil), "1.0.0", nil)
			require.NoError(t, svc.ConfigureDeployment(productinfo.Information{Product: "powerx", State: "development"}, ""))
			r := gin.New()
			RegisterRoutes(r.Group(prefix), &shared.Deps{PluginMarketplace: svc})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", prefix+"/system/product", nil))
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), `"state":"development"`)
			require.NotContains(t, w.Body.String(), "instance")
			require.NotContains(t, w.Body.String(), "token")
			for _, q := range []struct {
				query  string
				status int
			}{{"", 400}, {"?challenge_id=" + uuid.NewString(), 404}, {"?challenge_id=" + uuid.NewString() + "&url=https://evil.example", 400}} {
				w = httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("GET", prefix+"/system/marketplace-proof"+q.query, nil))
				require.Equal(t, q.status, w.Code)
				require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
			}
		})
	}
}
