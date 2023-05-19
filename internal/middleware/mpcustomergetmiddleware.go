package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/model"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
)

type MPCustomerGetMiddleware struct {
	conf *config.Config
	px   *uc.PowerXUseCase
}

func NewMPCustomerGetMiddleware(conf *config.Config, px *uc.PowerXUseCase, opts ...optionFunc) *MPCustomerGetMiddleware {
	return &MPCustomerGetMiddleware{
		conf: conf,
		px:   px,
	}
}

func (m *MPCustomerGetMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
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

		authCustomer, err := m.px.WechatMP.FindOneMPCustomer(r.Context(), &model.FindMPCustomerOption{
			OpenIds: []string{openId},
		})
		if err != nil {
			httpx.Error(w, errorx.WithCause(unAuth, "无效微信小程序客户"))
			return
		}

		ctx := context.WithValue(r.Context(), customerdomain.AuthCustomerKey, authCustomer)

		next(w, r.WithContext(ctx))
	}
}
