package system_recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// currentRestoreTarget 只接受 macOS 已验证的系统路径别名，不放开业务软链接。
// 先把所有目标归一为物理路径，再做重叠检查和原子替换，避免 /var 与
// /private/var 的两种写法被误拒绝或绕过同目录判断。
func currentRestoreTarget(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || absolute == string(filepath.Separator) {
		return "", fmt.Errorf("恢复文件目标无效：%s", path)
	}
	if runtime.GOOS == "darwin" {
		for _, alias := range []string{"/var", "/tmp", "/etc"} {
			if absolute != alias && !strings.HasPrefix(absolute, alias+string(filepath.Separator)) {
				continue
			}
			expected := "/private" + alias
			resolved, e := filepath.EvalSymlinks(alias)
			if e != nil || resolved != expected {
				return "", fmt.Errorf("macOS 系统路径别名无法验证，拒绝恢复：%s", path)
			}
			rel, _ := filepath.Rel(alias, absolute)
			absolute = filepath.Join(expected, rel)
			break
		}
	}
	if !canonicalNewPath(absolute) {
		return "", fmt.Errorf("恢复目标或父目录包含非系统软链接，拒绝覆盖：%s", path)
	}
	return absolute, nil
}

// existingCurrentBoundary 只规范已登记边界，不扩大允许恢复的目录范围。
func existingCurrentBoundary(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

// 不跟随替换阶段临时出现的业务链接。
func validatePreparedCurrentTarget(path string) error {
	if !filepath.IsAbs(path) || !canonicalNewPath(path) {
		return fmt.Errorf("恢复目标在预检查后发生路径变更，拒绝覆盖：%s", path)
	}
	if _, err := os.Lstat(filepath.Dir(path)); err != nil {
		return fmt.Errorf("恢复目标父目录不可读取：%s", path)
	}
	return nil
}
