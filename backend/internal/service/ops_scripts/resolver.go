package ops_scripts

import (
	"os"
	"path/filepath"
	"strings"
)

const EnvScriptDir = "POWERX_OPS_SCRIPT_DIR"

func ResolveDir(requiredScript string) string {
	if configured := strings.TrimSpace(os.Getenv(EnvScriptDir)); configured != "" {
		return absolutize(configured)
	}
	candidates := []string{
		filepath.Join("scripts", "ops"),
		filepath.Join("backend", "scripts", "ops"),
	}
	for i := range candidates {
		if requiredScript == "" || pathExists(filepath.Join(candidates[i], requiredScript)) {
			return absolutize(candidates[i])
		}
	}
	if root := detectProjectRoot(); root != "" {
		return filepath.Join(root, "backend", "scripts", "ops")
	}
	return absolutize(filepath.Join("backend", "scripts", "ops"))
}

func DetectProjectRoot() string {
	return detectProjectRoot()
}

func detectProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := filepath.Clean(wd)
	for {
		if pathExists(filepath.Join(dir, "backend", "etc", "config.yaml")) ||
			pathExists(filepath.Join(dir, "etc", "config.yaml")) {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir || next == "." || next == string(filepath.Separator) {
			break
		}
		dir = next
	}
	return ""
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func absolutize(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || clean == "" || filepath.IsAbs(clean) {
		return clean
	}
	if abs, err := filepath.Abs(clean); err == nil {
		return abs
	}
	return clean
}
