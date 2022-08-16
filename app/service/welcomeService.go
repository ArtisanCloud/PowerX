package service

import (
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/configs"
)

type WelcomeService struct {
	Customer *models.Customer
}

/**
 ** 初始化构造函数
 */

// 模块初始化函数 import 包时被调用
func init() {
	//fmt.Println("reservation service module init function")
}

func NewWelcomeService() (r *WelcomeService) {
	r = &WelcomeService{
		Customer: models.NewCustomer(nil),
	}
	return r
}

/**
 ** 实例函数
 */

func (srv *WelcomeService) GetWelcome() string {

	return "Welcome! " + config.APP_NAME + " version:" + config.APP_VERSION

}

func (srv *WelcomeService) GetWelcomeAPI() string {

	return "Welcome API!"

}
