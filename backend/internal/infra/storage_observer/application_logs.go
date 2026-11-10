package storageobserver

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	logfiles "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
	"golang.org/x/sys/unix"
)

// ApplicationLogSource 由部署配置构造；HTTP 不能指定目录、活动文件或匹配表达式。
type ApplicationLogSource struct {
	Directory                     string
	ActiveFiles                   []string
	RetentionDays, RetentionCount int
	NativeManaged                 bool
}
type ApplicationLogs struct {
	Sources       map[string]ApplicationLogSource
	LegacyEnabled func() bool
	Now           func() time.Time
}

func (a *ApplicationLogs) Control(id string) *storagegov.LogControl {
	s, ok := a.Sources[id]
	if !ok {
		return nil
	}
	out := &storagegov.LogControl{Supported: true, NativeManaged: s.NativeManaged, RetentionDays: s.RetentionDays, RetentionCount: s.RetentionCount}
	if a.LegacyEnabled != nil && a.LegacyEnabled() {
		out.BlockedReason = "legacy_retention_enabled"
	}
	return out
}
func (a *ApplicationLogs) clock() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}
func (a *ApplicationLogs) WithLock(ctx context.Context, id string, fn func() error) error {
	s, ok := a.Sources[id]
	if !ok {
		return fmt.Errorf("unknown log source")
	}
	fd, err := openSource(s.Directory)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return logfiles.WithDirectoryLockFD(ctx, fd, fn)
}
func rotationTime(name, active string) (time.Time, bool) {
	name = strings.TrimSuffix(name, ".gz")
	ext := filepath.Ext(active)
	prefix := strings.TrimSuffix(active, ext) + "-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
		return time.Time{}, false
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
	t, err := time.Parse("2006-01-02T15-04-05.000", stamp)
	return t, err == nil
}
func logStat(st unix.Stat_t, name string) storagegov.LogCandidate {
	return storagegov.LogCandidate{Name: name, Device: uint64(st.Dev), Inode: st.Ino, Size: st.Size, Blocks: st.Blocks, ModifiedNS: st.Mtim.Nano(), ChangedNS: st.Ctim.Nano(), Outcome: "candidate"}
}
func (a *ApplicationLogs) Inspect(ctx context.Context, id string, cutoff time.Time) (storagegov.LogScan, error) {
	s, ok := a.Sources[id]
	out := storagegov.LogScan{Complete: true, Skipped: map[string]int{}, Candidates: []storagegov.LogCandidate{}}
	if !ok {
		return out, fmt.Errorf("unknown log source")
	}
	fd, err := openSource(s.Directory)
	if err != nil {
		return out, err
	}
	f := os.NewFile(uintptr(fd), "registered-log-directory")
	defer f.Close()
	names, err := f.Readdirnames(1001)
	if err != nil && err != io.EOF {
		return out, err
	}
	if len(names) > 1000 {
		out.Complete = false
		names = names[:1000]
	}
	var root unix.Stat_t
	if unix.Fstat(fd, &root) != nil {
		return out, fmt.Errorf("log source unavailable")
	}
	if root.Mode&0022 != 0 || int(root.Uid) != unix.Geteuid() {
		out.Complete = false
		out.Skipped["unsafe_directory"]++
		return out, nil
	}
	out.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%v", s.Directory, root.Dev, root.Ino, s.ActiveFiles))))
	latest := map[string]time.Time{}
	for _, name := range names {
		for _, active := range s.ActiveFiles {
			if t, match := rotationTime(name, active); match && t.After(latest[active]) {
				latest[active] = t
			}
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		reason := "unknown_file"
		eligible := false
		for _, active := range s.ActiveFiles {
			if name == active {
				reason = "active_file"
				break
			}
			if t, match := rotationTime(name, active); match {
				eligible = true
				reason = ""
				if !t.Before(latest[active]) {
					reason = "latest_rotation"
				} else if !t.Before(cutoff) {
					reason = "within_retention"
				}
				break
			}
		}
		if !eligible || reason != "" {
			out.Skipped[reason]++
			continue
		}
		var st unix.Stat_t
		if unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
			out.Skipped["unreadable"]++
			continue
		}
		if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Dev != root.Dev || int(st.Uid) != unix.Geteuid() {
			out.Skipped["unsafe_file"]++
			continue
		}
		if !time.Unix(0, st.Mtim.Nano()).Before(cutoff) || !time.Unix(0, st.Mtim.Nano()).Before(a.clock().Add(-5*time.Minute)) {
			out.Skipped["recently_modified"]++
			continue
		}
		if st.Size < 0 || st.Size > (1<<53-1)/32 || st.Blocks < 0 {
			out.Skipped["unreadable"]++
			continue
		}
		if len(out.Candidates) == 32 {
			out.Skipped["next_batch"]++
			continue
		}
		out.Candidates = append(out.Candidates, logStat(st, name))
	}
	return out, nil
}
func (a *ApplicationLogs) Remove(ctx context.Context, id string, c storagegov.LogCandidate) (string, error) {
	if err := ctx.Err(); err != nil {
		return "not_attempted", err
	}
	s, ok := a.Sources[id]
	if !ok || filepath.Base(c.Name) != c.Name || c.Name == "." {
		return "changed", nil
	}
	valid := false
	for _, active := range s.ActiveFiles {
		if _, match := rotationTime(c.Name, active); match {
			valid = true
		}
	}
	if !valid {
		return "changed", nil
	}
	dir, err := openSource(s.Directory)
	if err != nil {
		return "unreadable", err
	}
	defer unix.Close(dir)
	fd, err := unix.Openat(dir, c.Name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return "missing", nil
	}
	if err != nil {
		return "changed", nil
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return "unreadable", nil
	}
	current := logStat(st, c.Name)
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || current.Device != c.Device || current.Inode != c.Inode || current.Size != c.Size || current.Blocks != c.Blocks || current.ModifiedNS != c.ModifiedNS || current.ChangedNS != c.ChangedNS {
		return "changed", nil
	}
	for _, active := range s.ActiveFiles {
		var activeStat unix.Stat_t
		if unix.Fstatat(dir, active, &activeStat, unix.AT_SYMLINK_NOFOLLOW) == nil && activeStat.Dev == st.Dev && activeStat.Ino == st.Ino {
			return "active_file", nil
		}
	}
	var named unix.Stat_t
	if unix.Fstatat(dir, c.Name, &named, unix.AT_SYMLINK_NOFOLLOW) != nil || named.Dev != st.Dev || named.Ino != st.Ino {
		return "changed", nil
	}
	if err = unix.Unlinkat(dir, c.Name, 0); err != nil {
		return "failed", err
	}
	return "deleted", nil
}
