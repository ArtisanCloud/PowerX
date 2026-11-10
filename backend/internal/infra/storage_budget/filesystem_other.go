//go:build !darwin

package storagebudget

// 未提供可靠平台证明时继续阻止设备变更，不能仅凭配置名称自动换绑。
func stableFilesystemMatches(string, string) bool { return false }
