// Package storageobserver 对登记目录做有界、只读计量，不读取文件内容或执行命令。
package storageobserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"golang.org/x/sys/unix"
)

type Collector struct {
	MaxEntries int
	Timeout    time.Duration
}
type fileKey struct{ device, inode uint64 }

var errSourceSymlink = errors.New("registered source contains a symlink")

func sourceProblem(err error) string {
	switch {
	case errors.Is(err, errSourceSymlink):
		return "source_symlink"
	case errors.Is(err, unix.ENOENT):
		return "source_missing"
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return "source_permission_denied"
	default:
		return "source_unreadable"
	}
}

func pointer(v int64) *int64 { return &v }
func metrics(st unix.Statfs_t) storagegov.Filesystem {
	out := storagegov.Filesystem{View: "process_filesystem", State: "unavailable"}
	if st.Bsize <= 0 {
		out.Problem = "filesystem_metrics_invalid"
		return out
	}
	size := uint64(st.Bsize)
	if uint64(st.Blocks) > uint64(math.MaxInt64)/size || uint64(st.Bfree) > uint64(st.Blocks) || uint64(st.Bavail) > uint64(st.Bfree) {
		out.Problem = "filesystem_metrics_invalid"
		return out
	}
	total, free, available := int64(uint64(st.Blocks)*size), int64(uint64(st.Bfree)*size), int64(uint64(st.Bavail)*size)
	out.ID = fmt.Sprintf("kernel-fsid:%08x:%08x", uint32(st.Fsid.Val[0]), uint32(st.Fsid.Val[1]))
	out.State = "observed"
	out.TotalBytes, out.UsedBytes, out.AvailableBytes, out.ReservedBytes = pointer(total), pointer(total-free), pointer(available), pointer(free-available)
	return out
}
func filesystem(path string) storagegov.Filesystem {
	var st unix.Statfs_t
	if unix.Statfs(path, &st) != nil {
		return storagegov.Filesystem{View: "process_filesystem", State: "unavailable", Problem: "filesystem_unreadable"}
	}
	return metrics(st)
}

// openSource 逐级使用目录描述符和 O_NOFOLLOW，父目录被替换后也不跟随软链。
func openSource(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		if e != nil {
			var st unix.Stat_t
			if unix.Fstatat(fd, part, &st, unix.AT_SYMLINK_NOFOLLOW) == nil && st.Mode&unix.S_IFMT == unix.S_IFLNK {
				e = fmt.Errorf("%w: %w", errSourceSymlink, e)
			}
		}
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}

