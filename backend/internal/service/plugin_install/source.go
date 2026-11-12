package plugininstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
)

func (s *Service) taskDirectory(id string) string { return filepath.Join(s.opts.Root, id) }
func privateDirectory(path string) error {
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	info, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return problem(409, "PLUGIN_INSTALL_STAGING_UNSAFE")
	}
	return nil
}
func fileLock(ctx context.Context, path string) (func(), error) {
	fd, e := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	file := os.NewFile(uintptr(fd), path)
	for {
		e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = file.Close() }, nil
		}
		if e != unix.EWOULDBLOCK && e != unix.EAGAIN {
			file.Close()
			return nil, e
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// copySource rejects links, changing files, unbounded archives and non-regular
// entries. The private destination is never selected by an HTTP path field.
func copySource(ctx context.Context, src, dst string) (string, int64, error) {
	root, e := filepath.Abs(src)
	if e != nil {
		return "", 0, e
	}
	info, e := os.Lstat(root)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", 0, problem(400, "PLUGIN_INSTALL_SOURCE_INVALID")
	}
	if filepath.Clean(root) == filepath.Clean(dst) || strings.HasPrefix(dst, root+string(filepath.Separator)) {
		return "", 0, problem(400, "PLUGIN_INSTALL_SOURCE_INVALID")
	}
	hash := sha256.New()
	var size int64
	files := 0
	e = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		stat, e := entry.Info()
		if e != nil {
			return e
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			return problem(400, "PLUGIN_INSTALL_SOURCE_LINK_FORBIDDEN")
		}
		if entry.IsDir() {
			if dst != "" {
				return os.MkdirAll(filepath.Join(dst, rel), 0700)
			}
			return nil
		}
		if !stat.Mode().IsRegular() || stat.Size() < 0 {
			return problem(400, "PLUGIN_INSTALL_SOURCE_INVALID")
		}
		if st, ok := stat.Sys().(*syscall.Stat_t); ok && st.Nlink != 1 {
			return problem(400, "PLUGIN_INSTALL_SOURCE_LINK_FORBIDDEN")
		}
		files++
		if files > 10000 || stat.Size() > MaxSourceBytes-size {
			return problem(413, "PLUGIN_INSTALL_SOURCE_TOO_LARGE")
		}
		fd, e := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
		if e != nil {
			return e
		}
		input := os.NewFile(uintptr(fd), path)
		defer input.Close()
		before, e := input.Stat()
		if e != nil || !os.SameFile(stat, before) {
			return problem(409, "PLUGIN_INSTALL_SOURCE_CHANGED")
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%t\x00%d\x00", filepath.ToSlash(rel), stat.Mode().Perm()&0111 != 0, stat.Size())
		var writer io.Writer = hash
		var output *os.File
		if dst != "" {
			mode := os.FileMode(0600)
			if stat.Mode().Perm()&0111 != 0 {
				mode = 0700
			}
			output, e = os.OpenFile(filepath.Join(dst, rel), os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if e != nil {
				return e
			}
			defer output.Close()
			writer = io.MultiWriter(hash, budget.Writer(ctx, output))
		}
		n, e := io.Copy(writer, io.LimitReader(sourceReader{ctx, input}, stat.Size()+1))
		if e != nil {
			return e
		}
		after, e := input.Stat()
		if e != nil || n != stat.Size() || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
			return problem(409, "PLUGIN_INSTALL_SOURCE_CHANGED")
		}
		if output != nil {
			if e = output.Sync(); e != nil {
				return e
			}
			if e = output.Close(); e != nil {
				return e
			}
		}
		if e = input.Close(); e != nil {
			return e
		}
		size += n
		return nil
	})
	if e != nil {
		return "", 0, e
	}
	if files == 0 {
		return "", 0, problem(400, "PLUGIN_INSTALL_SOURCE_INVALID")
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func (s *Service) AcceptSource(ctx context.Context, id, source string) (View, error) {
	if s == nil || s.opts.Disabled || (s.opts.RequireWorker && !s.running.Load()) {
		return View{}, problem(503, "PLUGIN_INSTALL_WORKER_DISABLED")
	}
	row, e := s.owned(ctx, id)
	if e != nil {
		return View{}, e
	}
	if row.SourceKind != "local" {
		return View{}, problem(409, "PLUGIN_INSTALL_SOURCE_NOT_EXPECTED")
	}
	if row.Status != "awaiting_upload" {
		return view(row), nil
	}
	if e = privateDirectory(s.opts.Root); e != nil {
		return View{}, e
	}
	dir := s.taskDirectory(id)
	if e = privateDirectory(dir); e != nil {
		return View{}, e
	}
	unlock, e := fileLock(ctx, filepath.Join(dir, "upload.lock"))
	if e != nil {
		return View{}, e
	}
	defer unlock()
	row, e = s.owned(ctx, id)
	if e != nil {
		return View{}, e
	}
	if row.Status != "awaiting_upload" {
		return view(row), nil
	}
	if e = purgeUploadTrees(dir, false); e != nil {
		return View{}, e
	}
	digest, bytes, e := copySource(ctx, source, "")
	if e != nil {
		return View{}, e
	}
	run, finish, e := budget.BeginPath(ctx, dir, "plugin-install-source", bytes*2+1<<20)
	if e != nil {
		return View{}, e
	}
	defer finish()
	target := filepath.Join(dir, "source")
	if _, e = os.Lstat(target); os.IsNotExist(e) {
		staging := filepath.Join(dir, "staging-"+uuid.NewString())
		if e = privateDirectory(staging); e != nil {
			return View{}, e
		}
		defer os.RemoveAll(staging)
		copied, n, e := copySource(run, source, staging)
		if e != nil {
			return View{}, e
		}
		if copied != digest || n != bytes {
			return View{}, problem(409, "PLUGIN_INSTALL_SOURCE_CHANGED")
		}
		if e = os.Rename(staging, target); e != nil {
			return View{}, e
		}
	} else if e != nil {
		return View{}, e
	}
	parent, e := os.Open(dir)
	if e != nil {
		return View{}, e
	}
	e = parent.Sync()
	_ = parent.Close()
	if e != nil {
		return View{}, e
	}
	existing, n, e := copySource(run, target, "")
	if e != nil || existing != digest || n != bytes {
		return View{}, problem(409, "PLUGIN_INSTALL_SOURCE_CHANGED")
	}
	e = s.db.WithContext(run).Transaction(func(tx *gorm.DB) error {
		if e := s.repo.Update(run, tx, row.TenantUUID, id, func(current *m.Installation) error {
			if current.Status != "awaiting_upload" {
				return problem(409, "PLUGIN_INSTALL_UPLOAD_CONFLICT")
			}
			current.SourceDigest = digest
			current.SourceBytes = bytes
			current.Status = "queued"
			current.Phase = "queued"
			current.Revision++
			*row = *current
			return nil
		}); e != nil {
			return e
		}
		return s.opts.Queue.Register(run, tx, s.message(row))
	})
	if e != nil {
		return View{}, e
	}
	s.dispatch(row)
	return view(row), nil
}

func removeOwnedSource(dir string) error { return purgeUploadTrees(dir, true) }

// The upload lock protects orphan cleanup before retry. Only recognized private
// upload trees are removed; the durable receipt and execution lock stay intact.
func purgeUploadTrees(dir string, includeSource bool) error {
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for _, entry := range entries {
		name := entry.Name()
		allowed := includeSource && name == "source"
		if strings.HasPrefix(name, "staging-") && canonicalID(strings.TrimPrefix(name, "staging-")) {
			allowed = true
		}
		if !allowed {
			continue
		}
		root := filepath.Join(dir, name)
		e = filepath.WalkDir(root, func(path string, item fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if item.Type()&os.ModeSymlink != 0 {
				return problem(409, "PLUGIN_INSTALL_STAGING_UNSAFE")
			}
			if item.IsDir() {
				return os.Chmod(path, 0700)
			}
			return nil
		})
		if e != nil {
			return e
		}
		if e = os.RemoveAll(root); e != nil {
			return e
		}
	}
	return nil
}

type sourceReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r sourceReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.reader.Read(p)
}
