package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"PowerX/internal/uc/powerx"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

type WebCustomerGetMiddleware struct {
	conf *config.Config
	px   *uc.PowerXUseCase
}

func NewWebCustomerGetMiddleware(conf *config.Config, px *uc.PowerXUseCase, opts ...optionFunc) *WebCustomerGetMiddleware {
	return &WebCustomerGetMiddleware{
		conf: conf,
		px:   px,
	}
}

func (m *WebCustomerGetMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		unAuth := errorx.ErrUnAuthorization.(*errorx.Error)

		vOpenId := r.Context().Value(customerdomain.AuthCustomerOpenIdKey)
		if vOpenId == nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效授权客户OpenId"))
			return
		}
		openId := vOpenId.(string)
		if openId == "" {
			httpx.Error(w, errorx.WithCause(unAuth, "授权客户OpenId为空"))
			return
		}

		// 小程序的客户记录是否存在
		authOACustomer, err := m.px.WechatOA.FindOneOACustomer(r.Context(), &powerx.FindOACustomerOption{
			OpenIds: []string{openId},
		})
		if err != nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效微信小程序客户"))
			return
		}

		// 小程序的客户记录是否存在
		if authOACustomer.Customer == nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效客户记录"))
			return
		}

		ctx := context.WithValue(r.Context(), customerdomain.AuthCustomerKey, authOACustomer.Customer)

		next(w, r.WithContext(ctx))
	}
}
