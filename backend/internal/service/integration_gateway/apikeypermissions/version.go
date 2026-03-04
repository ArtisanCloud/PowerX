package apikeypermissions

import "strings"

var introducedVersion = "v1.0.0"

func SetIntroducedVersion(version string) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		introducedVersion = "v1.0.0"
		return
	}
	introducedVersion = trimmed
}

func IntroducedVersion() string {
	return introducedVersion
}

