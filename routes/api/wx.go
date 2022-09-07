package api

import (
	apiWX "github.com/ArtisanCloud/PowerX/app/http/controllers/wx"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	requestWX "github.com/ArtisanCloud/PowerX/app/http/request/wx"
	"github.com/ArtisanCloud/PowerX/routes/global"
)

func InitWXRoutes() {
	/* ------------------------------------------ wechat api ------------------------------------------*/
	apiWechatRouter := global.Router.Group("/wechat/api")
	{

		apiWechatRouter.Use(middleware.CheckInstalled, middleware.Maintenance)
		{
			// ------
			// ------------------------------------------------------------ Mini Program ------------------------------------------------------------
			// ------
			// --- 小程序客户code换取session信息 ---
			apiWechatRouter.GET("/miniporgram/oauth2/authorize/customer", requestWX.ValidateMiniProgramCode2Session, apiWX.APIMiniProgramCode2Session)

			// ------
			// ------------------------------------------------------------ WeCom ------------------------------------------------------------
			// ------
			// --- 客户回调请求地址 ---
			// https://developer.work.weixin.qq.com/document/path/92129
			// 该回调在企业微信管理后台的“客户联系-客户”页面，点开“API”小按钮，再点击“接收事件服务器”配置，进入配置页面，要求填写URL、Token、EncodingAESKey三个参数。
			apiWechatRouter.GET("/wecom/customer", apiWX.APICallbackValidationCustomer)
			apiWechatRouter.POST("/wecom/customer", apiWX.APICallbackCustomer)

			// --- 员工回调地址 ---
			// https://developer.work.weixin.qq.com/document/path/90967
			// 在企业微信管理后台的“管理工具-通讯录同步-设置接收事件服务器”处，进入配置页面，要求填写通讯录同步助手的URL、Token、EncodingAESKey三个参数。为保证企业数据安全，URL需要配置本企业主体的域名链接。
			apiWechatRouter.GET("/wecom/employee", apiWX.APICallbackValidationEmployee)
			apiWechatRouter.POST("/wecom/employee", apiWX.APICallbackEmployee)

			// --- 网页授权员工登陆，获取访问code ---
			apiWechatRouter.GET("/wecom/oauth2/authorize/employee", apiWX.WeComToAuthorizeEmployee)
			// --- 网页授权员工登陆，code换取访问token ---
			apiWechatRouter.GET("/wecom/callback/authorized/employee", requestWX.ValidateRequestOAuthCallback, apiWX.WeComAuthorizedEmployee)

			// --- 网页扫码授权员工登陆，获取访问code ---
			apiWechatRouter.GET("/wecom/oauth2/authorize/qr/employee", apiWX.WeComToAuthorizeQREmployee)
			// --- 网页扫码授权员工登陆，code换取访问token ---
			apiWechatRouter.GET("/wecom/callback/authorized/qr/employee", requestWX.ValidateRequestOAuthCallbackQRCode, apiWX.WeComAuthorizedEmployeeQR)

			// --- 网页授权客户登陆，获取访问code ---
			apiWechatRouter.GET("/wecom/oauth2/authorize/customer", apiWX.WeComToAuthorizeCustomer)
			// --- 网页授权客户登陆，code换取访问token ---
			apiWechatRouter.GET("/wecom/callback/authorized/customer/", requestWX.ValidateRequestOAuthCallback, apiWX.WeComAuthorizedCustomer)

			apiWechatRouter.Use(middleware.AuthenticateEmployeeByHeader)
			{
				// 获取企业微信回调IP地址
				apiWechatRouter.GET("/getCallbackIPs", apiWX.APIGetCallbackIPs)

			}

		}

	}

}
