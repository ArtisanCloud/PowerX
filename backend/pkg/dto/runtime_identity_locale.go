package dto

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed locales/runtime_identity.json
var runtimeIdentityLocaleJSON []byte
var runtimeIdentityLocales = func() map[string]map[string]string {
	var result map[string]map[string]string
	if err := json.Unmarshal(runtimeIdentityLocaleJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

func RuntimeIdentityErrorMessage(language, code string) string {
	locale := "zh-CN"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		locale = "en-US"
	}
	return runtimeIdentityLocales[locale][code]
}
