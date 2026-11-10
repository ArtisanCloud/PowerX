package archive_ops

import (
	"context"
	"errors"
	"github.com/minio/minio-go/v7"
	"os"
	"sort"
	"time"
)

// ReconcileRemoteGC 只读远端确切版本，核对响应丢失；不会补删剩余对象。
func (s *Service) ReconcileRemoteGC(ctx context.Context, id string) (RemoteGCResult, error) {
	if err := s.check(ctx); err != nil {
		return RemoteGCResult{}, err
	}
	if !validID(id) {
		return RemoteGCResult{}, errors.New("invalid cleanup identity")
	}
	release, err := s.lock(ctx, "gc-"+id)
	if err != nil {
		return RemoteGCResult{}, ErrBusy
	}
	defer release()
	var plan remoteGCPlan
	if err = readJSON(s.path("remote-gc", id), &plan); err != nil {
		return plan.RemoteGCResult, err
	}
	if plan.State == "preview" || plan.State == "completed" || plan.State == "blocked" {
		return plan.RemoteGCResult, nil
	}
	releasePolicy, err := s.lock(ctx, plan.PolicyID)
	if err != nil {
		return plan.RemoteGCResult, ErrBusy
	}
	defer releasePolicy()
	c, err := s.s3Client(plan.Target)
	if err != nil {
		return plan.RemoteGCResult, err
	}
	plan.DeletedVersions = 0
	plan.ConfirmedBytes = 0
	allMissing := true
	for _, v := range plan.Versions {
		obj, e := c.GetObject(ctx, plan.Target.Bucket, v.Key, minio.GetObjectOptions{VersionID: v.VersionID})
		if e == nil {
			obj.Close()
			allMissing = false
			continue
		}
		if !missingS3Config(e, "NoSuchVersion", "NoSuchKey", "NotFound") {
			return plan.RemoteGCResult, errors.New("版本核对失败，保留未确认状态")
		}
		plan.DeletedVersions++
		plan.ConfirmedBytes += v.Size
	}
	plan.State = "unconfirmed"
	plan.Problem = "部分版本仍存在；未自动补删，请保留记录并人工核对引用后处理"
	if allMissing {
		for _, id := range plan.Jobs {
			j, e := s.readJob(id)
			if e != nil {
				return plan.RemoteGCResult, e
			}
			j.Status = "expired"
			j.RemoteState = "expired"
			if e = s.saveJob(&j); e != nil {
				return plan.RemoteGCResult, e
			}
		}
		plan.State = "completed"
		plan.Problem = ""
	}
	if err = writeJSON(s.path("remote-gc", id), plan); err == nil {
		s.auditAction(ctx, "remote_expiry_reconciled", id, plan.PolicyRevision)
	}
	return plan.RemoteGCResult, err
}

func (s *Service) pruneRemoteGCMetadata() {
	ids, err := s.records("remote-gc")
	if err != nil {
		return
	}
	completed := []remoteGCPlan{}
	for _, id := range ids {
		var p remoteGCPlan
		if readJSON(s.path("remote-gc", id), &p) != nil {
			continue
		}
		if p.State == "preview" && time.Now().After(p.ExpiresAt) {
			_ = os.Remove(s.path("remote-gc", id))
		}
		if p.State == "completed" || p.State == "blocked" {
			completed = append(completed, p)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].CreatedAt.After(completed[j].CreatedAt) })
	for i, p := range completed {
		if i >= 100 {
			_ = os.Remove(s.path("remote-gc", p.ID))
		}
	}
	// 未确认运行永不因历史上限而丢失；超过上限时由预览门禁停止生成更多清理任务。
}
