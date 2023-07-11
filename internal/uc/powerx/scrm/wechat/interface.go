package wechat

import (
    "PowerX/internal/model/scrm/customer"
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/model/scrm/resource"
    "PowerX/internal/types"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    kresp "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
    agentResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"
    customerGroupReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
    customerGroupResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"
    creq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/request"
    crsp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/response"
    customerResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"
    botReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"
    botResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/response"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/response"
    appReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
    appResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/response"
    "mime/multipart"
)

type IWechatInterface interface {
    //
    //  @Description:  Department
    //
    iWeWorkDepartmentInterface

    //
    //  @Description: Employee
    //
    iWeWorkEmployeeInterface

    //
    //  @Description: Customer
    //
    iWeWorkCustomerInterface

    //
    //  @Description: App
    //
    iWeWorkAppInterface

    //
    //  @Description:  Bot
    //
    iWeWorkBotInterface

    //
    //  @Description: invoke
    //
    iMakeInvokeInterface

    //
    //  @Description: common
    //
    iCommonInterface
}

//
//  iWeWorkDepartmentInterface
//  @Description: 部门
//
type iWeWorkDepartmentInterface interface {
    //
    // CreateWechatDepartment
    //  @Description: 创建部门
    //  @param ctx
    //  @param dep
    //  @return err
    //
    //CreateWechatDepartment(ctx context.Context, dep *organization.WeWorkDepartment) (err error)

    //
    // FindManyWeWorkDepartmentsPage
    //  @Description: 查询部门
    //  @param ctx
    //  @param option
    //  @return *types.Page[*organization.Department]
    //
    FindManyWeWorkDepartmentsPage(ctx context.Context, option *types.PageOption[FindManyWechatDepartmentsOption]) (*types.Page[*organization.WeWorkDepartment], error)
}

//
//  iWeWorkEmployeeInterface
//  @Description: 员工
//
type iWeWorkEmployeeInterface interface {
    //
    // PullSyncDepartmentsAndEmployeesRequest
    //  @Description: 同步组织架构
    //  @param ctx
    //  @return error
    //
    PullSyncDepartmentsAndEmployeesRequest(ctx context.Context) error
    //
    // FindManyWechatEmployeesPage
    //  @Description: 查询员工
    //  @param ctx
    //  @param opt
    //  @return *types.Page[*organization.WeWorkEmployee]
    //
    FindManyWechatEmployeesPage(ctx context.Context, opt *types.PageOption[FindManyWechatEmployeesOption]) (*types.Page[*organization.WeWorkEmployee], error)
}

//
//  iWeWorkCustomerInterface
//  @Description: 客户
//
type iWeWorkCustomerInterface interface {
    //
    // PullListWeWorkCustomerRequest
    //  @Description: 拉取客户列表
    //  @param userID
    //  @return []*customerResp.ResponseExternalContact
    //  @return error
    //
    PullListWeWorkCustomerRequest(userID ...string) ([]*customerResp.ResponseExternalContact, error)
    //
    // PullListWeWorkCustomerGroupRequest
    //  @Description: 拉取客户群列表
    //  @param opt
    //  @return list
    //  @return err
    //
    PullListWeWorkCustomerGroupRequest(opt *customerGroupReq.RequestGroupChatList) (list []*customerGroupResp.ResponseGroupChatGet, err error)

    //
    // FindManyWechatCustomerPage
    //  @Description: 所有客户
    //  @param ctx
    //  @param opt
    //  @param sync
    //  @return *types.Page[*customer.WeWorkExternalContacts]
    //  @return error
    //
    FindManyWeWorkCustomerPage(ctx context.Context, opt *types.PageOption[FindManyWechatCustomerOption], sync int) (*types.Page[*customer.WeWorkExternalContacts], error)

    //
    // PushWoWorkCustomerTemplateRequest
    //  @Description: 发送客户群信息1/day
    //  @param opt
    //  @return *crsp.ResponseAddMessageTemplate
    //  @return error
    //
    PushWoWorkCustomerTemplateRequest(opt *creq.RequestAddMsgTemplate) (*crsp.ResponseAddMessageTemplate, error)
}

//
//  iWeWorkBotInterface
//  @Description:
//
type iWeWorkBotInterface interface {
    //
    // PushWeWorkBotArticlesRequest
    //  @Description: 机器人发送图文
    //  @param key
    //  @param articles
    //  @return resp
    //  @return error
    //
    PushWeWorkBotArticlesRequest(key string, articles []*botReq.GroupRobotMsgNewsArticles) (resp *botResp.ResponseGroupRobotSend, err error)
}

//
//  iWeWorkAppInterface
//  @Description:
//
type iWeWorkAppInterface interface {
    //
    // PullDetailWeWorkAppRequest
    //  @Description: 应用详情
    //  @param agentID
    //  @return reply
    //  @return err
    //
    PullDetailWeWorkAppRequest(agentID int) (reply *agentResp.ResponseAgentGet, err error)

    //
    // PullListWeWorkAppRequest
    //  @Description: 应用列表
    //  @return reply
    //  @return err
    //
    PullListWeWorkAppRequest() (reply *agentResp.ResponseAgentList, err error)

    //
    // PushAppWeWorkMessageArticlesRequest
    //  @Description: 发送应用图文信息
    //  @param opt
    //  @return *appResp.ResponseMessageSend
    //  @return error
    //
    PushAppWeWorkMessageArticlesRequest(opt *appReq.RequestMessageSendNews, sendTime int64) (reply *appResp.ResponseMessageSend, err error)

    iWeWorkAppGroupInterface
}

type iWeWorkAppGroupInterface interface {
    //
    // PullListWeWorkAppGroupRequest
    //  @Description: 获取应用群聊
    //  @param chatID
    //  @return reply
    //  @return err
    //
    PullListWeWorkAppGroupRequest(chatIDs ...string) (replys []*power.HashMap, err error)
    //
    // AppWechatGroupCreate
    //  @Description: 创建应用群聊
    //  @param option
    //  @return reply
    //  @return err
    //
    CreateWeWorkAppGroupRequest(option *request.RequestAppChatCreate) (reply *response.ResponseAppChatCreate, err error)

    //
    // PushAppWeWorkGroupMessageArticlesRequest
    //  @Description: 群内推送
    //  @param messages
    //  @return resp
    //  @return err
    //
    PushAppWeWorkGroupMessageArticlesRequest(messages *power.HashMap, sendTime int64) (resp *kresp.ResponseWork, err error)
}

//
//  iMakeInvokeInterface
//  @Description: 消费信息
//
type iMakeInvokeInterface interface {
    //
    // InvokeTimerMessageGrabUniteSend
    //  @Description:
    //  @param ttp
    //  @param sendTime
    //  @return count
    //
    InvokeTimerMessageGrabUniteSend(ttp TimerTypeByte, sendTime int64) error
}

//
//  iCommonInterface
//  @Description:
//
type iCommonInterface interface {
    //
    // UploadImageToResourceRequest
    //  @Description: 上传图片到微信
    //  @param path
    //  @param handle
    //  @return link
    //  @return err
    //
    UploadImageToResourceRequest(path string, handle *multipart.FileHeader) (link string, err error)

    //
    // FindWeWorkResourceListFromLocalPage
    //  @Description: FindWeWorkResourceListFromLocalPage
    //  @param opt
    //  @return *types.Page[*resource.WeWorkResource]
    //  @return error
    //
    FindWeWorkResourceListFromLocalPage(opt *types.ListWeWorkResourceImageRequest) (*types.Page[*resource.WeWorkResource], error)
}
