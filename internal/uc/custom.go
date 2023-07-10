package uc

import (
	"PowerX/internal/config"
	fmt "PowerX/pkg/printx"
	"github.com/golang-module/carbon/v2"
	"gorm.io/gorm"
	"time"
)

type CustomUseCase struct {
	db *gorm.DB
}

func NewCustomUseCase(conf *config.Config) (uc *CustomUseCase, clean func()) {

	uc = &CustomUseCase{}

	// 需要打印当时系统的Timezone
	uc.CheckSystemTimeZone()
	return uc, func() {

	}
}

func (uc *CustomUseCase) CheckSystemTimeZone() {

	// 设置 Golang 的 time 包的默认时区
	cst := time.FixedZone("CST", 8*60*60)
	time.Local = cst

	// 设置 Carbon 库的默认时区
	strTimezone := "Asia/Shanghai"
	carbon.SetTimezone(strTimezone)

	// carbon 的timezone
	carbonTimezone := carbon.Now().Timezone()
	fmt.Dump("carbon timezone is :", carbonTimezone)

	// 输出系统默认时区
	defaultTimezone := time.Now().Location()
	fmt.Dump("System default timezone is:", defaultTimezone.String())

}
