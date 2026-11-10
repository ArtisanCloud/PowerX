package archive_ops

import "golang.org/x/sys/unix"

// WNOWAIT 只观察退出，保留 zombie leader 占用 PID，直到组信号完成后由 Cmd.Wait 回收。
func observeCommandExit(pid int) <-chan error {
	done := make(chan error, 1)
	go func() {
		var info unix.Siginfo
		for {
			e := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
			if e == unix.EINTR {
				continue
			}
			done <- e
			return
		}
	}()
	return done
}
