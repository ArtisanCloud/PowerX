package service

import (
	"github.com/ArtisanCloud/PowerLibs/v2/authorization/rbac/models"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"net/http"
)

type OpenAPIService struct {
	*Service
}

func NewOpenAPIService(ctx *gin.Context) (r *OpenAPIService) {
	r = &OpenAPIService{
		Service: NewService(ctx),
	}
	return r
}

const OPEN_API_VRESION = "3.0"
const OPEN_API_TITLE = "MarketX API"
const API_VERSION = "0.1"

func (srv *OpenAPIService) LoadOpenAPIFromFilePath(path string) (openAPI *openapi3.T, err error) {

	openAPI, err = openapi3.NewLoader().LoadFromFile(path)

	return openAPI, err
}

func (srv *OpenAPIService) ConvertRoutesToOpenAPI(arrayRoutes *gin.RoutesInfo) (openAPI *openapi3.T, err error) {
	//fmt.Dump(arrayRoutes)
	openAPI = &openapi3.T{
		OpenAPI: OPEN_API_VRESION,
		Info: &openapi3.Info{
			Title:   OPEN_API_TITLE,
			Version: "0.1",
		},
		Paths: openapi3.Paths{},
	}
	for _, route := range *arrayRoutes {
		//fmt.Dump(route.Method, route.Path)
		pathItem, err := srv.ConvertRouteToPathItem(&route)
		if err != nil {
			return nil, err
		}
		openAPI.Paths[route.Path] = pathItem
	}

	return openAPI, err
}

func (srv *OpenAPIService) ConvertRouteToPathItem(route *gin.RouteInfo) (openAPI *openapi3.PathItem, err error) {
	openAPI = &openapi3.PathItem{}

	switch route.Method {
	case http.MethodGet:
		openAPI.Get = &openapi3.Operation{}

	case http.MethodPost:
		openAPI.Post = &openapi3.Operation{}

	case http.MethodPut:
		openAPI.Put = &openapi3.Operation{}

	case http.MethodDelete:
		openAPI.Delete = &openapi3.Operation{}

	case http.MethodPatch:
		openAPI.Patch = &openapi3.Operation{}
	default:
	}
	return openAPI, err

}

// ---------------------------------------------------------------------------------------------------------------------
func (srv *OpenAPIService) ConvertOpenAPIToPermissions(openAPI *openapi3.T) (permissions []*models.Permission, err error) {
	permissions = []*models.Permission{}
	for uri, path := range openAPI.Paths {
		operations := path.Operations()
		for action, operation := range operations {
			permission := models.NewPermission(object.NewCollection(&object.HashMap{
				"objectValue": uri,
				"action":      action,
				"description": operation.Description,
			}))
			permissions = append(permissions, permission)
		}
	}

	return permissions, err
}

// ---------------------------------------------------------------------------------------------------------------------
func (srv *OpenAPIService) ConvertPermissionsToOpenAPI(permissions []*models.Permission) (openAPI *openapi3.T, err error) {

	openAPI = &openapi3.T{
		OpenAPI: OPEN_API_VRESION,
		Info: &openapi3.Info{
			Title:   OPEN_API_TITLE,
			Version: API_VERSION,
		},
		Paths: openapi3.Paths{},
	}
	for _, permission := range permissions {
		pathItem, err := srv.ConvertPermissionToPathItem(permission)
		if err != nil {
			return nil, err
		}
		openAPI.Paths[permission.ObjectValue] = pathItem
	}

	return openAPI, err
}

func (srv *OpenAPIService) ConvertPermissionToPathItem(permission *models.Permission) (openAPI *openapi3.PathItem, err error) {
	openAPI = &openapi3.PathItem{}

	switch permission.Action {
	case http.MethodGet:
		openAPI.Get = &openapi3.Operation{}

	case http.MethodPost:
		openAPI.Post = &openapi3.Operation{}

	case http.MethodPut:
		openAPI.Put = &openapi3.Operation{}

	case http.MethodDelete:
		openAPI.Delete = &openapi3.Operation{}

	case http.MethodPatch:
		openAPI.Patch = &openapi3.Operation{}
	default:
	}

	return openAPI, err

}
