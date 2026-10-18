package system_recovery

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func processIdentity(pid int) (string, error) {
	raw, e := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if e != nil {
		return "", e
	}
	// comm 可含空格和右括号；其后第 20 个字段是内核 starttime。
	tail := strings.LastIndexByte(string(raw), ')')
	if tail < 0 {
		return "", errors.New("invalid process stat")
	}
	fields := strings.Fields(string(raw[tail+1:]))
	if len(fields) < 20 || fields[0] == "Z" {
		return "", os.ErrNotExist
	}
	return strconv.Itoa(pid) + ":" + fields[19], nil
}
