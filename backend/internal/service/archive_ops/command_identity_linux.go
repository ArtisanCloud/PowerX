package archive_ops

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func commandBootIdentity() (string, string, error) {
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil {
		return "", "", e
	}
	if !validID(strings.TrimSpace(string(boot))) {
		return "", "", fmt.Errorf("boot identity unavailable")
	}
	pidns, e := os.Readlink("/proc/self/ns/pid")
	if e != nil {
		return "", "", e
	}
	mountns, e := os.Readlink("/proc/self/ns/mnt")
	if e != nil {
		return "", "", e
	}
	return dispositionHash(strings.TrimSpace(string(boot))), dispositionHash([]string{pidns, mountns}), nil
}
func commandProcessIdentity(pid int) (CommandProcessIdentity, bool, error) {
	var out CommandProcessIdentity
	dir := "/proc/" + strconv.Itoa(pid)
	file, e := os.Open(dir + "/stat")
	if errors.Is(e, os.ErrNotExist) {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	defer file.Close()
	info, e := os.Stat(dir)
	if e != nil {
		return out, false, e
	}
	raw := make([]byte, 8192)
	n, e := file.Read(raw)
	if e != nil {
		return out, false, e
	}
	text := string(raw[:n])
	end := strings.LastIndex(text, ") ")
	if end < 0 {
		return out, false, fmt.Errorf("process stat invalid")
	}
	fields := strings.Fields(text[end+2:])
	if len(fields) < 20 {
		return out, false, fmt.Errorf("process stat incomplete")
	}
	group, e := strconv.Atoi(fields[2])
	if e != nil {
		return out, false, e
	}
	start, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || start == 0 {
		return out, false, fmt.Errorf("process start invalid")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return out, false, fmt.Errorf("process owner unavailable")
	}
	return CommandProcessIdentity{PID: pid, PGID: group, UID: stat.Uid, Started: strconv.FormatUint(start, 10)}, true, nil
}
