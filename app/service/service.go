package service

import (
	"github.com/gin-gonic/gin"
)

const MODULE_CUSTOMER = 1
const MODULE_EMPLOYEE = 2
const MODULE_CONTACT_WAY = 3
const MODULE_TAG = 4
const MODULE_PRODUCT = 5
const MODULE_ORDER = 6

type Service struct {
	Context *gin.Context
}

/**
 ** 初始化构造函数
 */

// 模块初始化函数 import 包时被调用
func init() {
	//fmt.Println("reservation service module init function")
}

func NewService(ctx *gin.Context) (r *Service) {
	r = &Service{
		Context: ctx,
	}
	return r
}
