package api

import (
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/app/http"
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"runtime"
	"runtime/debug"
)

type APIController struct {
	ServiceUser *service.EmployeeService
	Context     *gin.Context
	RS          *http.APIResponse
}

func NewAPIController(context *gin.Context) *APIController {
	return &APIController{
		ServiceUser: service.NewEmployeeService(context),
		Context:     context,
		RS:          http.NewAPIResponse(context),
	}
}

func RecoverResponse(context *gin.Context, action string) {

	if p := recover(); p != nil {
		var err error
		apiResponse := &http.APIResponse{}
		apiResponse.Context = context
		switch rs := p.(type) {

		// 获取业务流程中的异常错误
		case *http.APIResponse:
			rs.ThrowJSONResponse(context)
			break

		case runtime.Error:
			err = p.(runtime.Error)
		case string:
			err = errors.New(p.(string))
		// 若非APIResponse，也许默认抛出一个若非APIResponse
		default:
		}

		if err != nil {
			fmt.Printf("Unknown panic: %v \r\n", err.Error())
			fmt.Printf("err stack: %v \r\n", string(debug.Stack()))

			apiResponse.SetReturnCode(config.API_RETURN_CODE_ERROR, "Inner Error")
			apiResponse.ThrowJSONResponse(context)
		}

	}
}
