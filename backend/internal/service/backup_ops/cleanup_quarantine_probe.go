package backup_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"golang.org/x/sys/unix"
)

// 文件证据仅摘要输出，不导出路径、内容、设备明细或凭据。
type quarantineStamp struct {
	Device      uint64
	Inode       uint64
	Size        int64
	Mode        uint32
	Modified    int64
	ChangedSec  int64
	ChangedNano int64
	Links       uint64
}

func quarantineFileStamp(info os.FileInfo) (quarantineStamp, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return quarantineStamp{}, errReviewUnsafe
	}
	v := reflect.ValueOf(*st)
	c := v.FieldByName("Ctim")
	if !c.IsValid() {
		c = v.FieldByName("Ctimespec")
	}
	if !c.IsValid() {
		return quarantineStamp{}, errReviewUnsafe
	}
	return quarantineStamp{uint64(st.Dev), uint64(st.Ino), info.Size(), uint32(info.Mode()), info.ModTime().UnixNano(), c.FieldByName("Sec").Int(), c.FieldByName("Nsec").Int(), uint64(st.Nlink)}, nil
}

type quarantineFile struct {
	Name    string
	Present bool
	Stamp   quarantineStamp
	file    *os.File
}
type quarantineSnapshot struct {
	parent      *os.File
	parentPath  string
	parentStamp quarantineStamp
	files       []quarantineFile
}

func (s *quarantineSnapshot) close() {
	if s == nil {
		return
	}
	for _, f := range s.files {
		if f.file != nil {
			f.file.Close()
		}
	}
	if s.parent != nil {
		s.parent.Close()
	}
}

func openQuarantineParent(path string) (*os.File, error) {
	if !filepath.IsAbs(path) {
		return nil, errReviewUnsafe
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	for _, part := range strings.Split(strings.Trim(filepath.Clean(path), "/"), "/") {
		if part == "" {
			continue
		}
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), "quarantine-parent"), nil
}
func openQuarantineChild(parent *os.File, name string) (*os.File, error) {
	fd, e := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "quarantine-evidence")
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || st.Nlink != 1 {
		f.Close()
		return nil, errReviewUnsafe
	}
	return f, nil
}
func captureQuarantineSnapshot(root string, row repo.CleanupReviewRow) (*quarantineSnapshot, error) {
	path, e := parseFileStorageURI(row.StorageURI)
	if e != nil || filepath.Ext(path) != ".dump" || !filepath.IsAbs(root) {
		return nil, errReviewUnsafe
	}
	rel, e := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errReviewUnsafe
	}
	parent, e := openQuarantineParent(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	snap := &quarantineSnapshot{parent: parent, parentPath: filepath.Dir(path)}
	fail := func(err error) (*quarantineSnapshot, error) { snap.close(); return nil, err }
	info, e := parent.Stat()
	if e != nil {
		return fail(e)
	}
	snap.parentStamp, e = quarantineFileStamp(info)
	if e != nil {
		return fail(e)
	}
	for _, suffix := range []string{"", ".json", ".sha256"} {
		entry := quarantineFile{Name: filepath.Base(path) + suffix}
		f, err := openQuarantineChild(parent, entry.Name)
		if errors.Is(err, os.ErrNotExist) {
			snap.files = append(snap.files, entry)
			continue
		}
		if err != nil {
			return fail(err)
		}
		entry.Present = true
		entry.file = f
		info, err = f.Stat()
		if err != nil {
			f.Close()
			return fail(err)
		}
		entry.Stamp, err = quarantineFileStamp(info)
		if err != nil {
			f.Close()
			return fail(err)
		}
		snap.files = append(snap.files, entry)
	}
	if !snap.stable() {
		return fail(controlFailure(409, "BACKUP_QUARANTINE_EVIDENCE_CHANGED"))
	}
	return snap, nil
}
func (s *quarantineSnapshot) parentStable() bool {
	info, e := s.parent.Stat()
	if e != nil {
		return false
	}
	stamp, e := quarantineFileStamp(info)
	if e != nil || stamp != s.parentStamp {
		return false
	}
	f, e := openQuarantineParent(s.parentPath)
	if e != nil {
		return false
	}
	defer f.Close()
	info, e = f.Stat()
	if e != nil {
		return false
	}
	stamp, e = quarantineFileStamp(info)
	return e == nil && stamp == s.parentStamp
}
func (s *quarantineSnapshot) stable() bool {
	if !s.parentStable() {
		return false
	}
	for _, entry := range s.files {
		f, e := openQuarantineChild(s.parent, entry.Name)
		if !entry.Present {
			if f != nil {
				f.Close()
			}
			if !errors.Is(e, os.ErrNotExist) {
				return false
			}
			continue
		}
		if e != nil {
			return false
		}
		info, e := f.Stat()
		f.Close()
		if e != nil {
			return false
		}
		stamp, e := quarantineFileStamp(info)
		if e != nil || stamp != entry.Stamp {
			return false
		}
		info, e = entry.file.Stat()
		if e != nil {
			return false
		}
		stamp, e = quarantineFileStamp(info)
		if e != nil || stamp != entry.Stamp {
			return false
		}
	}
	return s.parentStable()
}
func (s *quarantineSnapshot) digest(row repo.CleanupReviewRow, diagnosis string) string {
	type item struct {
		Present bool
		Stamp   quarantineStamp
	}
	files := []item{}
	for _, f := range s.files {
		files = append(files, item{f.Present, f.Stamp})
	}
	return historyHash(struct {
		Contract, Metadata, Diagnosis string
		Parent                        quarantineStamp
		Files                         []item
	}{"powerx.quarantine-evidence/v1", cleanupReviewVersion(row), diagnosis, s.parentStamp, files})
}

// probeQuarantine 重做有界诊断；只有稳定的已知缺损可进入提交，永不把缺损当有效备份。
func probeQuarantine(ctx context.Context, root string, row repo.CleanupReviewRow, progress func(CleanupVerification) error, commit func(string, string, func() bool) error) (CleanupVerification, string, error) {
	out := CleanupVerification{ArtifactID: cleanupReviewItem(row).ArtifactID, Token: row.CleanupToken, State: "running", TotalBytes: max(row.SizeBytes, 0), ReadOnly: true, CleanupUnchanged: true, ObservedAt: time.Now().UTC()}
	if e := ctx.Err(); e != nil {
		return out, "", e
	}
	snap, e := captureQuarantineSnapshot(root, row)
	if e != nil {
		out.State = reviewFileState(e)
		// 无法固定父目录时，不能把路径解析失败当作已证明的文件缺失。
		if out.State == "missing" {
			out.State = "unavailable"
		}
		if strings.Contains(e.Error(), "EVIDENCE_CHANGED") {
			out.State = "changed"
		}
		return out, "", nil
	}
	defer snap.close()
	if e = progress(out); e != nil {
		return out, "", e
	}
	out, e = verifyCleanupContent(ctx, root, row, progress)
	if e != nil {
		return out, "", e
	}
	if ctx.Err() != nil {
		return out, "", ctx.Err()
	}
	if !snap.stable() {
		out.State = "changed"
		out.ContentVerified = false
		return out, "", nil
	}
	digest := snap.digest(row, out.State)
	if recovery.ValidQuarantineDiagnosis(out.State) && commit != nil {
		if e = commit(out.State, digest, func() bool { return ctx.Err() == nil && snap.stable() }); e != nil {
			return out, "", e
		}
	}
	return out, digest, nil
}
