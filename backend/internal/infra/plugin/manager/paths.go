package manager

import "path/filepath"

// ResolvePath 将插件内的相对路径解析为绝对落地路径；为空则返回空
func ResolvePath(root, rel string) string {
	if rel == "" {
		return ""
	}
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	if root == "" {
		return filepath.Clean(rel)
	}
	return filepath.Clean(filepath.Join(root, rel))
}
