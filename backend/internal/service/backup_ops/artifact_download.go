package backup_ops

import (
	"context"
	"fmt"
	"io"
	"os"

	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"golang.org/x/sys/unix"
)

// OpenDownload 返回已核验且持有保留锁的文件；HTTP 必须在发送完成后调用 release。
func (s *JobService) OpenDownload(ctx context.Context, id uint64, operator string) (*os.File, func(), error) {
	if err := requireBackupExportRoot(ctx); err != nil {
		return nil, nil, err
	}
	job, err := s.GetJob(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if job.Status != "success" {
		return nil, nil, ErrInvalidBackupRequest
	}
	unlock, err := lockBackupPolicy(ctx, s.db, job.PolicyID)
	if err != nil {
		return nil, nil, err
	}
	fail := func(err error) (*os.File, func(), error) { unlock(); return nil, nil, err }
	artifact, err := s.artifactRepo.GetLatestByJobID(ctx, id)
	if err != nil {
		return fail(err)
	}
	if artifact == nil {
		return fail(restoreFailure(410, "BACKUP_FILE_UNAVAILABLE", "备份文件已清理或不可用，请重新备份"))
	}
	path, err := validatedArtifactPath(s.artifactBaseDir, artifact)
	if err != nil {
		if os.IsNotExist(err) {
			return fail(restoreFailure(410, "BACKUP_FILE_UNAVAILABLE", "备份文件已清理或不可用，请重新备份"))
		}
		return fail(restoreFailure(422, "BACKUP_FILE_INVALID", "备份文件路径校验失败，已停止下载"))
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return fail(err)
	}
	f := os.NewFile(uintptr(fd), path)
	if err = verifyOpenArtifactFile(f, artifact); err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return fail(restoreFailure(422, "BACKUP_FILE_INVALID", "备份文件大小或 SHA256 校验失败，请重新备份"))
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_job", ResourceID: fmt.Sprint(id), Operation: "download", Outcome: "success", Severity: "info", Detail: map[string]any{"operator": normalizeOperator(operator), "trace_id": reqctx.GetTraceID(ctx)}})
	return f, func() { f.Close(); unlock() }, nil
}

func requireBackupExportRoot(ctx context.Context) error {
	if !reqctx.IsRoot(ctx) {
		return restoreFailure(403, "BACKUP_ROOT_REQUIRED", "完整数据库导出仅允许 Root 管理员")
	}
	if claims := reqctx.GetClaims(ctx); claims != nil {
		if claims.Issuer == "powerx-sts" {
			return restoreFailure(403, "BACKUP_USER_REQUIRED", "导出需要管理员用户会话")
		}
		for _, plane := range claims.Platforms {
			if plane == "api_key" {
				return restoreFailure(403, "BACKUP_USER_REQUIRED", "API Key 不能下载整库备份")
			}
		}
	}
	return nil
}
