package api

import (
	"github.com/ArtisanCloud/PowerX/app/http/controllers/admin"
	"github.com/ArtisanCloud/PowerX/app/http/controllers/wx"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	"github.com/ArtisanCloud/PowerX/app/http/request"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/contactWay"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/customer"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/department"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/employee"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/groupChat"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/permission/policy"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/role"
	sendChatMsg "github.com/ArtisanCloud/PowerX/app/http/request/admin/sendChatMessage"
	sendGroupChatMsg "github.com/ArtisanCloud/PowerX/app/http/request/admin/sendGroupChatMessage"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/tag"
	groupChat2 "github.com/ArtisanCloud/PowerX/app/http/request/admin/tag/groupChat"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/wx/weCom/media"
	wxTag "github.com/ArtisanCloud/PowerX/app/http/request/admin/wx/weCom/tag"
	"github.com/ArtisanCloud/PowerX/routes/global"
)

/* ------------------------------------------ admin api ------------------------------------------*/

func InitAdminAPIRoutes() {

	apiAdminRouter := global.G_Router.Group("/admin/api")
	{
		apiAdminRouter.Use(middleware.CheckInstalled, middleware.Maintenance, middleware.AuthenticateEmployeeByHeader, middleware.AuthorizeAPI)
		{

			//  Customer - 企微客户接口
			apiAdminRouter.POST("/customer/sync", customer.ValidateSyncCustomer, admin.APIWXCustomerSync)
			apiAdminRouter.GET("/customer/list", request.ValidateList, admin.APIGetCustomerList)
			apiAdminRouter.GET("/customer/detail", customer.ValidateCustomerDetail, admin.APIGetCustomerDetail)

			//  Customer - wechat platform - 客户微信平台直连接口
			apiAdminRouter.GET("/wxPlatform/weCom/customer/list", customer.ValidateWXPlatformCustomerList, admin.APIGetCustomerListOnWXPlatform)
			apiAdminRouter.GET("/wxPlatform/weCom/customer/detail", customer.ValidateCustomerDetail, admin.APIGetCustomerDetailOnWXPlatform)

			//  Employee - 企微员工接口
			apiAdminRouter.POST("/employee/sync", admin.APISyncWXEmployees)
			apiAdminRouter.POST("/employee/sync/customers", admin.APISyncEmployeeAndWXAccount)
			apiAdminRouter.GET("/employee/list", request.ValidateList, admin.APIGetEmployeeList)
			apiAdminRouter.GET("/employee/detail", employee.ValidateEmployeeDetail, admin.APIGetEmployeeDetail)
			apiAdminRouter.POST("/employee/bind/customer", employee.ValidateBindCustomerToEmployee, admin.APIBindCustomerToEmployee)
			apiAdminRouter.POST("/employee/unbind/customer", employee.ValidateBindCustomerToEmployee, admin.APIUnbindCustomerToEmployee)

			//  Employee - wechat platform - 企微部门微信平台直连接口
			apiAdminRouter.GET("/wxPlatform/weCom/employee/list", employee.ValidateWXPlatformEmployeeList, admin.APIGetEmployeeListOnWXPlatform)
			apiAdminRouter.GET("/wxPlatform/weCom/employee/detail", employee.ValidateEmployeeDetail, admin.APIGetEmployeeDetailOnWXPlatform)
			apiAdminRouter.DELETE("/wxPlatform/weCom/employee/delete", employee.ValidateWXPlatformDeleteEmployee, admin.APIDeleteEmployeesOnWXPlatform)

			// Department - 企微部门接口
			apiAdminRouter.GET("/department/sync", wx.APISyncWXDepartments)
			apiAdminRouter.GET("/department/list", wx.APIGetDepartmentList)

			// Department - 企微部门微信平台直连接口
			apiAdminRouter.GET("/wxPlatform/weCom/department/simpleList", department.ValidateWXPlatformDepartmentList, wx.APIGetDepartmentSimpleListOnWXPlatform)
			apiAdminRouter.GET("/wxPlatform/weCom/department/list", department.ValidateWXPlatformDepartmentList, wx.APIGetDepartmentListOnWXPlatform)

			//  WX Tag Group - 微信标签组接口
			apiAdminRouter.GET("/wechat/tag/group/sync", wxTag.ValidateWXTagGroupSync, wx.APIGetWXTagGroupSync)
			apiAdminRouter.GET("/wechat/tag/group/list", wxTag.ValidateWXTagGroupList, wx.APIGetWXTagGroupList)
			apiAdminRouter.GET("/wechat/tag/group/detail", wxTag.ValidateWXTagGroupDetail, wx.APIGetWXTagGroupDetail)
			apiAdminRouter.POST("/wechat/tag/group/create", wxTag.ValidateInsertWXTagGroup, wx.APIInsertWXTagGroup)
			apiAdminRouter.PUT("/wechat/tag/group/update", wxTag.ValidateUpdateWXTagGroup, wx.APIUpdateWXTagGroup)
			apiAdminRouter.DELETE("/wechat/tag/group/delete", wxTag.ValidateDeleteWXTagGroup, wx.APIDeleteWXTagGroups)

			//  WX Tag - 打企业标签接口
			apiAdminRouter.POST("/wechat/tag/bind/customerToEmployee/by/contactWay", wxTag.ValidateBindTagsToCustomerToEmployeeByContactWayTags, wx.APIBindWXTagsToCustomerToEmployeeByContactWayTags)
			apiAdminRouter.POST("/wechat/tag/bind/customerToEmployee", wxTag.ValidateBindTagsToCustomerToEmployee, wx.APIBindWXTagsToCustomerToEmployee)

			//  Tag Group - 标签组接口
			apiAdminRouter.GET("/tag/group/list", tag.ValidateTagGroupList, admin.APIGetTagGroupList)
			apiAdminRouter.GET("/tag/group/detail", tag.ValidateTagGroupDetail, admin.APIGetTagGroupDetail)
			apiAdminRouter.POST("/tag/group/create", tag.ValidateInsertTagGroup, admin.APIInsertTagGroup)
			apiAdminRouter.PUT("/tag/group/update", tag.ValidateUpdateTagGroup, admin.APIUpdateTagGroup)
			apiAdminRouter.DELETE("/tag/group/delete", tag.ValidateDeleteTagGroup, admin.APIDeleteTagGroups)

			// Group chat (群聊)接口
			apiAdminRouter.POST("/tag/bind/groupChat", groupChat2.ValidateBindTagsToGroupChats, admin.APIBindTagsToGroupChat)
			apiAdminRouter.GET("/tag/group/groupChat/list", request.ValidateList, admin.APIGetGroupChatTagGroupList)
			apiAdminRouter.POST("/tag/group/groupChat/create", groupChat2.ValidateInsertTagGroup, admin.APIInsertTagGroup)
			apiAdminRouter.PUT("/tag/group/groupChat/update", groupChat2.ValidateUpdateTagGroup, admin.APIUpdateTagGroup)

			// Contact Way 渠道码分组接口
			apiAdminRouter.GET("/contactWay/group/list", admin.APIGetContactWayGroupList)
			apiAdminRouter.GET("/contactWay/group/detail", request.ValidateDetail, admin.APIGetContactWayGroupDetail)
			apiAdminRouter.POST("/contactWay/group/create", contactWay.ValidateUpsertContactWayGroup, admin.APIUpsertContactWayGroup)
			apiAdminRouter.PUT("/contactWay/group/update", contactWay.ValidateUpsertContactWayGroup, admin.APIUpsertContactWayGroup)
			apiAdminRouter.DELETE("/contactWay/group/delete", request.ValidateDelete, admin.APIDeleteContactWayGroups)

			// Contact way 渠道码接口
			apiAdminRouter.GET("/contactWay/sync", contactWay.ValidateContactWaySync, admin.APIContactWaySync)
			apiAdminRouter.GET("/contactWay/list", contactWay.ValidateContactWayList, admin.APIGetContactWayList)
			apiAdminRouter.GET("/contactWay/detail", contactWay.ValidateContactWayDetail, admin.APIGetContactWayDetail)
			apiAdminRouter.POST("/contactWay/create", contactWay.ValidateCreateContactWay, admin.APICreateContactWay)
			apiAdminRouter.PUT("/contactWay/update", contactWay.ValidateUpdateContactWay, admin.APIUpdateContactWay)
			apiAdminRouter.DELETE("/contactWay/delete", contactWay.ValidateDeleteContactWay, admin.APIDeleteContactWays)

			// Contact way 渠道码 - 微信平台直连接口
			apiAdminRouter.GET("/wxPlatform/weCom/contactWay/list", contactWay.ValidateWXPlatformContactWayList, admin.APIGetContactWayListOnWXPlatform)
			apiAdminRouter.GET("/wxPlatform/weCom/contactWay/detail", contactWay.ValidateContactWayDetail, admin.APIGetContactWayDetailOnWXPlatform)
			apiAdminRouter.DELETE("/wxPlatform/weCom/contactWay/delete", contactWay.ValidateWXPlatformDeleteContactWay, admin.APIDeleteContactWaysOnWXPlatform)

			// WeCom media - 企微的媒体接口
			apiAdminRouter.POST("/wxPlatform/weCom/media/upload/image", media.ValidateUploadMedia, wx.APIWeComMediaUploadImage)
			apiAdminRouter.POST("/wxPlatform/weCom/media/upload/tempImage", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempImage)
			apiAdminRouter.POST("/wxPlatform/weCom/media/upload/tempVoice", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempVoice)
			apiAdminRouter.POST("/wxPlatform/weCom/media/upload/tempVideo", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempVideo)
			apiAdminRouter.POST("/wxPlatform/weCom/media/upload/tempFile", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempFile)

			//  WeCom group chat - 企微的群聊接口
			apiAdminRouter.GET("/groupChat/sync", wx.APIGroupChatSync)
			apiAdminRouter.POST("/groupChat/list", groupChat.ValidateGroupChatList, wx.APIGetGroupChatList)
			apiAdminRouter.GET("/groupChat/detail", groupChat.ValidateGroupChatDetail, wx.APIGetGroupChatDetail)

			apiAdminRouter.GET("/wxPlatform/weCom/groupChat/list", groupChat.ValidateWXPlatformGroupChatList, wx.APIGetGroupChatListOnWXPlatform)
			apiAdminRouter.POST("/wxPlatform/weCom/groupChat/detail", groupChat.ValidateWXPlatformGroupChatDetail, wx.APIGetGroupChatDetailOnWXPlatform)

			//  send chat message - 发送客户群发消息接口
			apiAdminRouter.GET("/sendChatMessage/doSend", admin.APIDoSendChatMsgs)
			apiAdminRouter.POST("/sendChatMessage/sync", sendChatMsg.ValidateSendChatMsgSync, admin.APISendChatMsgSync)
			apiAdminRouter.POST("/sendChatMessage/list", sendChatMsg.ValidateSendChatMsgList, admin.APIGetSendChatMsgList)
			apiAdminRouter.GET("/sendChatMessage/detail", sendChatMsg.ValidateSendChatMsgDetail, admin.APIGetSendChatMsgDetail)
			apiAdminRouter.POST("/sendChatMessage/create", sendChatMsg.ValidateCreateSendChatMsg, admin.APICreateSendChatMsg)
			apiAdminRouter.POST("/sendChatMessage/estimateExternalUsers", sendChatMsg.ValidateCreateSendChatMsg, admin.APIEstimateSendChatCustomersCount)

			//  send group chat message - 发送客户群群发消息接口
			apiAdminRouter.GET("/sendGroupChatMessage/doSend", admin.APIDoSendGroupChatMsgs)
			apiAdminRouter.POST("/sendGroupChatMessage/sync", sendGroupChatMsg.ValidateSendGroupChatMsgSync, admin.APISendGroupChatMsgSync)
			apiAdminRouter.POST("/sendGroupChatMessage/list", sendGroupChatMsg.ValidateSendGroupChatMsgList, admin.APIGetSendGroupChatMsgList)
			apiAdminRouter.GET("/sendGroupChatMessage/detail", sendGroupChatMsg.ValidateSendGroupChatMsgDetail, admin.APIGetSendGroupChatMsgDetail)
			apiAdminRouter.POST("/sendGroupChatMessage/create", sendGroupChatMsg.ValidateCreateSendGroupChatMsg, admin.APICreateSendGroupChatMsg)
			apiAdminRouter.POST("/sendGroupChatMessage/estimateExternalUsers", sendGroupChatMsg.ValidateCreateSendGroupChatMsg, admin.APIEstimateSendGroupChatCustomersCount)

			//  Role
			apiAdminRouter.GET("/role/list", role.ValidateRoleList, admin.APIGetRoleList)
			apiAdminRouter.GET("/role/detail", role.ValidateRoleDetail, admin.APIGetRoleDetail)
			apiAdminRouter.POST("/role/create", role.ValidateInsertRole, admin.APIInsertRole)
			apiAdminRouter.PUT("/role/update", role.ValidateUpdateRole, admin.APIUpdateRole)
			apiAdminRouter.DELETE("/role/delete", role.ValidateDeleteRole, admin.APIDeleteRoles)
			apiAdminRouter.POST("/role/bind/employee", role.ValidateBindRoleToEmployees, admin.APIBindRoleToEmployees)

			apiAdminRouter.GET("/permission/policy/list", policy.ValidatePolicyList, admin.APIGetPolicyList)
			apiAdminRouter.PUT("/permission/policy/update", policy.ValidateUpdatePolicy, admin.APIUpdatePolicy)

		}
	}

	apiAdminRouterByQuery := global.G_Router.Group("/admin/api")
	{
		apiAdminRouterByQuery.Use(middleware.Maintenance, middleware.AuthenticateEmployeeByQuery)
		{
			apiAdminRouterByQuery.GET("/wxPlatform/weCom/media/detail", media.ValidateGetMedia, wx.APIWeComMediaGetMedia)
		}
	}
}
