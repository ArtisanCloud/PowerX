package backup_ops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
)

// 只允许访问稳定根目录内登记的普通 dump 文件；任何路径组件为符号链接均拒绝。
func validatedArtifactPath(root string, artifact *modelops.BackupArtifact) (string, error) {
	if artifact == nil || !filepath.IsAbs(root) {
		return "", fmt.Errorf("invalid artifact root or metadata")
	}
	path, err := parseFileStorageURI(artifact.StorageURI)
	if err != nil || !filepath.IsAbs(path) || filepath.Ext(path) != ".dump" {
		return "", fmt.Errorf("invalid backup artifact path")
	}
	root, path = filepath.Clean(root), filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("backup artifact is outside configured root")
	}
	// 从 / 开始逐层 lstat；阻止中间目录链接到另一个环境。
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(path, current), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, e := os.Lstat(current)
		if e != nil {
			return path, e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink backup path is not allowed")
		}
	}
	return path, nil
}

func verifyArtifactFile(path string, artifact *modelops.BackupArtifact) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() != artifact.SizeBytes {
		return fmt.Errorf("backup artifact size or file type mismatch")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if artifact.Checksum == "" || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), artifact.Checksum) {
		return fmt.Errorf("backup artifact SHA256 mismatch")
	}
	return nil
}
