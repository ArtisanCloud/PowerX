package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"PowerX/internal/uc/openapi"
	fmt "PowerX/pkg/printx"
	"context"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

type OpenAPIPlatformGetMiddleware struct {
	conf *config.Config
	px   *uc.PowerXUseCase
}

func NewOpenAPIPlatformGetMiddleware(conf *config.Config, px *uc.PowerXUseCase, opts ...OptionFunc) *OpenAPIPlatformGetMiddleware {
	return &OpenAPIPlatformGetMiddleware{
		conf: conf,
		px:   px,
	}
}

func (m *OpenAPIPlatformGetMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		unAuth := errorx.ErrUnAuthorization.(*errorx.Error)

		vPlatformId := r.Context().Value(openapi.AuthPlatformKey)
		if vPlatformId == nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效授权平台Id"))
			return
		}
		fmt.Dump(vPlatformId)
		platformId := vPlatformId.(string)
		if platformId == "" {
			httpx.Error(w, errorx.WithCause(unAuth, "授权授权平台Id为空"))
			return
		}

		//// 平台记录是否存在
		//authMPCustomer, err := m.px.WechatMP.FindOneMPCustomer(r.Context(), &wechat.FindMPCustomerOption{
		//	OpenIds: []string{openId},
		//})
		//if err != nil {
		//	httpx.Error(w, errorx.WithCause(unAuth, "无效微信小程序客户"))
		//	return
		//}

		// 平台记录是否存在
		//if authMPCustomer.Customer == nil {
		//	httpx.Error(w, errorx.WithCause(unAuth, "无效客户记录"))
		//	return
		//}

		ctx := context.WithValue(r.Context(), openapi.AuthPlatformKey, "tobe query platform record")

		next(w, r.WithContext(ctx))
	}
}
