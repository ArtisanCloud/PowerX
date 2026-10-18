package system_recovery

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

// 内核启动时间不随 Redis/PostgreSQL 修改进程标题变化，避免误判归属。
func processIdentity(pid int) (string, error) {
	info, e := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if e != nil {
		return "", e
	}
	if info.Proc.P_pid != int32(pid) || info.Proc.P_stat == 5 {
		return "", os.ErrNotExist
	}
	return fmt.Sprintf("%d:%d:%d", pid, info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec), nil
}
