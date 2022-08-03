package web

import (
	"github.com/ArtisanCloud/PowerX/app/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

type WelcomeController struct {
	ServiceWelcome *service.WelcomeService
}

// 模块初始化函数 import 包时被调用
func init() {
	//fmt.Println("Init reservation Controller")
}

func NewWelcomeController() (ctl *WelcomeController) {

	return &WelcomeController{
		ServiceWelcome: service.NewWelcomeService(),
	}
}

func WebGetHome(context *gin.Context) {
	ctl := NewWelcomeController()

	r := ctl.ServiceWelcome.GetWelcome()

	context.JSON(http.StatusOK, r)
}
