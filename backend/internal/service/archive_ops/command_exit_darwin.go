package archive_ops

import "golang.org/x/sys/unix"

// NOTE_EXIT 不回收直接子进程；若注册前已退出，ESRCH 同样不代表已经 Wait/reap。
func observeCommandExit(pid int) <-chan error {
	done := make(chan error, 1)
	go func() {
		fd, e := unix.Kqueue()
		if e != nil {
			done <- e
			return
		}
		defer unix.Close(fd)
		change := unix.Kevent_t{Ident: uint64(pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT}
		events := make([]unix.Kevent_t, 1)
		for {
			n, err := unix.Kevent(fd, []unix.Kevent_t{change}, events, nil)
			if err == unix.EINTR {
				continue
			}
			if err == unix.ESRCH {
				done <- nil
				return
			}
			if err != nil {
				done <- err
				return
			}
			if n > 0 {
				if events[0].Flags&unix.EV_ERROR != 0 && events[0].Data != 0 && events[0].Data != int64(unix.ESRCH) {
					done <- unix.Errno(events[0].Data)
				} else {
					done <- nil
				}
				return
			}
		}
	}()
	return done
}
