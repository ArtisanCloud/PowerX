package archive_ops

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func commandBootIdentity() (string, string, error) {
	boot, e := unix.SysctlTimeval("kern.boottime")
	if e != nil {
		return "", "", e
	}
	return dispositionHash([]any{"darwin-boot", boot.Sec, boot.Usec}), dispositionHash("darwin-host-processes"), nil
}
func commandProcessIdentity(pid int) (CommandProcessIdentity, bool, error) {
	rows, e := unix.SysctlKinfoProcSlice("kern.proc.pid", pid)
	if e != nil {
		return CommandProcessIdentity{}, false, e
	}
	if len(rows) == 0 {
		return CommandProcessIdentity{}, false, nil
	}
	if len(rows) != 1 || int(rows[0].Proc.P_pid) != pid {
		return CommandProcessIdentity{}, false, fmt.Errorf("process identity ambiguous")
	}
	p := rows[0]
	return CommandProcessIdentity{PID: pid, PGID: int(p.Eproc.Pgid), UID: p.Eproc.Ucred.Uid, Started: fmt.Sprintf("%d:%d", p.Proc.P_starttime.Sec, p.Proc.P_starttime.Usec)}, true, nil
}
