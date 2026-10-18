package archive_ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrSourceChanged 表示恢复点引用的来源发生变化；上层仅重试备份，不阻止业务。
var ErrSourceChanged = errors.New("采集来源发生变化")

// openReferencedFile 使用逐级目录描述符，不允许软链接或路径竞争越过媒体根。
// 返回的文件描述符固定 inode，后续 unlink/原子替换不会改变正在读取的内容。
func openReferencedFile(root, relative string) (*os.File, error) {
	if !cleanEntryPath(relative) {
		return nil, errors.New("媒体引用路径无效")
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(relative, "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), relative), nil
}
func (s *Service) captureReferencedFile(ctx context.Context, root, relative string, size int64, checksum string) (Entry, int64, error) {
	f, err := openReferencedFile(root, relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Entry{}, 0, fmt.Errorf("%w: 媒体已删除或替换 %s", ErrSourceChanged, relative)
		}
		return Entry{}, 0, fmt.Errorf("媒体无法安全读取: %s", relative)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return Entry{}, 0, errors.New("媒体引用必须是普通文件")
	}
	if before.Size() != size {
		return Entry{}, 0, fmt.Errorf("%w: 媒体大小变化 %s", ErrSourceChanged, relative)
	}
	entry, added, err := s.captureReader(ctx, f, "media/"+relative, size, before.ModTime())
	if err != nil {
		if ctx.Err() != nil {
			return entry, added, ctx.Err()
		}
		after, statErr := f.Stat()
		if statErr == nil && (after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime())) {
			return entry, added, fmt.Errorf("%w: 媒体在读取中变化 %s", ErrSourceChanged, relative)
		}
		return entry, added, err
	}
	after, err := f.Stat()
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || entry.SHA256 != checksum {
		return entry, added, fmt.Errorf("%w: 媒体内容与数据库恢复点不符 %s", ErrSourceChanged, relative)
	}
	return entry, added, nil
}