// Collect 共用条目／时长上限及唯一 inode 集合；不扫描未登记目录或跨挂载点。
func (c Collector) Collect(ctx context.Context, root string, resources []storagegov.Resource) storagegov.Snapshot {
	limit, timeout := c.MaxEntries, c.Timeout
	if limit <= 0 {
		limit = 20000
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	run, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out := storagegov.Snapshot{Filesystem: filesystem(root), Resources: []storagegov.Usage{}}
	seen := map[fileKey]bool{}
	entries := 0
	for i, resource := range resources {
		u := storagegov.Usage{Resource: resource, State: "unavailable"}
		if run.Err() != nil || entries >= limit {
			u.State = "partial"
			u.Problem = "scan_limit_reached"
			out.Resources = append(out.Resources, u)
			continue
		}
		fd, err := openSource(resource.Path)
		if err != nil {
			u.Problem = sourceProblem(err)
			out.Resources = append(out.Resources, u)
			continue
		}
		var fsStat unix.Statfs_t
		var rootStat unix.Stat_t
		if unix.Fstatfs(fd, &fsStat) != nil || unix.Fstat(fd, &rootStat) != nil {
			unix.Close(fd)
			u.Problem = "source_unreadable"
			out.Resources = append(out.Resources, u)
			continue
		}
		fsInfo := metrics(fsStat)
		u.FilesystemID = fsInfo.ID
		if fsInfo.State != "observed" {
			unix.Close(fd)
			u.Problem = fsInfo.Problem
			out.Resources = append(out.Resources, u)
			continue
		}
		complete := true
		// 小来源未用完的额度留给后面的来源，避免最后的快照目录也只能使用固定 1/N 配额。
		remainingSources := len(resources) - i
		sourceLimit := entries + max(1, (limit-entries)/remainingSources)
		deadline, _ := run.Deadline()
		sourceDeadline := time.Now().Add(time.Until(deadline) / time.Duration(remainingSources))
		local := map[fileKey]bool{}
		var physical, logical, attributed int64
		bad := func(reason string) { complete = false; u.Skipped++; u.Problem = reason }
		limited := func() bool {
			if run.Err() != nil || entries >= limit || entries >= sourceLimit || time.Now().After(sourceDeadline) {
				complete = false
				u.Problem = "scan_limit_reached"
				return true
			}
			return false
		}
		visit := func(st unix.Stat_t) {
			entries++
			kind := st.Mode & unix.S_IFMT
			if kind != unix.S_IFREG && kind != unix.S_IFDIR {
				bad("special_files_or_symlinks_skipped")
				return
			}
			if uint64(st.Dev) != uint64(rootStat.Dev) {
				bad("nested_mount_skipped")
				return
			}
			if st.Blocks < 0 || st.Blocks > (math.MaxInt64-physical)/512 || st.Size < 0 || st.Size > math.MaxInt64-logical {
				bad("allocation_unknown")
				return
			}
			key := fileKey{uint64(st.Dev), st.Ino}
			if local[key] {
				return
			}
			local[key] = true
			allocated := st.Blocks * 512
			physical += allocated
			if kind == unix.S_IFREG {
				logical += st.Size
				u.Files++
			}
			if !seen[key] {
				attributed += allocated
			}
		}
		var walk func(int, int)
		walk = func(current, depth int) {
			file := os.NewFile(uintptr(current), "storage-observation")
			defer file.Close()
			if depth > 64 {
				bad("scan_limit_reached")
				return
			}
			if limited() {
				return
			}
			var st unix.Stat_t
			if unix.Fstat(current, &st) != nil {
				bad("source_changed_or_unreadable")
				return
			}
			if len(resource.LogActiveFiles) == 0 {
				visit(st)
			}
			if st.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(st.Dev) != uint64(rootStat.Dev) {
				return
			}
			for {
				if limited() {
					return
				}
				names, e := file.Readdirnames(64)
				if e != nil && e != io.EOF {
					bad("source_partially_unreadable")
					return
				}
				for _, name := range names {
					if limited() {
						return
					}
					if len(resource.LogActiveFiles) > 0 {
						entries++
						included := false
						for _, active := range resource.LogActiveFiles {
							_, rotated := rotationTime(name, active)
							if name == active || rotated {
								included = true
								break
							}
						}
						if !included {
							continue
						}
					}
					var child unix.Stat_t
					if unix.Fstatat(current, name, &child, unix.AT_SYMLINK_NOFOLLOW) != nil {
						bad("source_changed_or_unreadable")
						continue
					}
					if child.Mode&unix.S_IFMT == unix.S_IFDIR {
						if len(resource.LogActiveFiles) > 0 {
							continue
						}
						if uint64(child.Dev) != uint64(rootStat.Dev) {
							entries++
							bad("nested_mount_skipped")
							continue
						}
						childFD, e := unix.Openat(current, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
						if e != nil {
							bad("source_changed_or_unreadable")
							continue
						}
						walk(childFD, depth+1)
					} else {
						visit(child)
					}
				}
				if e == io.EOF {
					return
				}
			}
		}
		walk(fd, 0)
		if complete {
			u.State = "observed"
			u.PhysicalBytes, u.LogicalBytes, u.AttributedBytes = pointer(physical), pointer(logical), pointer(attributed)
			for key := range local {
				seen[key] = true
			}
		} else {
			u.State = "partial"
			u.PartialBytes = physical
		}
		out.Resources = append(out.Resources, u)
	}
	return out
}
