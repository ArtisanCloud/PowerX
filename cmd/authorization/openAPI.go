package main

import (
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	service "github.com/ArtisanCloud/PowerX/app/services"
	global2 "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/ArtisanCloud/PowerX/routes/global"
	"os"
	"path"
)

var openAPIJsonFileFromRoutes string = path.Join("configs", "openapi3_from_routes.json")
var openAPIJsonFileFromPermissions string = path.Join("configs", "openapi3_from_permissions.json")

func ConvertRouts2OpenAPI() (err error) {

	fmt.Dump("convert routes to openapi")

	serviceOpenAPI := service.NewOpenAPIService(nil)
	arrayRoutes := global.Router.Routes()
	openAPI, err := serviceOpenAPI.ConvertRoutesToOpenAPI(&arrayRoutes)
	if err != nil {
		return err
	}

	data, err := openAPI.MarshalJSON()
	if err != nil {
		return err
	}

	err = os.WriteFile(openAPIJsonFileFromRoutes, data, 0644)
	if err != nil {
		return err
	}

	return err
}

func ConvertOpenAPI2Permissions() (err error) {
	fmt.Dump("convert openapi to permissions")
	serviceOpenAPI := service.NewOpenAPIService(nil)
	openAPI, err := serviceOpenAPI.LoadOpenAPIFromFilePath(openAPIJsonFileFromRoutes)
	if err != nil {
		return err
	}

	permissions, err := serviceOpenAPI.ConvertOpenAPIToPermissions(openAPI)
	if err != nil {
		return err
	}
	serviceRBAC := service.NewRBACService(nil)
	err = serviceRBAC.UpsertPermissions(global2.G_DBConnection, permissions, []string{
		"updated_at",
		"object_value",
		"action",
	})

	return err
}

func ConvertRoutes2Permissions() (err error) {
	err = ConvertRouts2OpenAPI()
	if err != nil {
		return err
	}
	err = ConvertOpenAPI2Permissions()

	return err
}

func ConvertPermissions2OpenAPI() (err error) {
	fmt.Dump("convert permissions to openapi")

	serviceRBAC := service.NewRBACService(nil)
	permission, err := serviceRBAC.GetPermissionList(global2.G_DBConnection)
	if err != nil {
		return err
	}
	serviceOpenAPI := service.NewOpenAPIService(nil)
	openAPI, err := serviceOpenAPI.ConvertPermissionsToOpenAPI(permission)
	if err != nil {
		return err
	}

	data, err := openAPI.MarshalJSON()
	if err != nil {
		return err
	}

	err = os.WriteFile(openAPIJsonFileFromPermissions, data, 0644)
	if err != nil {
		return err
	}

	return err
}
