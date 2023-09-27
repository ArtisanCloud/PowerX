package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"context"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest/httpx"
	"net/http"
	"strings"
)

type MPCustomerJWTAuthMiddleware struct {
	conf *config.Config
	px   *uc.PowerXUseCase
}

func NewMPCustomerJWTAuthMiddleware(conf *config.Config, px *uc.PowerXUseCase, opts ...optionFunc) *MPCustomerJWTAuthMiddleware {
	return &MPCustomerJWTAuthMiddleware{
		conf: conf,
		px:   px,
	}
}

func (m *MPCustomerJWTAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	secret := m.conf.JWT.MPJWTSecret
	unAuth := errorx.ErrUnAuthorization.(*errorx.Error)

	return func(writer http.ResponseWriter, request *http.Request) {

		authorization := request.Header.Get("Authorization")
		splits := strings.Split(authorization, "Bearer")
		if len(splits) != 2 {
			httpx.Error(writer, unAuth)
			return
		}
		tokenString := strings.TrimSpace(splits[1])

		var claims types.TokenClaims
		token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			if errors.Is(err, jwt.ErrTokenMalformed) {
				httpx.Error(writer, unAuth)
			} else if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
				httpx.Error(writer, unAuth)
			} else {
				logx.WithContext(request.Context()).Error(err)
				httpx.Error(writer, errorx.WithCause(unAuth, "违规Token"))
			}
			return
		}

		// 获取小程序授权的openid
		payload, err := customerdomain.GetPayloadFromToken(token.Raw)
		if err != nil {
			logx.WithContext(request.Context()).Error(err)
			httpx.Error(writer, errorx.WithCause(unAuth, "无效客户信息"))
			return
		}
		openId := payload[customerdomain.AuthCustomerOpenIdKey]
		ctx := context.WithValue(request.Context(), customerdomain.AuthCustomerOpenIdKey, openId)

		// Pass through to next handler if need
		next(writer, request.WithContext(ctx))
	}
}
