package media

import (
	"os"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRegisterHostContractUsesSeparateActionPathSegments(t *testing.T) {
	engine := gin.New()
	registerHostContract(engine.Group("/api/v1"), &shared.Deps{
		DB:       &gorm.DB{},
		MediaSvc: &mediasvc.MediaService{},
	})

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		if route.Method == "POST" {
			routes[route.Path] = true
		}
	}
	require.True(t, routes["/api/v1/tenant/media/assets/:asset_uuid/presign-upload"])
	require.True(t, routes["/api/v1/tenant/media/assets/:asset_uuid/complete-upload"])
	require.True(t, routes["/api/v1/tenant/media/assets/:asset_uuid/presign-download"])
	for _, action := range []string{"presign-upload", "complete-upload", "presign-download"} {
		require.True(t, routes["/api/v1/tenant/media/assets/:asset_uuid/variants/:variant_uuid/"+action])
	}
}

func TestRegisterPublicResourceUsesCoreMediatedTransferRoutes(t *testing.T) {
	engine := gin.New()
	RegisterPublicResource(engine, &shared.Deps{MediaSvc: &mediasvc.MediaService{}})
	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	require.True(t, routes["PUT /api/v1/media/transfers/:asset_uuid/upload"])
	require.True(t, routes["GET /api/v1/media/transfers/:asset_uuid/download"])
	require.False(t, routes["PUT /media/transfers/:asset_uuid/upload"])
	require.False(t, routes["GET /media/transfers/:asset_uuid/download"])
	require.True(t, routes["PUT /api/v1/media/transfers/:asset_uuid/variants/:variant_uuid/upload"])
	require.True(t, routes["GET /api/v1/media/transfers/:asset_uuid/variants/:variant_uuid/download"])
	require.False(t, routes["GET /media/:uuid/resource"])
}

func TestMediaHostOpenAPIDoesNotExposeLegacyStorageOrVariantNames(t *testing.T) {
	content, err := os.ReadFile("../../../../../../specs/001-media-storage/contracts/http-openapi.yaml")
	require.NoError(t, err)
	for _, forbidden := range []string{"objectKey", "object_key", "ownerSubjectId", "tenantId", "/media/assets/{uuid}/variants/{variant}"} {
		require.NotContains(t, string(content), forbidden)
	}
}
