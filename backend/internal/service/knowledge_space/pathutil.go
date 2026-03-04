package knowledge_space

import (
	"os"
	"path/filepath"
	"strings"
)

func isTestBinary() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func findRepoRoot(start string) string {
	dir := filepath.Clean(start)
	for {
		if dir == "" || dir == string(filepath.Separator) || dir == "." {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, ".specify")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			return ""
		}
		dir = next
	}
}

func projectTmpDir() string {
	if env := strings.TrimSpace(os.Getenv("POWERX_TMP_DIR")); env != "" {
		return env
	}
	wd, err := os.Getwd()
	if err != nil {
		return "tmp"
	}
	root := findRepoRoot(wd)
	if root == "" {
		return filepath.Join(wd, "tmp")
	}
	return filepath.Join(root, "tmp")
}
