package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc"
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
	secret := m.conf.JWTSecret
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

		// Passthrough to next handler if need
		next(writer, request)
	}
}
