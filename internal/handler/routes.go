// Code generated by goctl. DO NOT EDIT.
package handler

import (
	"net/http"

	adminauth "PowerX/internal/handler/admin/auth"
	admincommon "PowerX/internal/handler/admin/common"
	admincustomer "PowerX/internal/handler/admin/customer"
	admindepartment "PowerX/internal/handler/admin/department"
	admindictionary "PowerX/internal/handler/admin/dictionary"
	adminemployee "PowerX/internal/handler/admin/employee"
	adminlead "PowerX/internal/handler/admin/lead"
	adminmedia "PowerX/internal/handler/admin/media"
	adminopportunity "PowerX/internal/handler/admin/opportunity"
	adminpermission "PowerX/internal/handler/admin/permission"
	adminuserinfo "PowerX/internal/handler/admin/userinfo"
	mpcustomer "PowerX/internal/handler/mp/customer"
	"PowerX/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeNoPermJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/options/employees",
					Handler: admincommon.GetEmployeeOptionsHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/options/employee-query",
					Handler: admincommon.GetEmployeeQueryOptionsHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/options/departments",
					Handler: admincommon.GetDepartmentOptionsHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/common"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/department-tree/:depId",
					Handler: admindepartment.GetDepartmentTreeHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/departments/:id",
					Handler: admindepartment.GetDepartmentHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/departments",
					Handler: admindepartment.CreateDepartmentHandler(serverCtx),
				},
				{
					Method:  http.MethodPatch,
					Path:    "/departments/:id",
					Handler: admindepartment.PatchDepartmentHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/departments/:id",
					Handler: admindepartment.DeleteDepartmentHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/department"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodPost,
					Path:    "/employees/actions/sync",
					Handler: adminemployee.SyncEmployeesHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/employees/:id",
					Handler: adminemployee.GetEmployeeHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/employees",
					Handler: adminemployee.ListEmployeesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/employees",
					Handler: adminemployee.CreateEmployeeHandler(serverCtx),
				},
				{
					Method:  http.MethodPatch,
					Path:    "/employees/:id",
					Handler: adminemployee.UpdateEmployeeHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/employees/:id",
					Handler: adminemployee.DeleteEmployeeHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/employees/actions/reset-password",
					Handler: adminemployee.ResetPasswordHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/employee"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/roles",
					Handler: adminpermission.ListRolesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/roles",
					Handler: adminpermission.CreateRoleHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/roles/:roleCode",
					Handler: adminpermission.GetRoleHandler(serverCtx),
				},
				{
					Method:  http.MethodPatch,
					Path:    "/roles/:roleCode",
					Handler: adminpermission.PatchRoleHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/roles/:roleCode/users",
					Handler: adminpermission.GetRoleEmployeesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/roles/:roleCode/actions/set-permissions",
					Handler: adminpermission.SetRolePermissionsHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/api-list",
					Handler: adminpermission.ListAPIHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/roles/:roleCode/actions/set-employees",
					Handler: adminpermission.SetRoleEmployeesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/users/:userId/actions/set-roles",
					Handler: adminpermission.SetUserRolesHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/permission"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/access/actions/basic-login",
				Handler: adminauth.LoginHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/access/actions/exchange-token",
				Handler: adminauth.ExchangeHandler(serverCtx),
			},
		},
		rest.WithPrefix("/api/v1/admin/auth"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/leads",
					Handler: adminlead.ListLeadsHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/leads",
					Handler: adminlead.CreateLeadHandler(serverCtx),
				},
				{
					Method:  http.MethodPatch,
					Path:    "/leads/:id",
					Handler: adminlead.PatchLeadHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/leads/:id",
					Handler: adminlead.DeleteLeadHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/leads/:id/actions/assign-to-employee",
					Handler: adminlead.AssignLeadToEmployeeHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/lead"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/customers/:id",
					Handler: admincustomer.GetCustomerHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/customers",
					Handler: admincustomer.ListCustomersHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/customers",
					Handler: admincustomer.CreateCustomerHandler(serverCtx),
				},
				{
					Method:  http.MethodPatch,
					Path:    "/customers/:id",
					Handler: admincustomer.PatchCustomerHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/customers/:id",
					Handler: admincustomer.DeleteCustomerHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/customers/:id/actions/employees",
					Handler: admincustomer.AssignCustomerToEmployeeHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/customer"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/medias",
					Handler: adminmedia.GetMediaListHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/medias/actions/create-upload-url",
					Handler: adminmedia.CreateMediaUploadRequestHandler(serverCtx),
				},
				{
					Method:  http.MethodPut,
					Path:    "/medias/:mediaKey",
					Handler: adminmedia.CreateOrUpdateMediaHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/medias/:key",
					Handler: adminmedia.GetMediaByKeyHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/medias/:key",
					Handler: adminmedia.DeleteMediaHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/media"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/types",
					Handler: admindictionary.GetDictionaryTypesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/types",
					Handler: admindictionary.CreateDictionaryTypeHandler(serverCtx),
				},
				{
					Method:  http.MethodPut,
					Path:    "/types/:id",
					Handler: admindictionary.UpdateDictionaryTypeHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/types/:id",
					Handler: admindictionary.DeleteDictionaryTypeHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/items",
					Handler: admindictionary.GetDictionaryItemsHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/items",
					Handler: admindictionary.CreateDictionaryItemHandler(serverCtx),
				},
				{
					Method:  http.MethodPut,
					Path:    "/items/:id",
					Handler: admindictionary.UpdateDictionaryItemHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/items/:id",
					Handler: admindictionary.DeleteDictionaryItemHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/dictionary"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/opportunities",
					Handler: adminopportunity.GetOpportunityListHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/opportunities",
					Handler: adminopportunity.CreateOpportunityHandler(serverCtx),
				},
				{
					Method:  http.MethodPut,
					Path:    "/opportunities/:id/assign-employee",
					Handler: adminopportunity.AssignEmployeeToOpportunityHandler(serverCtx),
				},
				{
					Method:  http.MethodPut,
					Path:    "/opportunities/:id",
					Handler: adminopportunity.UpdateOpportunityHandler(serverCtx),
				},
				{
					Method:  http.MethodDelete,
					Path:    "/opportunities/:id",
					Handler: adminopportunity.DeleteOpportunityHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/opportunity"),
	)

	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.EmployeeJWTAuth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/user-info",
					Handler: adminuserinfo.GetUserInfoHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/menu-roles",
					Handler: adminuserinfo.GetMenuRolesHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/users/actions/modify-password",
					Handler: adminuserinfo.ModifyUserPasswordHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1/admin/user-center"),
	)

	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/login",
				Handler: mpcustomer.LoginHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/authByPhone",
				Handler: mpcustomer.AuthByPhoneHandler(serverCtx),
			},
			{
				Method:  http.MethodPost,
				Path:    "/authByProfile",
				Handler: mpcustomer.AuthByProfileHandler(serverCtx),
			},
		},
		rest.WithPrefix("/api/v1/mp/customer"),
	)
}
