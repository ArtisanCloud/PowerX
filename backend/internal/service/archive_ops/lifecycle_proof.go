package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// VerifyLifecycleFile 是内部固定来源握手；必须重新验证远端对象，不能以历史上传成功代替。
func (s *Service) VerifyLifecycleFile(ctx context.Context, directory, name string, size int64, modified time.Time) (bool, error) {
	if s == nil || s.initErr != nil || s.ctx.Err() != nil || filepath.Base(name) != name || size < 0 {
		return false, nil
	}
	var source *Source
	for _, registered := range s.cfg.Sources {
		if registered.Directory == directory {
			copy := registered
			source = &copy
			break
		}
	}
	if source == nil {
		return false, nil
	}
	ready, err := s.LifecycleArchiveReady(ctx, directory)
	if err != nil || !ready {
		return false, err
	}
	fd, err := unix.Open(filepath.Join(directory, name), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), "sealed-log-proof")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size || !info.ModTime().Equal(modified) {
		return false, nil
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, &contextReader{ctx, file}); err != nil {
		return false, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	ids, err := s.records("jobs")
	if err != nil {
		return false, err
	}
	if len(ids) > 1000 {
		return false, nil
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		j, err := s.readJob(id)
		if err != nil {
			return false, err
		}
		if j.SourceID != source.ID || j.Status != "success" || j.RemoteState != "success" || j.Target == nil {
			continue
		}
		if err := s.transferAllowed(ctx, j); err != nil {
			continue
		}
		var found *Entry
		for _, entry := range j.Files {
			if filepath.Base(entry.Path) == name && entry.Size == size && entry.SHA256 == digest && entry.ModifiedAt.Equal(modified) {
				copy := entry
				found = &copy
				break
			}
		}
		if found == nil {
			continue
		}
		result, err := s.remote(ctx, *j.Target, "verify", j.PolicyID, j.ID, nil)
		if err != nil {
			return false, err
		}
		raw, err := json.Marshal(result["manifest"])
		if err != nil {
			return false, err
		}
		var remote Job
		if json.Unmarshal(raw, &remote) != nil || remote.ID != j.ID || remote.PolicyID != j.PolicyID || remote.SourceID != j.SourceID {
			return false, nil
		}
		for _, entry := range remote.Files {
			if entry.Path == found.Path && entry.Size == found.Size && entry.SHA256 == found.SHA256 {
				return true, nil
			}
		}
	}
	return false, nil
}
