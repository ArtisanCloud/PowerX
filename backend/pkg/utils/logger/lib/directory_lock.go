package lumberjack

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
)

// WithDirectoryLockFD 让原生日志轮转和受控手动回收共用同一个目录锁。
// 调用方保有目录描述符；锁附着目录 inode，不生成可被替换的锁文件。
func WithDirectoryLockFD(ctx context.Context, dir int, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var st unix.Stat_t
	if unix.Fstat(dir, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("unsafe log retention lock")
	}
	if err := unix.Flock(dir, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return errors.New("log retention directory busy")
	}
	defer unix.Flock(dir, unix.LOCK_UN)
	return fn()
}

func withDirectoryLock(path string, fn func() error) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return WithDirectoryLockFD(context.Background(), fd, fn)
}
