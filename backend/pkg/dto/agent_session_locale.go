package dto

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed locales/agent_session.json
var agentSessionLocaleJSON []byte

var agentSessionLocales = func() map[string]map[string]string {
	var result map[string]map[string]string
	if err := json.Unmarshal(agentSessionLocaleJSON, &result); err != nil {
		panic(err)
	}
	return result
}()

func AgentSessionErrorMessage(language, code string) string {
	locale := "zh-CN"
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "en") {
		locale = "en-US"
	}
	return agentSessionLocales[locale][code]
}
