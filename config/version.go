package config

import (
	"github.com/ArtisanCloud/PowerX/config/app"
)

var APP_NAME string

const APP_VERSION = "{{version}}"

func LoadVersion() {
	APP_NAME = app.G_AppConfigure.Name + "-" + app.G_AppConfigure.Env
}
