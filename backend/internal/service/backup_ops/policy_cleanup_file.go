package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
)

type backupChecksumReader struct {
	ctx  context.Context
	file *os.File
}

func (r backupChecksumReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Read(p)
}

// 完整读取放在 SQL 守卫外，避免大产物校验长期占用配置行锁；仍持有既有作业锁。
func verifyControlledArtifact(ctx context.Context, root string, artifact *model.BackupArtifact) (os.FileInfo, error) {
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := run.Err(); err != nil {
		return nil, err
	}
	path, err := validatedArtifactPath(root, artifact)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() != artifact.SizeBytes {
		return nil, fmt.Errorf("backup artifact size or file type mismatch")
	}
	hash := sha256.New()
	if _, err = io.CopyBuffer(hash, backupChecksumReader{run, file}, make([]byte, 64<<10)); err != nil {
		return nil, err
	}
	if artifact.Checksum == "" || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), artifact.Checksum) {
		return nil, fmt.Errorf("backup artifact SHA256 mismatch")
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !sameControlledArtifact(before, after) {
		return nil, fmt.Errorf("backup artifact changed during verification")
	}
	return after, nil
}
func sameControlledArtifact(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// 真正删除前只做有界身份复核；发现替换、变化或此前不存在的新文件均停止。
func controlledArtifactRemovalPath(root string, artifact *model.BackupArtifact, verified os.FileInfo) (string, error) {
	path, err := validatedArtifactPath(root, artifact)
	if errors.Is(err, os.ErrNotExist) && verified == nil {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !sameControlledArtifact(verified, current) {
		return "", fmt.Errorf("backup artifact changed after verification")
	}
	return path, nil
}
