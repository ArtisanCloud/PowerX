package api

import (
	apiWX "github.com/ArtisanCloud/PowerX/app/http/controllers/wx"
	"github.com/ArtisanCloud/PowerX/app/http/middleware"
	requestWX "github.com/ArtisanCloud/PowerX/app/http/request/wx"
	"github.com/gin-gonic/gin"
)

func InitWXRoutes(router *gin.Engine) {
	/* ------------------------------------------ wx api ------------------------------------------*/
	apiRouter := router.Group("/wx/api")
	{

		// ------
		// ------------------------------------------------------------ Mini Program ------------------------------------------------------------
		// ------
		// --- 小程序客户code换取session信息 ---
		apiRouter.GET("/miniporgram/oauth2/authorize/customer", requestWX.ValidateMiniProgramCode2Session, apiWX.APIMiniProgramCode2Session)

		// ------
		// ------------------------------------------------------------ WeCom ------------------------------------------------------------
		// ------
		// --- customer callback url ---
		apiRouter.GET("/wecom/customer", apiWX.APICallbackValidationCustomer)
		apiRouter.POST("/wecom/customer", apiWX.APICallbackCustomer)

		// --- employee callback url ---
		apiRouter.GET("/wecom/employee", apiWX.APICallbackValidationEmployee)
		apiRouter.POST("/wecom/employee", apiWX.APICallbackEmployee)

		// --- 网页授权员工登陆，获取访问code ---
		apiRouter.GET("/wecom/oauth2/authorize/employee", apiWX.WeComToAuthorizeEmployee)
		// --- 网页授权员工登陆，code换取访问token ---
		apiRouter.GET("/wecom/callback/authorized/employee", requestWX.ValidateRequestOAuthCallback, apiWX.WeComAuthorizedEmployee)

		// --- 网页扫码授权员工登陆，获取访问code ---
		apiRouter.GET("/wecom/oauth2/authorize/qr/employee", apiWX.WeComToAuthorizeQREmployee)
		// --- 网页扫码授权员工登陆，code换取访问token ---
		apiRouter.GET("/wecom/callback/authorized/qr/employee", requestWX.ValidateRequestOAuthCallbackQRCode, apiWX.WeComAuthorizedEmployeeQR)

		// --- 网页授权客户登陆，获取访问code ---
		apiRouter.GET("/wecom/oauth2/authorize/customer", apiWX.WeComToAuthorizeCustomer)
		// --- 网页授权客户登陆，code换取访问token ---
		apiRouter.GET("/wecom/callback/authorized/customer/", requestWX.ValidateRequestOAuthCallback, apiWX.WeComAuthorizedCustomer)

		apiRouter.Use(middleware.Maintenance, middleware.AuthEmployeeAPI)
		{
			// 获取企业微信回调IP地址
			apiRouter.GET("/getCallbackIPs", apiWX.APIGetCallbackIPs)

		}

	}

}
