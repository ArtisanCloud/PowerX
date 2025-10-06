package env

import "strings"

const (
	Dev     = "dev"
	Staging = "staging"
	Prod    = "prod"

	// 扩展
	Canary = "canary"
	Blue   = "blue"
	Green  = "green"

	// 别名（输入兼容）
	Default    = "default"    // → dev
	Production = "production" // → prod
	StageAlias = "stage"      // → staging
	StgAlias   = "stg"        // → staging

	PreviewPrefix = "preview-"
)

var Canonical = []string{Dev, Staging, Prod, Canary, Blue, Green}

func Normalize(in string) string {
	e := strings.ToLower(strings.TrimSpace(in))
	switch e {
	case "", Default:
		return Dev
	case Production:
		return Prod
	case StageAlias, StgAlias:
		return Staging
	default:
		if strings.HasPrefix(e, PreviewPrefix) {
			return e
		}
		return e
	}
}

func Canonicalize(in string) string {
	e := Normalize(in)
	if strings.HasPrefix(e, PreviewPrefix) {
		return e
	}
	for _, c := range Canonical {
		if e == c {
			return e
		}
	}
	return Dev
}

func IsValid(in string) bool {
	e := Normalize(in)
	if strings.HasPrefix(e, PreviewPrefix) {
		return true
	}
	for _, c := range Canonical {
		if e == c {
			return true
		}
	}
	return false
}

func IsProdLike(in string) bool {
	switch Normalize(in) {
	case Prod, Canary, Blue, Green:
		return true
	default:
		return false
	}
}
