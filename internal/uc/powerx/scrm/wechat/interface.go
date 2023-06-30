package wechat

import (
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/types"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    kresp "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
    agentResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/agent/response"
    customerGroupReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
    customerGroupResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"
    customerResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/response"
    botReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/request"
    botResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/groupRobot/response"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/response"
    appReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
    appResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/response"
)

type IWechatInterface interface {
    //
    //  @Description:  Department
    //
    iWechatDepartmentInterface

    //
    //  @Description: Employee
    //
    iWechatEmployeeInterface

    //
    //  @Description: Customer
    //
    iWechatCustomerInterface

    //
    //  @Description: App
    //
    iWechatAppInterface

    //
    //  @Description:  Bot
    //
    iWechatBotInterface
}

//
//  iWechatDepartmentInterface
//  @Description: 部门
//
type iWechatDepartmentInterface interface {
    //
    // CreateWechatDepartment
    //  @Description: 创建部门
    //  @param ctx
    //  @param dep
    //  @return err
    //
    //CreateWechatDepartment(ctx context.Context, dep *organization.WeWorkDepartment) (err error)

    //
    // FindManyWechatDepartmentsPage
    //  @Description: 查询部门
    //  @param ctx
    //  @param option
    //  @return *types.Page[*organization.Department]
    //
    FindManyWechatDepartmentsPage(ctx context.Context, option *types.PageOption[FindManyWechatDepartmentsOption]) (*types.Page[*organization.WeWorkDepartment], error)
}

//
//  iWechatEmployeeInterface
//  @Description: 员工
//
type iWechatEmployeeInterface interface {
    //
    // SyncDepartmentsAndEmployees
    //  @Description: 同步数据
    //  @param ctx
    //  @return error
    //
    SyncDepartmentsAndEmployees(ctx context.Context) error
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
//  iWechatCustomerInterface
//  @Description: 客户
//
type iWechatCustomerInterface interface {
    //
    // CustomerListWechatWorkRequest
    //  @Description: 客户列表
    //  @param userID
    //  @return []*customerResp.ResponseExternalContact
    //  @return error
    //
    CustomerListWechatWorkRequest(userID ...string) ([]*customerResp.ResponseExternalContact, error)
    //
    // CustomerGroupListWechatWorkRequest
    //  @Description: 客户群列表
    //  @param opt
    //  @return list
    //  @return err
    //
    CustomerGroupListWechatWorkRequest(opt *customerGroupReq.RequestGroupChatList) (list []*customerGroupResp.ResponseGroupChatGet, err error)
}

//
//  iWechatBotInterface
//  @Description:
//
type iWechatBotInterface interface {
    //
    // BotArticles
    //  @Description: 机器人发送图文
    //  @param key
    //  @param articles
    //  @return resp
    //  @return error
    //
    BotArticles(key string, articles []*botReq.GroupRobotMsgNewsArticles) (resp *botResp.ResponseGroupRobotSend, err error)
}

//
//  iWechatAppInterface
//  @Description:
//
type iWechatAppInterface interface {
    //
    // AppWechatDetailRequest
    //  @Description: 应用详情
    //  @param agentID
    //  @return reply
    //  @return err
    //
    AppWechatDetailRequest(agentID int) (reply *agentResp.ResponseAgentGet, err error)

    //
    // AppWechatListRequest
    //  @Description: 应用列表
    //  @return reply
    //  @return err
    //
    AppWechatListRequest() (reply *agentResp.ResponseAgentList, err error)

    //
    // AppWechatMessageArticles
    //  @Description: 发送应用图文信息
    //  @param opt
    //  @return *appResp.ResponseMessageSend
    //  @return error
    //
    AppWechatMessageArticles(opt *appReq.RequestMessageSendNews) (*appResp.ResponseMessageSend, error)

    iWechatAppGroupInterface
}

type iWechatAppGroupInterface interface {
    //
    // AppWechatGroupDetail
    //  @Description: 获取应用群聊
    //  @param chatID
    //  @return reply
    //  @return err
    //
    AppWechatGroupDetail(chatID string) (reply *response.ResponseAppChatGet, err error)

    //
    // AppWechatGroupCreate
    //  @Description: 创建应用群聊
    //  @param option
    //  @return reply
    //  @return err
    //
    AppWechatGroupCreate(option *request.RequestAppChatCreate) (reply *response.ResponseAppChatCreate, err error)

    //
    // AppWechatGroupMessageArticles
    //  @Description: 群内推送
    //  @param messages
    //  @return resp
    //  @return err
    //
    AppWechatGroupMessageArticles(messages *power.HashMap) (resp *kresp.ResponseWork, err error)
}
