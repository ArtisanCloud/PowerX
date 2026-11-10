package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
)

// CleanupVerification is an ephemeral diagnostic, never a deletion or restore proof.
type CleanupVerification struct {
	ArtifactID       string    `json:"artifact_id"`
	Token            string    `json:"token"`
	State            string    `json:"state"`
	BytesRead        int64     `json:"bytes_read"`
	TotalBytes       int64     `json:"total_bytes"`
	DumpMatched      bool      `json:"dump_matched"`
	ContentVerified  bool      `json:"content_verified"`
	ReadOnly         bool      `json:"read_only"`
	CleanupUnchanged bool      `json:"cleanup_unchanged"`
	ObservedAt       time.Time `json:"observed_at"`
}

func sameCleanupReview(a, b repo.CleanupReviewRow) bool {
	return a.ID == b.ID && a.JobID == b.JobID && a.PolicyID == b.PolicyID && a.CleanupToken == b.CleanupToken && a.CleanupState == b.CleanupState && a.StorageURI == b.StorageURI && a.SizeBytes == b.SizeBytes && a.Checksum == b.Checksum && a.ArtifactDeleted == b.ArtifactDeleted && a.JobFound == b.JobFound && a.JobDeleted == b.JobDeleted && a.JobStatus == b.JobStatus && a.JobProtected == b.JobProtected && a.UpdatedAt.Equal(b.UpdatedAt)
}

