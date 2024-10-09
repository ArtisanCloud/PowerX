package wechat

import (
	"PowerX/internal/model/scene"
	"PowerX/internal/model/scrm/customer"
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/model/scrm/resource"
	"PowerX/internal/model/scrm/tag"
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
	tagReq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/request"
	tagResp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/tag/response"
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
	//  @Description: User
	//
	iWeWorkUserInterface

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

	//
	//  @Description: qrcode
	//
	iQrcodeInterface
	//
	//  @Description: tag
	//
	iTagInterface
}

// iWeWorkDepartmentInterface
// @Description: 部门
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

// iWeWorkUserInterface
// @Description: 员工
type iWeWorkUserInterface interface {
	//
	// PullSyncDepartmentsAndUsersRequest
	//  @Description: 同步组织架构
	//  @param ctx
	//  @return error
	//
	PullSyncDepartmentsAndUsersRequest(ctx context.Context) error
	//
	// FindManyWechatUsersPage
	//  @Description: 查询员工
	//  @param ctx
	//  @param opt
	//  @return *types.Page[*organization.WeWorkUser]
	//
	FindManyWechatUsersPage(ctx context.Context, opt *types.PageOption[FindManyWechatUsersOption]) (*types.Page[*organization.WeWorkUser], error)
}

// iWeWorkCustomerInterface
// @Description: 客户
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
	//  @return *types.Page[*customer.WeWorkExternalContact]
	//  @return error
	//
	FindManyWeWorkCustomerPage(ctx context.Context, opt *types.PageOption[FindManyWechatCustomerOption], sync int) (*types.Page[*customer.WeWorkExternalContact], error)

	//
	// PushWoWorkCustomerTemplateRequest
	//  @Description: 发送客户群信息1/day
	//  @param opt
	//  @param sendTime
	//  @return *crsp.ResponseAddMessageTemplate
	//  @return error
	//
	PushWoWorkCustomerTemplateRequest(opt *creq.RequestAddMsgTemplate, sendTime int64) (*crsp.ResponseAddMessageTemplate, error)
}

// iWeWorkBotInterface
// @Description:
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

// iWeWorkAppInterface
// @Description:
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

// iMakeInvokeInterface
// @Description: 消费信息
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

// iCommonInterface
// @Description:
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

// iQrcodeInterface
// @Description: 活码
type iQrcodeInterface interface {

	//
	// CreateWeWorkCustomerGroupQrcodeRequest
	//  @Description: 创建群活码
	//  @param opt
	//  @return err
	//
	CreateWeWorkCustomerGroupQrcodeRequest(opt *types.QrcodeActiveRequest) (err error)
	//
	// UpdateWeWorkCustomerGroupQrcodeRequest
	//  @Description: 更新群活码
	//  @param opt
	//  @return err
	//
	UpdateWeWorkCustomerGroupQrcodeRequest(opt *types.QrcodeActiveRequest) (err error)
	//
	// FindWeWorkCustomerGroupQrcodePage
	//  @Description: 客户群活码
	//  @param opt
	//  @return reply
	//  @return err
	//
	FindWeWorkCustomerGroupQrcodePage(opt *types.PageOption[types.ListWeWorkGroupQrcodeActiveReqeust]) (reply *types.Page[*scene.SceneQRCode], err error)

	//
	// ActionCustomerGroupQrcode
	//  @Description: 启用，禁用，删除
	//  @param qid
	//  @param action
	//  @return error
	//
	ActionCustomerGroupQrcode(qid string, action int) error

	//
	// UpdateSceneQRCodeLink
	//  @Description: 更新场景码
	//  @param qid
	//  @param link
	//  @return error
	//
	UpdateSceneQRCodeLink(qid string, link string) error
}

// iTagInterface
// @Description: TAG
type iTagInterface interface {
	//
	// FindListWeWorkTagGroupOption
	//  @Description: 标签组
	//  @return reply
	//  @return err
	//
	FindListWeWorkTagGroupOption() (reply []*tag.WeWorkTagGroup, err error)
	//
	// FindListWeWorkTagGroupPage
	//  @Description: 标签组分页
	//  @param option
	//  @return reply
	//  @return err
	//
	FindListWeWorkTagGroupPage(option *types.PageOption[types.ListWeWorkTagGroupPageRequest]) (reply *types.Page[*tag.WeWorkTagGroup], err error)

	//
	// ActionWeWorkCorpTagGroupRequest
	//  @Description: 添加，删除标签组内的标签
	//  @param options
	//  @return work
	//  @return err
	//
	ActionWeWorkCorpTagGroupRequest(options *types.ActionCorpTagGroupRequest) (work *kresp.ResponseWork, err error)

	//
	// FindListWeWorkTagOption
	//  @Description: 标签
	//  @return reply
	//  @return err
	//
	FindListWeWorkTagOption() (reply []*tag.WeWorkTag, err error)
	//
	// FindListWeWorkTagPage
	//  @Description: 企业标签查询
	//  @param option
	//  @return reply
	//  @return err
	//
	FindListWeWorkTagPage(option *types.PageOption[types.ListWeWorkTagReqeust]) (reply *types.Page[*tag.WeWorkTag], err error)
	//
	// PullListWeWorkCorpTagRequest
	//  @Description: 企业标签
	//  @param tagIds
	//  @param groupIds
	//  @param sync
	//  @return reply
	//  @return err
	//
	PullListWeWorkCorpTagRequest(tagIds []string, groupIds []string, sync int) (reply *tagResp.ResponseTagGetCorpTagList, err error)

	//
	// PullListWeWorkStrategyTagRequest
	//  @Description: 策略标签
	//  @param options
	//  @return reply
	//  @return err
	//
	PullListWeWorkStrategyTagRequest(options *tagReq.RequestTagGetStrategyTagList) (reply *tagResp.ResponseTagGetStrategyTagList, err error)

	//
	// CreateWeWorkCorpTagRequest
	//  @Description: 创建企业标签
	//  @param options
	//  @return *tagResp.ResponseTagAddCorpTag
	//  @return error
	//
	CreateWeWorkCorpTagRequest(options *tagReq.RequestTagAddCorpTag) (*tagResp.ResponseTagAddCorpTag, error)

	//
	// UpdateWeWorkCorpTagRequest
	//  @Description: 更新企业标签
	//  @param options
	//  @return *kresp.ResponseWork
	//  @return error
	//
	UpdateWeWorkCorpTagRequest(options *tagReq.RequestTagEditCorpTag) (*kresp.ResponseWork, error)

	//
	// DeleteWeWorkCorpTagRequest
	//  @Description: 删除标签
	//  @param options
	//  @return *kresp.ResponseWork
	//  @return error
	//
	DeleteWeWorkCorpTagRequest(options *tagReq.RequestTagDelCorpTag) (*kresp.ResponseWork, error)

	//
	// ActionWeWorkCustomerTagRequest
	//  @Description: 添加/移除客户标签
	//  @param option
	//  @return *kresp.ResponseWork
	//  @return error
	//
	ActionWeWorkCustomerTagRequest(option *tagReq.RequestTagMarkTag) (*kresp.ResponseWork, error)
}
