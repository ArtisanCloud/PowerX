package service

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon"
)

type ClientProfileService struct {
	Service *Service
}

const PLATFORM_WECHAT_MINI_PROGRAM = "WeChat Mini Program"
const PLATFORM_IOS = "iOS"
const PLATFORM_ANDROID = "Android"
const PLATFORM_RETAIL = "Retail"
const PLATFORM_JD = "JD"
const PLATFORM_TMALL = "TMall"
const PLATFORM_DIANPING = "DianPing"
const PLATFORM_ALL = "All"
const PLATFORM_MGM = "MGM"
const PLATFORM_APP = "App"
const PLATFORM_WEBSITE = "Website"
const PLATFORM_TAIPEI_OLD_CYCLE_SYSTEM = "Taipei Old Cycle System"

const OS_TYPE_IOS = 1
const OS_TYPE_ANDROID = 2

const LOCALE_EN = "en_US"
const LOCALE_CN = "zh_CN"
const LOCALE_TW = "zh_TW"

const TIMEZONE = carbon.UCT
const REQUEST_TIMEZONE = carbon.Shanghai

var ARRAY_PLATFORM = []string{
	PLATFORM_WECHAT_MINI_PROGRAM,
	PLATFORM_IOS,
	PLATFORM_ANDROID,
	PLATFORM_RETAIL,
	PLATFORM_JD,
	PLATFORM_TMALL,
	PLATFORM_DIANPING,
	PLATFORM_ALL,
	PLATFORM_MGM,
	PLATFORM_APP,
	PLATFORM_WEBSITE,
	PLATFORM_TAIPEI_OLD_CYCLE_SYSTEM,
}

var ARRAY_OS_TYPE = []int{
	OS_TYPE_IOS,
	OS_TYPE_ANDROID,
}

var ARRAY_LOCALE = []string{
	LOCALE_EN,
	LOCALE_CN,
	LOCALE_TW,
}

var ARRAY_TIMEZONE = []string{
	LOCALE_EN,
	LOCALE_CN,
	LOCALE_TW,
}

/**
 ** 初始化构造函数
 */

// 模块初始化函数 import 包时被调用
func init() {
	//fmt.Println("clientProfile service module init function")
}

func NewClientProfileService(context *gin.Context) (r *ClientProfileService) {
	r = &ClientProfileService{
		Service: NewService(context),
	}
	return r
}

/**
 ** 静态函数
 */
func GetCurrentPlatform(c *gin.Context) string {
	return c.GetHeader("platform")

}

func GetCurrentUUID(c *gin.Context) string {

	return c.GetHeader("uuid")
}

func GetCurrentSource(c *gin.Context) string {

	return c.GetHeader("source")
}

func GetSessionLocale(c *gin.Context) string {

	// client cannot override the message locales.
	locale := GetRequestLocale(c)
	//        dd($locale);
	if locale == "" {
		locale = LOCALE_CN
	}
	return locale
}

func GetRequestLocale(c *gin.Context) string {

	requestLocal := c.GetHeader("locale")
	if requestLocal != "" && object.InArray(requestLocal, ARRAY_LOCALE) {
		return requestLocal
	}
	return ""
}

func SetAuthEmployee(ctx *gin.Context, employee *models.Employee) {
	ctx.Set("AuthEmployee", employee)
}

func GetAuthEmployee(ctx *gin.Context) (employee *models.Employee) {
	value, result := ctx.Get("AuthEmployee")
	if result {
		employee = value.(*models.Employee)
	}
	return employee
}

func SetAuthCustomer(ctx *gin.Context, account *models.Customer) {
	ctx.Set("AuthAccount", account)
}

func GetAuthCustomer(ctx *gin.Context) (account *models.Customer) {
	value, result := ctx.Get("AuthAccount")
	if result {
		account = value.(*models.Customer)
	}
	return account
}

/**
 ** 实例函数
 */
