package storagebudget

import "golang.org/x/sys/unix"

func sameDevice(path string, device uint64) bool {
	var st unix.Stat_t
	return unix.Stat(path, &st) == nil && uint64(st.Dev) == device
}
