package lang

import (
	"github.com/ArtisanCloud/PowerX/resources/lang/en_US"
	"github.com/ArtisanCloud/PowerX/resources/lang/zh_CN"
	"github.com/ArtisanCloud/PowerX/resources/lang/zh_TW"
)

func LoadLanguages() {
	en_US.LoadLang()
	zh_CN.LoadLang()
	en_TW.LoadLang()
}
