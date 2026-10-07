package dto

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed locales/service_credential.json
var serviceCredentialLocaleJSON []byte

var serviceCredentialLocales = func() map[string]map[string]string {
	var result map[string]map[string]string
	if err := json.Unmarshal(serviceCredentialLocaleJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

func ServiceCredentialErrorMessage(language string, status int) string {
	locale := "zh-CN"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		locale = "en-US"
	}
	return serviceCredentialLocales[locale][strconv.Itoa(status)]
}
