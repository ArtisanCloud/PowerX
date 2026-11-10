package model

import "strings"

// CanonicalAIEnvironment 与管理页一致：default 是开发环境的历史别名。
// 不修改通用 WithScope，也不把其他环境或租户纳入查询。
func CanonicalAIEnvironment(env string) string {
	env = strings.TrimSpace(env)
	if env == "default" {
		return "dev"
	}
	return env
}

// AIEnvironmentReadOrder 优先完整的 dev 记录，缺失时才读取同一作用域的旧记录。
func AIEnvironmentReadOrder(env string) []string {
	canonical := CanonicalAIEnvironment(env)
	if canonical == "dev" {
		return []string{"dev", "default"}
	}
	return []string{canonical}
}

// AICredentialNameForEnvironment 仅转换自动生成的环境前缀，保留自定义名称。
func AICredentialNameForEnvironment(name, env, providerSlug string) string {
	if env != "dev" && env != "default" {
		return name
	}
	for _, source := range []string{"dev", "default"} {
		if name == source+"-"+providerSlug {
			return env + "-" + providerSlug
		}
	}
	return name
}
