package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"PowerX/internal/uc/powerx/crm/customerdomain"
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

		vCustomerId := r.Context().Value(customerdomain.AuthCustomerCustomerId)
		if vCustomerId == nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效授权客户Id"))
			return
		}
		customerId := vCustomerId.(int64)
		if customerId <= 0 {
			httpx.Error(w, errorx.WithCause(unAuth, "授权客户Id为空"))
			return
		}

		authCustomer, err := m.px.Customer.GetCustomer(r.Context(), customerId)
		if err != nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效客户"))
			return
		}

		//// 小程序的客户记录是否存在
		//authOACustomer, err := m.px.WechatOA.FindOneOACustomer(r.Context(), &powerx.FindOACustomerOption{
		//	Ids: []int64{customerId},
		//})
		//if err != nil {
		//	httpx.Error(w, errorx.WithCause(unAuth, "无效微信公众号客户"))
		//	return
		//}

		//// 小程序的客户记录是否存在
		//if authOACustomer.Customer == nil {
		//	httpx.Error(w, errorx.WithCause(unAuth, "无效客户记录"))
		//	return
		//}

		ctx := context.WithValue(r.Context(), customerdomain.AuthCustomerKey, authCustomer)

		next(w, r.WithContext(ctx))
	}
}
