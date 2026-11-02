package storagelifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// ActiveFiles contains trusted filename prefixes for this format, not a snapshot
// of active names. PostgreSQL replaces current_logfiles on each rotation.
func validPostgresSource(s Source) bool {
	if s.Kind != "postgresql_logs" || !filepath.IsAbs(s.CurrentLogfiles) || filepath.Clean(s.CurrentLogfiles) != s.CurrentLogfiles || filepath.Base(s.CurrentLogfiles) != "current_logfiles" || len(s.ActiveFiles) == 0 || len(s.ActiveFiles) > 16 {
		return false
	}
	for _, prefix := range s.ActiveFiles {
		if prefix == "" || prefix == "." || prefix == ".." || filepath.Base(prefix) != prefix || strings.ContainsAny(prefix, "\x00\r\n") {
			return false
		}
	}
	return true
}

func postgresLogGroup(s Source, name string) string {
	if filepath.Base(name) != name {
		return ""
	}
	raw := strings.TrimSuffix(name, ".gz")
	ext := filepath.Ext(raw)
	if ext != ".log" && ext != ".csv" && ext != ".json" {
		return ""
	}
	for _, prefix := range s.ActiveFiles {
		if !strings.HasPrefix(raw, prefix) {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(raw, prefix), ext)
		if parsed, err := time.Parse("2006-01-02_150405", stamp); err == nil && parsed.Format("2006-01-02_150405") == stamp {
			return prefix + ext
		}
	}
	return ""
}

type postgresActiveSet struct {
	active map[string]bool
	token  string
}

// Read only a bounded, owner-controlled manifest. A missing, malformed, replaced
// or foreign-directory manifest never turns every file into a sealed candidate.
func postgresActivity(s Source) (postgresActiveSet, error) {
	if s.Format != "postgresql" {
		return postgresActiveSet{}, nil
	}
	if !validPostgresSource(s) {
		return postgresActiveSet{}, ErrBlocked
	}
	var directory unix.Stat_t
	if unix.Lstat(s.Directory, &directory) != nil || directory.Mode&unix.S_IFMT != unix.S_IFDIR || directory.Uid != s.OwnerUID || directory.Mode&0022 != 0 {
		return postgresActiveSet{}, ErrBlocked
	}
	parent, err := unix.Open(filepath.Dir(s.CurrentLogfiles), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return postgresActiveSet{}, ErrBlocked
	}
	defer unix.Close(parent)
	var root, before, after unix.Stat_t
	if unix.Fstat(parent, &root) != nil || root.Uid != s.OwnerUID || root.Mode&0022 != 0 {
		return postgresActiveSet{}, ErrBlocked
	}
	fd, err := unix.Openat(parent, "current_logfiles", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return postgresActiveSet{}, ErrBlocked
	}
	f := os.NewFile(uintptr(fd), "postgresql-current-logfiles")
	defer f.Close()
	if unix.Fstat(fd, &before) != nil || before.Mode&unix.S_IFMT != unix.S_IFREG || before.Nlink != 1 || before.Uid != s.OwnerUID || before.Mode&0022 != 0 || before.Size < 1 || before.Size > 4096 {
		return postgresActiveSet{}, ErrBlocked
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 || unix.Fstat(fd, &after) != nil || before.Ino != after.Ino || before.Size != after.Size || before.Mtim != after.Mtim || before.Ctim != after.Ctim || strings.ContainsAny(string(raw), "\x00\r") {
		return postgresActiveSet{}, ErrBlocked
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) < 1 || len(lines) > 3 {
		return postgresActiveSet{}, ErrBlocked
	}
	result := postgresActiveSet{active: map[string]bool{}}
	seen := map[string]bool{}
	identities := ""
	for _, line := range lines {
		destination, path, ok := strings.Cut(line, " ")
		ext := map[string]string{"stderr": ".log", "csvlog": ".csv", "jsonlog": ".json"}[destination]
		if !ok || ext == "" || seen[destination] || path == "" || strings.TrimSpace(path) != path {
			return postgresActiveSet{}, ErrBlocked
		}
		seen[destination] = true
		for _, component := range strings.Split(filepath.ToSlash(path), "/") {
			if component == ".." {
				return postgresActiveSet{}, ErrBlocked
			}
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(s.CurrentLogfiles), path)
		}
		path = filepath.Clean(path)
		name := filepath.Base(path)
		if filepath.Dir(path) != s.Directory || !strings.HasSuffix(name, ext) || postgresLogGroup(s, name) == "" || result.active[name] {
			return postgresActiveSet{}, ErrBlocked
		}
		var active unix.Stat_t
		if unix.Lstat(path, &active) != nil || active.Mode&unix.S_IFMT != unix.S_IFREG || active.Nlink != 1 || active.Uid != s.OwnerUID || active.Mode&0022 != 0 {
			return postgresActiveSet{}, ErrBlocked
		}
		result.active[name] = true
		identities += fmt.Sprintf(":%s:%d:%d", name, active.Dev, active.Ino)
	}
	result.token = fmt.Sprintf("%d:%d:%d:%d:%d:%d:%x%s", root.Dev, root.Ino, before.Ino, before.Size, before.Mtim.Nano(), before.Ctim.Nano(), sha256.Sum256(raw), identities)
	return result, nil
}

func (a postgresActiveSet) unchanged(s Source) bool {
	if s.Format != "postgresql" {
		return true
	}
	next, err := postgresActivity(s)
	return err == nil && next.token == a.token
}

// After atomic capture, PostgreSQL cannot reopen this private name. A handle
// opened before capture still refers to this inode and must stop deletion.
// Missing lsof, insufficient ownership, warnings and timeouts fail closed.
func postgresFileClosed(ctx context.Context, s Source, path string) bool {
	if os.Geteuid() != 0 && uint32(os.Geteuid()) != s.OwnerUID {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "lsof", "-nP", "-F", "p", "--", path)
	var stdout, stderr boundedPostgresOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exited *exec.ExitError
	if ctx.Err() != nil || stdout.Len() != 0 || stderr.Len() != 0 {
		return false
	}
	if errors.As(err, &exited) {
		return exited.ExitCode() == 1
	}
	return false
}

type boundedPostgresOutput struct{ buffer bytes.Buffer }

func (b *boundedPostgresOutput) Len() int { return b.buffer.Len() }

func (b *boundedPostgresOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 4096 {
		return 0, io.ErrShortBuffer
	}
	return b.buffer.Write(p)
}
