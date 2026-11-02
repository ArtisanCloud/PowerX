package storagebudget

import (
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
)

func (c *Coordinator) openDirectory() (int, error) {
	fd, err := unix.Open(c.cfg.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrUnavailable
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&0077 != 0 || int(st.Uid) != os.Geteuid() || uint64(st.Dev) != c.device || st.Ino != c.inode {
		unix.Close(fd)
		return -1, ErrUnavailable
	}
	return fd, nil
}
func (c *Coordinator) readPrivate(name string, limit int64) ([]byte, error) {
	if filepath.Base(name) != name {
		return nil, ErrUnavailable
	}
	dir, err := c.openDirectory()
	if err != nil {
		return nil, err
	}
	defer unix.Close(dir)
	fd, err := unix.Openat(dir, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if err == unix.ENOENT {
			return nil, os.ErrNotExist
		}
		return nil, ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "private-storage-metadata")
	defer file.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&0077 != 0 || st.Nlink != 1 || int(st.Uid) != os.Geteuid() || st.Size > limit {
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrUnavailable
	}
	return raw, nil
}
func (c *Coordinator) writePrivate(name string, raw []byte) error {
	if filepath.Base(name) != name {
		return ErrUnavailable
	}
	dir, err := c.openDirectory()
	if err != nil {
		return err
	}
	defer unix.Close(dir)
	temp := ".metadata-" + uuid.NewString()
	fd, err := unix.Openat(dir, temp, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return ErrUnavailable
	}
	defer unix.Unlinkat(dir, temp, 0)
	file := os.NewFile(uintptr(fd), "private-storage-metadata")
	_, err = file.Write(raw)
	if err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil || closed != nil {
		return ErrUnavailable
	}
	if unix.Renameat(dir, temp, dir, name) != nil || unix.Fsync(dir) != nil {
		return ErrUnavailable
	}
	return nil
}
