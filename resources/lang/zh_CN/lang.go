package zh_CN

import (
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var lang = language.Chinese

func LoadLang() {
	message.SetString(lang, fmt2.Sprintf("%d", config.API_RETURN_CODE_INIT), "")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_RETURN_CODE_WARNING), "警告消息")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_RETURN_CODE_ERROR), "错误消息")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_RESULT_CODE_INIT), "")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_RESULT_CODE_SUCCESS_RESET_PASSWORD), "密码修改成功")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_WARNING_CODE_SYSTEM_NOT_INSTALLED), "请先安装本系统")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_WARNING_CODE_SYSTEM_INSTALLED), "系统已经安装过，请咨询系统开发者")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_WARNING_CODE_IN_MAINTENANCE), "系统维护中")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_WARNING_CODE_NEED_UPDATE), "推出新版本，请更新最新的版本")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSTALL_SYSTEM), "安装系统失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SHUT_DOWN_SYSTEM), "关闭系统失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CHECK_ROOT), "检查Root初始化失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_REQUEST_PARAM_ERROR), "请求参数错误")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_HEADER_RESELLER), "请求参数错误-销售渠道")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_TOKEN_NOT_IN_HEADER), "请求参数错误-请求token")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_USER_UNREGISTER), "user未注册")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_REPORT_LIST), "获取客户列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_REPORT), "新增或更新客户列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_REPORT), "删除客户列表失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_LIST), "获取员工列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_EMPLOYEE), "新增或更新员工列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_EMPLOYEE_DETAIL), "获取员工失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_EMPLOYEE), "删除员工列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_CORD_ID), "获取CordID失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_BIND_CUSOTMER_TO_EMPLOYEE), "绑定客户到员工失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_EMPLOYEE_UNREGISTER), "员工未注册")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_EMPLOYEE_STATUS_NOT_ACTIVE), "员工在微信状态未激活")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_LIST), "获取部门列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_DETAIL), "获取部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_DEPARTMENT), "新增部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_DEPARTMENT), "更新部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_DEPARTMENT), "删除部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_LIST_ON_WX_PLATFORM), "同步获取部门列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_DEPARTMENT_DETAIL_ON_WX_PLATFORM), "同步获取部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_DEPARTMENT_ON_WX_PLATFORM), "同步新增部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_DEPARTMENT_ON_WX_PLATFORM), "同步更新部门失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_DEPARTMENT_ON_WX_PLATFORM), "同步删除部门失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_DEPARTMENT), "新增部门列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_DEPARTMENT), "新增或更新部门列表失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_MESSAGE_SEND), "发送消息失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_ADD_MESSAGE_TEMPLATE), "添加消息模版失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SEND_WELOCME_MESSAGE), "发送欢迎语失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_ACCOUNT_INVALID_TOKEN), "访问Token失效")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_ACCOUNT_UNREGISTER), "客户账号未注册")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_LACK_OF_WX_EXTERNAL_USER_ID), "缺少企业微信外部用户ID")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_LACK_OF_WX_USER_ID), "缺少企业微信用户ID")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_LIST), "获取客户列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_ACCOUNT), "新增或更新客户列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_ACCOUNT), "删除客户列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ACCOUNT_DETAIL), "获取客户详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_WECOM_AGENT_ID_INVALID), "企微的应用ID无效")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_SIGNATURE_FILE_FAILED), "签名验证文件失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_REPORT_LIST), "获取报表列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_REPORT), "新增或更新报表列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_REPORT), "删除报表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_REPORT_DETAIL), "获取报表列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_HOURLY_REPORT), "获取时报时报")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_DAILY_REPORT), "获取日报时报")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_WEEKLY_REPORT), "获取周报时报")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MONTHLY_REPORT), "获取月报时报")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SEND_ACCOUNT_REPROT), "发送客户报表失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PRODUCT_LIST), "获取产品列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_PRODUCT), "新增或更新产品失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_PRODUCT), "删除产品失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PRODUCT_DETAIL), "获取产品详情失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_PURCHASE_PRODUCT), "购买产品失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PRICE_BOOK_LIST), "获取价格手册列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_PRICE_BOOK), "新增或更新价格手册失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_PRICE_BOOK), "删除价格手册失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PRICE_BOOK_DETAIL), "获取价格手册详情失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PRICE_BOOK_ENTRY_DETAIL), "获取价格手册条目详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ORDER_LIST), "获取订单列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_ORDER), "新增或更新订单失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_ORDER), "删除订单失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ORDER_DETAIL), "获取订单详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PAYMENT_LIST), "获取支付单列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_PAYMENT), "新增或更新支付单失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_PAYMENT), "删除支付单失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PAYMENT_DETAIL), "获取支付单详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MEMBERSHIP_LIST), "获取会籍列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_MEMBERSHIP), "新增或更新会籍失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_MEMBERSHIP), "删除会籍失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MEMBERSHIP_DETAIL), "获取会籍详情失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GENERATE_MEMBERSHIP), "生成会籍失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_FISSION_LIST), "获取裂变列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_FISSION), "新增或更新裂变失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_FISSION), "删除裂变失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_FISSION_DETAIL), "获取裂变详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_COUPON_LIST), "获取优惠券列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_COUPON), "新增或更新优惠券失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_COUPON), "删除优惠券失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_COUPON_DETAIL), "获取优惠券详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_RESELLER_LIST), "获取销售渠道列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_RESELLER), "新增或更新销售渠道失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_RESELLER), "删除销售渠道失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_RESELLER_DETAIL), "获取销售渠道详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_COMMISSION_LIST), "获取分润列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_COMMISSION), "新增或更新分润失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_COMMISSION), "删除分润失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_COMMISSION_DETAIL), "获取分润详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MGM_LIST), "获取MGM列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_MGM), "新增或更新MGM失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_MGM), "删除MGM失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MGM_DETAIL), "获取MGM详情失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_LIST), "获取ContactWay列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_DETAIL), "获取ContactWay详情失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_CONTACT_WAY), "新增ContactWay失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_CONTACT_WAY), "更新ContactWay失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_CONTACT_WAY), "删除ContactWay失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_CONTACT_WAY_ON_WX_PLATFORM), "上传新增ContactWay失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_CONTACT_WAY_ON_WX_PLATFORM), "上传更新ContactWay失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_CONTACT_WAY_ON_WX_PLATFORM), "上传删除ContactWay失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_GROUP_LIST), "获取ContactWay组列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_CONTACT_WAY_GROUP_DETAIL), "获取ContactWay组失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_CONTACT_WAY_GROUP), "新增ContactWay组失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_CONTACT_WAY_GROUP), "更新ContactWay组失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_CONTACT_WAY_GROUP), "删除ContactWay组失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_LOAD_CONTACT_WAY_LIST), "加载ContactWay列表失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_GROUP_LIST), "获取企微组标签列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_GROUP_DETAIL), "获取企微组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP_ON_WX_PLATFORM), "新增企微组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_GROUP), "更新企微组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_GROUP), "删除企微组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG_GROUP_ON_WX_PLATFORM), "上传新增微信组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG_GROUP), "上传更新微信组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_GROUP_ON_WX_PLATFORM), "上传删除微信组标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG_GROUP_ON_WX_PLATFORM), "同步微信组标签失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_TAG_LIST), "获取标签列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_TAG_DETAIL), "获取标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPSERT_TAG), "新增标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_TAG), "更新标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_TAG), "删除标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_TAG), "上传新增标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_TAG), "同步标签失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_LIST), "获取企微标签列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_WX_TAG_DETAIL), "获取企微标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG), "新增企微标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG), "更新企微标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG), "删除企微标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_WX_TAG_ON_WX_PLATFORM), "上传新增微信标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_WX_TAG_ON_WX_PLATFORM), "上传更新微信标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_WX_TAG_ON_WX_PLATFORM), "上传删除微信标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG_ON_WX_PLATFORM), "上传同步微信标签失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_WX_TAG), "同步微信标签失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_IMAGE), "上传企微图片失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_UPLOAD_MEDIA), "上传企微媒体失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_WECOM_MEDIA_GET_MEDIA), "下载企微媒体失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_LIST), "获取客户群列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL), "获取客户群失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_LIST_ON_WX_PLATFORM), "下载获取客户群列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_GROUP_CHAT_DETAIL_ON_WX_PLATFORM), "下载获取客户群失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_GROUP_CHAT_ON_WX_PLATFORM), "同步客户群失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MESSAGE_TEMPLATE_LIST), "获取消息模版列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_MESSAGE_TEMPLATE_DETAIL), "获取消息模版失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_MESSAGE_TEMPLATE), "下载获取消息模版列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_MESSAGE_TEMPLATE), "下载获取消息模版失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_MESSAGE_TEMPLATE), "同步消息模版失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_MESSAGE_TEMPLATE), "同步消息模版失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_LIST), "获取客户群发列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_SEND_CHAT_MSG_DETAIL), "获取客户群发失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_SEND_CHAT_MSG), "新增客户群发失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_ESTIMATE_SEND_CHAT_MSG_CUSTOMERS_COUNT), "估算发送客户群发的客户数量失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_LIST), "获取客户群群发列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_SEND_GROUP_CHAT_MSG_DETAIL), "获取客户群群发失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CREATE_SEND_GROUP_CHAT_MSG), "新增客户群群发失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_ESTIMATE_SEND_GROUP_CHAT_MSG_CUSTOMERS_COUNT), "估算发送客户群群发的客户数量失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_SYNC_SEND_CHAT_MSG_ON_WX_PLATFORM), "删除客户群发失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ROLE_LIST), "获取角色列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ROLE_DETAIL), "获取角色失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_ROLE), "新增角色失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_ROLE), "更新角色失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_ROLE), "删除角色失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_BIND_ROLE_TO_EMPLOYEE), "角色绑定员工失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_EMPLOYEE_HAS_NO_ROLE), "员工未分配角色")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PERMISSION_LIST), "获取权限列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PERMISSION_DETAIL), "获取权限失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_PERMISSION), "新增权限失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_PERMISSION), "更新权限失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_PERMISSION), "删除权限失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PERMISSION_MODULE_LIST), "获取权限模块列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_PERMISSION_MODULE_DETAIL), "获取权限模块失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_INSERT_PERMISSION_MODULE), "新增权限模块失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_PERMISSION_MODULE), "更新权限模块失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_DELETE_PERMISSION_MODULE), "删除权限模块失败")

	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_GET_ROLE_POLICY_LIST), "获取权角色限策略列表失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_UPDATE_ROLE_POLICY), "更新角色权限策略失败")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_AUTHORIZATE_ROLE), "该登陆角色无操作权限")
	message.SetString(lang, fmt2.Sprintf("%d", config.API_ERR_CODE_FAIL_TO_CLEAR_CACHE_ROLE_POLICY), "清除角色权限缓存失败")

}
