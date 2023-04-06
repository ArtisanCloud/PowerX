package middleware

import (
	"PowerX/internal/config"
	"PowerX/internal/uc"
	"github.com/gorilla/mux"
	"net/http"
)

type EmployeeNoPermJWTAuthMiddleware struct {
	conf *config.Config
	px   *uc.PowerXUseCase
	opt  option
}

func NewEmployeeNoPermJWTAuthMiddleware(conf *config.Config, px *uc.PowerXUseCase, opts ...optionFunc) *EmployeeJWTAuthMiddleware {
	opt := option{}
	for _, o := range opts {
		o(&opt)
	}

	return &EmployeeJWTAuthMiddleware{
		conf: conf,
		px:   px,
		opt:  opt,
	}
}

func (m *EmployeeNoPermJWTAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	//secret := m.conf.JWTSecret
	//unAuth := errorx.ErrUnAuthorization.(*errorx.Error)
	//unKnow := errorx.ErrUnKnow.(*errorx.Error)

	publicRouter := mux.NewRouter()
	for _, s := range m.opt.public {
		publicRouter.NewRoute().PathPrefix(s)
	}

	whiteRouter := mux.NewRouter()
	for _, s := range m.opt.whiteList {
		whiteRouter.NewRoute().PathPrefix(s)
	}

	return func(writer http.ResponseWriter, request *http.Request) {
		//// public
		//var match mux.RouteMatch
		//if publicRouter.Match(request, &match) {
		//	next(writer, request)
		//	return
		//}
		//
		//// 校验Token
		//if m.opt.disableAuth {
		//	next(writer, request)
		//	return
		//}
		//
		//authorization := request.Header.Get("Authorization")
		//splits := strings.Split(authorization, "Bearer")
		//if len(splits) != 2 {
		//	httpx.Error(writer, unAuth)
		//	return
		//}
		//tokenString := strings.TrimSpace(splits[1])
		//
		//var claims types.TokenClaims
		//token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		//	return []byte(secret), nil
		//})
		//if err != nil || !token.Valid {
		//	if errors.Is(err, jwt.ErrTokenMalformed) {
		//		httpx.Error(writer, unAuth)
		//	} else if errors.Is(err, jwt.ErrTokenExpired) || errors.Is(err, jwt.ErrTokenNotValidYet) {
		//		httpx.Error(writer, unAuth)
		//	} else {
		//		logx.WithContext(request.Context()).Error(err)
		//		httpx.Error(writer, errorx.WithCause(unAuth, "违规Token"))
		//	}
		//	return
		//}

		// todo metadata ctx

		next(writer, request)
	}
}
