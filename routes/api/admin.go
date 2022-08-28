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
	sendChatMsg "github.com/ArtisanCloud/PowerX/app/http/request/admin/sendChatMessage"
	sendGroupChatMsg "github.com/ArtisanCloud/PowerX/app/http/request/admin/sendGroupChatMessage"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/tag"
	groupChat2 "github.com/ArtisanCloud/PowerX/app/http/request/admin/tag/groupChat"
	"github.com/ArtisanCloud/PowerX/app/http/request/admin/wx/wecom/media"
	wxTag "github.com/ArtisanCloud/PowerX/app/http/request/admin/wx/wecom/tag"
	"github.com/ArtisanCloud/PowerX/routes/global"
)

/* ------------------------------------------ admin api ------------------------------------------*/

func InitAdminAPIRoutes() {

	apiRouter := global.Router.Group("/admin/api")
	{
		apiRouter.Use(middleware.Maintenance, middleware.AuthenticateEmployeeAPI, middleware.AuthorizeAPI)
		{

			//  Customer - 企微客户接口
			apiRouter.POST("/customer/sync", customer.ValidateSyncCustomer, admin.APIWXCustomerSync)
			apiRouter.GET("/customer/list", request.ValidateList, admin.APIGetCustomerList)
			apiRouter.GET("/customer/detail", customer.ValidateCustomerDetail, admin.APIGetCustomerDetail)

			//  Customer - wx platform - 客户微信平台直连接口
			apiRouter.GET("/wxPlatform/wecom/customer/list", customer.ValidateWXPlatformCustomerList, admin.APIGetCustomerListOnWXPlatform)
			apiRouter.GET("/wxPlatform/wecom/customer/detail", customer.ValidateCustomerDetail, admin.APIGetCustomerDetailOnWXPlatform)

			//  Employee - 企微员工接口
			apiRouter.POST("/employee/sync", admin.APISyncWXEmployees)
			apiRouter.POST("/employee/sync/customers", admin.APISyncEmployeeAndWXAccount)
			apiRouter.GET("/employee/list", request.ValidateList, admin.APIGetEmployeeList)
			apiRouter.GET("/employee/detail", employee.ValidateEmployeeDetail, admin.APIGetEmployeeDetail)
			apiRouter.POST("/employee/bind/customer", employee.ValidateBindCustomerToEmployee, admin.APIBindCustomerToEmployee)
			apiRouter.POST("/employee/unbind/customer", employee.ValidateBindCustomerToEmployee, admin.APIUnbindCustomerToEmployee)

			//  Employee - wx platform - 企微部门微信平台直连接口
			apiRouter.GET("/wxPlatform/wecom/employee/list", employee.ValidateWXPlatformEmployeeList, admin.APIGetEmployeeListOnWXPlatform)
			apiRouter.GET("/wxPlatform/wecom/employee/detail", employee.ValidateEmployeeDetail, admin.APIGetEmployeeDetailOnWXPlatform)
			apiRouter.DELETE("/wxPlatform/wecom/employee/delete", employee.ValidateWXPlatformDeleteEmployee, admin.APIDeleteEmployeesOnWXPlatform)

			// Department - 企微部门接口
			apiRouter.GET("/department/sync", wx.APISyncWXDepartments)
			apiRouter.GET("/department/list", wx.APIGetDepartmentList)

			// Department - 企微部门微信平台直连接口
			apiRouter.GET("/wxPlatform/wecom/department/simpleList", department.ValidateWXPlatformDepartmentList, wx.APIGetDepartmentSimpleListOnWXPlatform)
			apiRouter.GET("/wxPlatform/wecom/department/list", department.ValidateWXPlatformDepartmentList, wx.APIGetDepartmentListOnWXPlatform)

			//  WX Tag Group - 微信标签组接口
			apiRouter.GET("/wx/tag/group/sync", wxTag.ValidateWXTagGroupSync, wx.APIGetWXTagGroupSync)
			apiRouter.GET("/wx/tag/group/list", wxTag.ValidateWXTagGroupList, wx.APIGetWXTagGroupList)
			apiRouter.GET("/wx/tag/group/detail", wxTag.ValidateWXTagGroupDetail, wx.APIGetWXTagGroupDetail)
			apiRouter.POST("/wx/tag/group/create", wxTag.ValidateInsertWXTagGroup, wx.APIInsertWXTagGroup)
			apiRouter.PUT("/wx/tag/group/update", wxTag.ValidateUpdateWXTagGroup, wx.APIUpdateWXTagGroup)
			apiRouter.DELETE("/wx/tag/group/delete", wxTag.ValidateDeleteWXTagGroup, wx.APIDeleteWXTagGroups)

			//  WX Tag - 打企业标签接口
			apiRouter.POST("/wx/tag/bind/customerToEmployee/by/contactWay", wxTag.ValidateBindTagsToCustomerToEmployeeByContactWayTags, wx.APIBindWXTagsToCustomerToEmployeeByContactWayTags)
			apiRouter.POST("/wx/tag/bind/customerToEmployee", wxTag.ValidateBindTagsToCustomerToEmployee, wx.APIBindWXTagsToCustomerToEmployee)

			//  Tag Group - 标签组接口
			apiRouter.GET("/tag/group/list", tag.ValidateTagGroupList, admin.APIGetTagGroupList)
			apiRouter.GET("/tag/group/detail", tag.ValidateTagGroupDetail, admin.APIGetTagGroupDetail)
			apiRouter.POST("/tag/group/create", tag.ValidateInsertTagGroup, admin.APIInsertTagGroup)
			apiRouter.PUT("/tag/group/update", tag.ValidateUpdateTagGroup, admin.APIUpdateTagGroup)
			apiRouter.DELETE("/tag/group/delete", tag.ValidateDeleteTagGroup, admin.APIDeleteTagGroups)

			// Group chat (群聊)接口
			apiRouter.POST("/tag/bind/groupChat", groupChat2.ValidateBindTagsToGroupChats, admin.APIBindTagsToGroupChat)
			apiRouter.GET("/tag/group/groupChat/list", request.ValidateList, admin.APIGetGroupChatTagGroupList)
			apiRouter.POST("/tag/group/groupChat/create", groupChat2.ValidateInsertTagGroup, admin.APIInsertTagGroup)
			apiRouter.PUT("/tag/group/groupChat/update", groupChat2.ValidateUpdateTagGroup, admin.APIUpdateTagGroup)

			// Contact Way 渠道码分组接口
			apiRouter.GET("/contactWay/group/list", admin.APIGetContactWayGroupList)
			apiRouter.GET("/contactWay/group/detail", request.ValidateDetail, admin.APIGetContactWayGroupDetail)
			apiRouter.POST("/contactWay/group/create", contactWay.ValidateUpsertContactWayGroup, admin.APIUpsertContactWayGroup)
			apiRouter.PUT("/contactWay/group/update", contactWay.ValidateUpsertContactWayGroup, admin.APIUpsertContactWayGroup)
			apiRouter.DELETE("/contactWay/group/delete", request.ValidateDelete, admin.APIDeleteContactWayGroups)

			// Contact way 渠道码接口
			apiRouter.GET("/contactWay/sync", contactWay.ValidateContactWaySync, admin.APIContactWaySync)
			apiRouter.GET("/contactWay/list", contactWay.ValidateContactWayList, admin.APIGetContactWayList)
			apiRouter.GET("/contactWay/detail", contactWay.ValidateContactWayDetail, admin.APIGetContactWayDetail)
			apiRouter.POST("/contactWay/create", contactWay.ValidateCreateContactWay, admin.APICreateContactWay)
			apiRouter.PUT("/contactWay/update", contactWay.ValidateUpdateContactWay, admin.APIUpdateContactWay)
			apiRouter.DELETE("/contactWay/delete", contactWay.ValidateDeleteContactWay, admin.APIDeleteContactWays)

			// Contact way 渠道码 - 微信平台直连接口
			apiRouter.GET("/wxPlatform/wecom/contactWay/list", contactWay.ValidateWXPlatformContactWayList, admin.APIGetContactWayListOnWXPlatform)
			apiRouter.GET("/wxPlatform/wecom/contactWay/detail", contactWay.ValidateContactWayDetail, admin.APIGetContactWayDetailOnWXPlatform)
			apiRouter.DELETE("/wxPlatform/wecom/contactWay/delete", contactWay.ValidateWXPlatformDeleteContactWay, admin.APIDeleteContactWaysOnWXPlatform)

			// WeCom media - 企微的媒体接口
			apiRouter.POST("/wxPlatform/wecom/media/upload/image", media.ValidateUploadMedia, wx.APIWeComMediaUploadImage)
			apiRouter.POST("/wxPlatform/wecom/media/upload/tempImage", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempImage)
			apiRouter.POST("/wxPlatform/wecom/media/upload/tempVoice", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempVoice)
			apiRouter.POST("/wxPlatform/wecom/media/upload/tempVideo", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempVideo)
			apiRouter.POST("/wxPlatform/wecom/media/upload/tempFile", media.ValidateUploadMedia, wx.APIWeComMediaUploadTempFile)
			apiRouter.GET("/wxPlatform/wecom/media/detail", media.ValidateGetMedia, wx.APIWeComMediaGetMedia)

			//  WeCom group chat - 企微的群聊接口
			apiRouter.GET("/groupChat/sync", wx.APIGroupChatSync)
			apiRouter.POST("/groupChat/list", groupChat.ValidateGroupChatList, wx.APIGetGroupChatList)
			apiRouter.GET("/groupChat/detail", groupChat.ValidateGroupChatDetail, wx.APIGetGroupChatDetail)

			apiRouter.GET("/wxPlatform/wecom/groupChat/list", groupChat.ValidateWXPlatformGroupChatList, wx.APIGetGroupChatListOnWXPlatform)
			apiRouter.POST("/wxPlatform/wecom/groupChat/detail", groupChat.ValidateWXPlatformGroupChatDetail, wx.APIGetGroupChatDetailOnWXPlatform)

			//  send chat message - 发送客户群发消息接口
			apiRouter.GET("/sendChatMessage/doSend", admin.APIDoSendChatMsgs)
			apiRouter.POST("/sendChatMessage/sync", sendChatMsg.ValidateSendChatMsgSync, admin.APISendChatMsgSync)
			apiRouter.POST("/sendChatMessage/list", sendChatMsg.ValidateSendChatMsgList, admin.APIGetSendChatMsgList)
			apiRouter.GET("/sendChatMessage/detail", sendChatMsg.ValidateSendChatMsgDetail, admin.APIGetSendChatMsgDetail)
			apiRouter.POST("/sendChatMessage/create", sendChatMsg.ValidateCreateSendChatMsg, admin.APICreateSendChatMsg)
			apiRouter.POST("/sendChatMessage/estimateExternalUsers", sendChatMsg.ValidateCreateSendChatMsg, admin.APIEstimateSendChatCustomersCount)

			//  send group chat message - 发送客户群群发消息接口
			apiRouter.GET("/sendGroupChatMessage/doSend", admin.APIDoSendGroupChatMsgs)
			apiRouter.POST("/sendGroupChatMessage/sync", sendGroupChatMsg.ValidateSendGroupChatMsgSync, admin.APISendGroupChatMsgSync)
			apiRouter.POST("/sendGroupChatMessage/list", sendGroupChatMsg.ValidateSendGroupChatMsgList, admin.APIGetSendGroupChatMsgList)
			apiRouter.GET("/sendGroupChatMessage/detail", sendGroupChatMsg.ValidateSendGroupChatMsgDetail, admin.APIGetSendGroupChatMsgDetail)
			apiRouter.POST("/sendGroupChatMessage/create", sendGroupChatMsg.ValidateCreateSendGroupChatMsg, admin.APICreateSendGroupChatMsg)
			apiRouter.POST("/sendGroupChatMessage/estimateExternalUsers", sendGroupChatMsg.ValidateCreateSendGroupChatMsg, admin.APIEstimateSendGroupChatCustomersCount)

			apiRouter.GET("/permission/policy/list", policy.ValidatePolicyList, admin.APIGetPolicyList)
			apiRouter.PUT("/permission/policy/update", policy.ValidateUpdatePolicy, admin.APIUpdatePolicy)

		}
	}

}
