package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/google/uuid"
)

// MigrateArchive 创建独立、受保护的副本；永不删除旧对象或修改其到期时间。
func (s *Service) MigrateArchive(ctx context.Context, id, policy, key, operator string) (Job, error) {
	if err := s.check(ctx); err != nil {
		return Job{}, err
	}
	if !validID(id) || !validID(policy) || !validID(key) {
		return Job{}, failure(400, "ARCHIVE_MIGRATION_INVALID", "请选择历史归档和目标策略，并使用 UUID 请求标识")
	}
	origin, err := s.readJob(id)
	if err != nil {
		return Job{}, err
	}
	if !isLogSource(origin.Kind) || origin.RemoteState != "success" || origin.Target == nil || origin.Target.Driver != "s3" {
		return Job{}, failure(409, "ARCHIVE_MIGRATION_SOURCE_INVALID", "源归档尚未完整校验")
	}
	p, err := s.policy(ctx, policy)
	if err != nil {
		return Job{}, err
	}
	t, err := s.target(p.TargetID)
	if err != nil {
		return Job{}, err
	}
	source, err := s.source(p.SourceID)
	if err != nil {
		return Job{}, err
	}
	if !p.Enabled || t.Driver != "s3" || !s.targetVerified(t) || source.Kind != origin.Kind || origin.Target.ID == t.ID {
		return Job{}, failure(409, "ARCHIVE_MIGRATION_TARGET_INVALID", "请选择同类来源、已验证并已开启的新 S3 目标策略")
	}
	hash := sha256.Sum256([]byte("migration:" + key))
	request := hex.EncodeToString(hash[:])
	jobID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(request)).String()
	unlock, err := s.lock(ctx, "migration-"+jobID)
	if err != nil {
		return Job{}, ErrBusy
	}
	if old, e := s.readJob(jobID); e == nil {
		unlock()
		if old.MigratedFrom != id || old.PolicyID != policy {
			return Job{}, failure(409, "ARCHIVE_IDEMPOTENCY_CONFLICT", "同一请求不能迁移另一归档")
		}
		return s.observedJob(old), nil
	}
	if err = s.catalogCapacity(); err != nil {
		unlock()
		return Job{}, err
	}
	now := time.Now().UTC()
	j := jobRecord{Job: Job{ID: jobID, PolicyID: p.ID, PolicyRevision: p.Revision, SourceID: p.SourceID, Kind: origin.Kind, Status: "running", Phase: "migration", RemoteState: "pending", CreatedAt: now, Target: &t, Protected: true, ProvenanceVerified: origin.ProvenanceVerified, SourceFileName: origin.SourceFileName, FileCount: origin.FileCount, SizeBytes: origin.SizeBytes, RemoteRetentionDays: p.RemoteRetentionDays, RemoteRetentionCount: p.RemoteRetentionCount, Operator: operator, Warnings: []string{"迁移完成后旧目标继续保留；新副本默认保护"}, MigratedFrom: id, OriginManifest: origin.RemotePath, OriginManifestVersion: origin.RemoteManifestVersion, OriginManifestSHA256: origin.RemoteManifestSHA256}, RequestKey: request}
	finishExecution, err := s.beginExecution(&j)
	if err != nil {
		unlock()
		return Job{}, err
	}
	if err = s.saveJob(&j); err != nil {
		finishExecution()
		unlock()
		return Job{}, err
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		finishExecution()
		s.mu.Unlock()
		unlock()
		return Job{}, failure(503, "ARCHIVE_STOPPED", "归档服务已停止")
	}
	s.workers.Add(1)
	s.mu.Unlock()
	response := s.observedJob(j)
	go func() {
		defer finishExecution()
		defer s.workers.Done()
		defer unlock()
		run, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
		defer cancel()
		err := func() error {
			ids := []string{origin.PolicyID, p.ID}
			sort.Strings(ids)
			for i, pid := range ids {
				if i > 0 && ids[i-1] == pid {
					continue
				}
				release, e := s.lock(run, pid)
				if e != nil {
					return e
				}
				defer release()
				blocked, e := s.policyHasInterruptedExcept(run, pid, j.ID)
				if e != nil {
					return e
				}
				if blocked {
					return failure(409, "ARCHIVE_INTERRUPTED_JOB_PENDING", "原策略有中断待核对任务，停止迁移")
				}
			}
			current, e := s.readJob(origin.ID)
			if e != nil {
				return e
			}
			if current.RemoteState != "success" {
				return failure(409, "ARCHIVE_MIGRATION_SOURCE_CHANGED", "源归档状态已变化")
			}
			if e = s.ensureObjects(run, current); e != nil {
				return e
			}
			j.Files = append([]Entry(nil), current.Files...)
			for i := range j.Files {
				j.Files[i].ArchiveKey = ""
				j.Files[i].ArchiveVersion = ""
			}
			if e = s.uploadS3Snapshot(run, &j); e != nil {
				return e
			}
			_, e = s.verifyS3Job(run, j)
			return e
		}()
		j.Status = "success"
		j.RemoteState = "success"
		j.Phase = "completed"
		if err != nil {
			j.Status = "failed"
			j.RemoteState = "unconfirmed"
			j.Error = "迁移未完整确认，旧对象继续保留"
		}
		_ = s.saveJob(&j)
	}()
	s.auditAction(ctx, "migration_started", j.ID, p.Revision)
	return response, nil
}