// VerifyCleanupReview runs only while its caller is connected. No worker/job/retry is created.
// The existing session lock prevents concurrent cooperating backup operations; no long SQL transaction is held.
func (s *JobService) VerifyCleanupReview(ctx context.Context, id uint64, token string, progress func(CleanupVerification) error) (CleanupVerification, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return CleanupVerification{}, e
	}
	if id == 0 || !validCleanupToken(token) || progress == nil {
		return CleanupVerification{}, controlFailure(400, "BACKUP_REVIEW_INTENT_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pool, e := s.db.DB()
	if e != nil {
		return CleanupVerification{}, reviewReadError(e)
	}
	if pool.Stats().MaxOpenConnections == 1 {
		return CleanupVerification{}, controlFailure(503, "BACKUP_REVIEW_DATABASE_POOL_INSUFFICIENT")
	}
	row, e := s.verificationRow(run, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupVerification{}, controlFailure(404, "BACKUP_REVIEW_NOT_FOUND")
	}
	if e != nil {
		return CleanupVerification{}, reviewReadError(e)
	}
	if row.CleanupToken != token || !cleanupReviewItem(row).Inspectable || row.PolicyID == 0 {
		return CleanupVerification{}, controlFailure(409, "BACKUP_REVIEW_INTENT_CHANGED")
	}
	acquire, done := context.WithTimeout(run, 3*time.Second)
	unlock, e := lockBackupPolicy(acquire, s.db, row.PolicyID)
	done()
	if errors.Is(e, ErrBackupJobAlreadyRunning) {
		return CleanupVerification{}, controlFailure(409, "BACKUP_REVIEW_BUSY")
	}
	if e != nil {
		return CleanupVerification{}, reviewReadError(e)
	}
	defer unlock()
	current, e := s.verificationRow(run, id)
	if e != nil {
		return CleanupVerification{}, reviewReadError(e)
	}
	if !sameCleanupReview(row, current) {
		return CleanupVerification{}, controlFailure(409, "BACKUP_REVIEW_INTENT_CHANGED")
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return CleanupVerification{}, controlFailure(503, "BACKUP_CLEANUP_REVIEW_UNAVAILABLE")
	}
	out, e := verifyCleanupContent(run, root, row, progress)
	if e != nil {
		return CleanupVerification{}, e
	}
	current, e = s.verificationRow(run, id)
	if e != nil || !sameCleanupReview(row, current) {
		out.ContentVerified = false
		if errors.Is(e, gorm.ErrRecordNotFound) || e == nil {
			out.State = "changed"
		} else {
			out.State = "unavailable"
		}
	}
	out.ObservedAt = time.Now().UTC()
	return out, nil
}

func (s *JobService) verificationRow(ctx context.Context, id uint64) (repo.CleanupReviewRow, error) {
	run, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return s.artifactRepo.GetCleanupReview(run, id)
}

var errReviewUnsafe = errors.New("unsafe backup review file")

// Open every path component through a pinned descriptor. Nonblocking prevents FIFO/device stalls before Stat rejects them.
func openReviewFile(root, path string) (*os.File, error) {
	if !filepath.IsAbs(root) || !filepath.IsAbs(path) {
		return nil, errReviewUnsafe
	}
	rel, e := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errReviewUnsafe
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	parts := strings.Split(strings.TrimPrefix(filepath.Clean(path), "/"), "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), "backup-review")
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Nlink != 1 {
		f.Close()
		return nil, errReviewUnsafe
	}
	return f, nil
}
func reviewFileState(e error) string {
	switch {
	case errors.Is(e, os.ErrNotExist):
		return "missing"
	case errors.Is(e, errReviewUnsafe) || errors.Is(e, unix.ELOOP) || errors.Is(e, unix.ENOTDIR):
		return "unsafe"
	default:
		return "unavailable"
	}
}
func reviewFileStable(root, path string, f *os.File, before os.FileInfo) bool {
	after, e := f.Stat()
	if e != nil || !sameControlledArtifact(before, after) {
		return false
	}
	stat, ok := after.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return false
	}
	current, e := openReviewFile(root, path)
	if e != nil {
		return false
	}
	defer current.Close()
	info, e := current.Stat()
	return e == nil && sameControlledArtifact(before, info)
}

func verifyCleanupContent(ctx context.Context, root string, row repo.CleanupReviewRow, progress func(CleanupVerification) error) (CleanupVerification, error) {
	return verifyCleanupContentFinal(ctx, root, row, progress, nil)
}
func verifyCleanupContentFinal(ctx context.Context, root string, row repo.CleanupReviewRow, progress func(CleanupVerification) error, commit func(func() bool) error) (CleanupVerification, error) {
	out := CleanupVerification{ArtifactID: cleanupReviewItem(row).ArtifactID, Token: row.CleanupToken, State: "running", TotalBytes: row.SizeBytes, ReadOnly: true, CleanupUnchanged: true, ObservedAt: time.Now().UTC()}
	if row.SizeBytes <= 0 || !cleanupReviewChecksum.MatchString(row.Checksum) {
		out.State = "metadata_invalid"
		if out.TotalBytes < 0 {
			out.TotalBytes = 0
		}
		return out, nil
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	// Parse validates the registered URI without following any filesystem links.
	path, e := parseFileStorageURI(row.StorageURI)
	if e != nil || filepath.Ext(path) != ".dump" {
		out.State = "unsafe"
		return out, nil
	}
	dump, e := openReviewFile(root, path)
	if e != nil {
		out.State = reviewFileState(e)
		return out, nil
	}
	defer dump.Close()
	before, e := dump.Stat()
	if e != nil {
		out.State = "unavailable"
		return out, nil
	}
	if before.Size() != row.SizeBytes {
		out.State = "size_changed"
		return out, nil
	}
	if e = progress(out); e != nil {
		return out, e
	}
	h := sha256.New()
	buffer := make([]byte, 64<<10)
	last := time.Now()
	lastBytes := int64(0)
	// Bound reads by the captured size; growth cannot turn this into an unbounded read.
	remaining := row.SizeBytes
	for remaining > 0 {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		n, err := dump.Read(buffer[:min(int64(len(buffer)), remaining)])
		if n > 0 {
			h.Write(buffer[:n])
			out.BytesRead += int64(n)
			remaining -= int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				out.State = "size_changed"
				return out, nil
			}
			out.State = "unavailable"
			return out, nil
		}
		if n == 0 {
			out.State = "unavailable"
			return out, nil
		}
		if out.BytesRead-lastBytes >= 4<<20 || time.Since(last) >= 250*time.Millisecond {
			out.ObservedAt = time.Now().UTC()
			if e = progress(out); e != nil {
				return out, e
			}
			last = time.Now()
			lastBytes = out.BytesRead
		}
	}
	if !reviewFileStable(root, path, dump, before) {
		out.State = "changed"
		return out, nil
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), row.Checksum) {
		out.State = "checksum_mismatch"
		return out, nil
	}
	out.DumpMatched = true
	type sidecarSnapshot struct {
		path string
		file *os.File
		info os.FileInfo
	}
	var sidecars []sidecarSnapshot
	defer func() {
		for _, v := range sidecars {
			v.file.Close()
		}
	}()
	// Sidecars are optional on legacy backups; absence never becomes a complete verification.
	for _, suffix := range []string{".json", ".sha256"} {
		if e = ctx.Err(); e != nil {
			return out, e
		}
		f, err := openReviewFile(root, path+suffix)
		if err != nil {
			out.State = reviewFileState(err)
			if out.State == "missing" {
				out.State = "sidecars_missing"
			}
			return out, nil
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			out.State = "unavailable"
			return out, nil
		}
		if info.Size() <= 0 || info.Size() > 65536 {
			f.Close()
			out.State = "sidecar_mismatch"
			return out, nil
		}
		data, err := io.ReadAll(io.LimitReader(backupChecksumReader{ctx, f}, 65537))
		stable := reviewFileStable(root, path+suffix, f, info)
		sidecars = append(sidecars, sidecarSnapshot{path + suffix, f, info})
		if err != nil {
			return out, err
		}
		if !stable {
			out.State = "changed"
			return out, nil
		}
		valid := false
		if suffix == ".sha256" {
			value := strings.TrimSpace(string(data))
			valid = len(value) == 64+2+len(filepath.Base(path)) && strings.EqualFold(value[:64], row.Checksum) && value[64:] == "  "+filepath.Base(path)
		} else {
			var manifest artifactManifest
			valid = json.Unmarshal(data, &manifest) == nil && manifest.Contract == "powerx.database-backup/v1" && manifest.JobID == row.JobID && manifest.PolicyID == row.PolicyID && manifest.File == filepath.Base(path) && manifest.Size == row.SizeBytes && strings.EqualFold(manifest.SHA256, row.Checksum) && manifest.Format == "postgresql-custom" && !manifest.CreatedAt.IsZero() && manifest.Database != ""
		}
		if !valid {
			out.State = "sidecar_mismatch"
			return out, nil
		}
	}
	if e = ctx.Err(); e != nil {
		return out, e
	}
	if !reviewFileStable(root, path, dump, before) {
		out.State = "changed"
		return out, nil
	}
	for _, v := range sidecars {
		if !reviewFileStable(root, v.path, v.file, v.info) {
			out.State = "changed"
			return out, nil
		}
	}
	if commit != nil {
		if e := commit(func() bool {
			if !reviewFileStable(root, path, dump, before) {
				return false
			}
			for _, v := range sidecars {
				if !reviewFileStable(root, v.path, v.file, v.info) {
					return false
				}
			}
			return ctx.Err() == nil
		}); e != nil {
			return out, e
		}
	}
	out.State = "verified"
	out.ContentVerified = true
	return out, nil
}
