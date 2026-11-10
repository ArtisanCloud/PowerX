package backup_ops

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type CleanupReviewItem struct {
	ReviewVersion string    `json:"review_version,omitempty"`
	ArtifactID    string    `json:"artifact_id"`
	JobID         string    `json:"job_id"`
	PolicyID      string    `json:"policy_id"`
	State         string    `json:"state"`
	Token         string    `json:"token"`
	SizeBytes     int64     `json:"size_bytes"`
	UpdatedAt     time.Time `json:"updated_at"`
	Protected     bool      `json:"protected"`
	Inspectable   bool      `json:"inspectable"`
	Problem       string    `json:"problem,omitempty"`
}
type CleanupReviewPage struct {
	Items      []CleanupReviewItem `json:"items"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	ObservedAt time.Time           `json:"observed_at"`
}
type CleanupReviewFile struct {
	Role      string `json:"role"`
	State     string `json:"state"`
	SizeBytes *int64 `json:"size_bytes,omitempty"`
}
type CleanupReviewInspection struct {
	Item            CleanupReviewItem   `json:"item"`
	State           string              `json:"state"`
	Files           []CleanupReviewFile `json:"files"`
	ObservedAt      time.Time           `json:"observed_at"`
	ContentVerified bool                `json:"content_verified"`
	ReadOnly        bool                `json:"read_only"`
}

var cleanupReviewChecksum = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func validCleanupToken(value string) bool {
	id, e := uuid.Parse(value)
	return e == nil && id != uuid.Nil && id.String() == value
}
func cleanupReviewItem(row repo.CleanupReviewRow) CleanupReviewItem {
	out := CleanupReviewItem{ArtifactID: strconv.FormatUint(row.ID, 10), JobID: strconv.FormatUint(row.JobID, 10), PolicyID: strconv.FormatUint(row.PolicyID, 10), State: row.CleanupState, Token: row.CleanupToken, SizeBytes: row.SizeBytes, UpdatedAt: row.UpdatedAt, Protected: row.JobProtected, Inspectable: true}
	switch {
	case !validCleanupToken(row.CleanupToken):
		out.Problem = "BACKUP_REVIEW_INTENT_INVALID"
	case !row.JobFound || row.JobDeleted || row.ArtifactDeleted:
		out.Problem = "BACKUP_REVIEW_RECORD_UNAVAILABLE"
	}
	out.Inspectable = out.Problem == ""
	return out
}
func reviewReadError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "42P01" || pg.Code == "42703") {
		return controlFailure(503, "BACKUP_CLEANUP_REVIEW_SCHEMA_NOT_READY")
	}
	return controlFailure(503, "BACKUP_CLEANUP_REVIEW_UNAVAILABLE")
}
func (s *JobService) requireReviewRoot(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return controlFailure(403, "BACKUP_ROOT_REQUIRED")
	}
	if s == nil || s.db == nil || s.artifactRepo == nil {
		return controlFailure(503, "BACKUP_CLEANUP_REVIEW_UNAVAILABLE")
	}
	return nil
}
func (s *JobService) CleanupReviews(ctx context.Context, page, size int) (CleanupReviewPage, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return CleanupReviewPage{}, e
	}
	if page < 1 || page > 100000 || size < 1 || size > 100 {
		return CleanupReviewPage{}, controlFailure(400, "BACKUP_REVIEW_PAGE_INVALID")
	}
	rows, total, e := s.artifactRepo.ListCleanupReviews(ctx, page, size)
	if e != nil {
		return CleanupReviewPage{}, reviewReadError(e)
	}
	out := CleanupReviewPage{Items: []CleanupReviewItem{}, Total: total, Page: page, PageSize: size, ObservedAt: time.Now().UTC()}
	// Missing control/anchor proof must not prevent read-only inventory, but it
	// must never produce an actionable version for the protection operation.
	binding, _ := s.retentionReviewBinding(ctx)
	for _, row := range rows {
		item := cleanupReviewItem(row)
		if binding != "" {
			item.ReviewVersion = boundCleanupReviewVersion(row, binding)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
func (s *JobService) InspectCleanupReview(ctx context.Context, id uint64, token string) (CleanupReviewInspection, error) {
	if e := s.requireReviewRoot(ctx); e != nil {
		return CleanupReviewInspection{}, e
	}
	if id == 0 || !validCleanupToken(token) {
		return CleanupReviewInspection{}, controlFailure(400, "BACKUP_REVIEW_INTENT_INVALID")
	}
	row, e := s.artifactRepo.GetCleanupReview(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupReviewInspection{}, controlFailure(404, "BACKUP_REVIEW_NOT_FOUND")
	}
	if e != nil {
		return CleanupReviewInspection{}, reviewReadError(e)
	}
	if row.CleanupToken != token {
		return CleanupReviewInspection{}, controlFailure(409, "BACKUP_REVIEW_INTENT_CHANGED")
	}
	out, e := inspectCleanupFiles(ctx, row)
	if e != nil {
		return CleanupReviewInspection{}, e
	}
	current, e := s.artifactRepo.GetCleanupReview(ctx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return CleanupReviewInspection{}, controlFailure(409, "BACKUP_REVIEW_INTENT_CHANGED")
	}
	if e != nil {
		return CleanupReviewInspection{}, reviewReadError(e)
	}
	if !sameCleanupReview(row, current) {
		return CleanupReviewInspection{}, controlFailure(409, "BACKUP_REVIEW_INTENT_CHANGED")
	}
	if binding, err := s.retentionReviewBinding(ctx); err == nil {
		out.Item.ReviewVersion = boundCleanupReviewVersion(row, binding)
	}
	return out, nil
}

// 只读结构快照，不读取 dump 内容或把缺失文件解释为删除完成。
func inspectCleanupFiles(ctx context.Context, row repo.CleanupReviewRow) (CleanupReviewInspection, error) {
	out := CleanupReviewInspection{Item: cleanupReviewItem(row), ReadOnly: true, Files: []CleanupReviewFile{}, ObservedAt: time.Now().UTC(), State: "unavailable"}
	if !out.Item.Inspectable {
		return out, nil
	}
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return out, nil
	}
	rootInfo, e := os.Lstat(root)
	if e != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return out, nil
	}
	artifact := model.BackupArtifact{StorageURI: row.StorageURI, SizeBytes: row.SizeBytes, Checksum: row.Checksum}
	if row.SizeBytes <= 0 || !cleanupReviewChecksum.MatchString(row.Checksum) {
		out.State = "metadata_invalid"
		return out, nil
	}
	path, pathErr := validatedArtifactPath(root, &artifact)
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		out.State = "unsafe"
		return out, nil
	}
	if path == "" {
		return out, nil
	}
	for i, suffix := range []string{"", ".json", ".sha256"} {
		if e := ctx.Err(); e != nil {
			return CleanupReviewInspection{}, reviewReadError(e)
		}
		file := CleanupReviewFile{Role: []string{"dump", "manifest", "checksum"}[i], State: "unavailable"}
		info, statErr := os.Lstat(path + suffix)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			file.State = "missing"
		case statErr != nil:
		case !info.Mode().IsRegular():
			file.State = "unsafe"
		default:
			meta, ok := info.Sys().(*syscall.Stat_t)
			if !ok || meta.Nlink != 1 {
				file.State = "unsafe"
			} else {
				size := info.Size()
				file.SizeBytes = &size
				file.State = "present"
				if i == 0 && size != row.SizeBytes {
					file.State = "size_changed"
				}
			}
		}
		out.Files = append(out.Files, file)
	}
	out.State = "present_unverified"
	missing := 0
	for _, f := range out.Files {
		switch f.State {
		case "unsafe", "size_changed", "unavailable":
			out.State = f.State
			return out, nil
		case "missing":
			missing++
		}
	}
	if missing == len(out.Files) {
		out.State = "missing"
	} else if missing > 0 {
		out.State = "partial"
	}
	return out, nil
}
