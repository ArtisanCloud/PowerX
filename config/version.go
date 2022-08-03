package config

var APP_NAME string

const APP_VERSION = "{{version}}"

func LoadVersion() {
	APP_NAME = AppConfigure.Name + "-" + AppConfigure.Env
}
