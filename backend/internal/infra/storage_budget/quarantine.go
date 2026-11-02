package storagebudget

import (
	"context"
	"golang.org/x/sys/unix"
	"os"
)

// WithQuarantine 仅提供私有同盘隔离区的描述符，不允许 HTTP 指定路径。
// 调用方持有主机执行锁；目录内文件必须有持久执行意图，不能被通用 GC 删除。
func (c *Coordinator) WithQuarantine(ctx context.Context, fn func(int) error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	root, err := c.openDirectory()
	if err != nil {
		return err
	}
	defer unix.Close(root)
	if err = unix.Mkdirat(root, "quarantine", 0700); err != nil && err != unix.EEXIST {
		return ErrUnavailable
	}
	fd, err := unix.Openat(root, "quarantine", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return ErrUnavailable
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() || uint64(st.Dev) != c.device {
		return ErrUnavailable
	}
	return fn(fd)
}
func (c *Coordinator) ContainsPath(path string) bool { return c.sameDevice(path) }
