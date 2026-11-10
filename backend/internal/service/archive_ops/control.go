package archive_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

// auditAction 不保存请求正文或凭据，只记录策略／目标变更和操作者。
func (s *Service) auditAction(ctx context.Context, action, id string, revision int64) error {
	writeErr := writeJSON(filepath.Join(s.cfg.Directory, "audit", time.Now().UTC().Format("20060102T150405.000000000")+"-"+uuid.NewString()+".json"), struct {
		Action   string    `json:"action"`
		ID       string    `json:"id"`
		Revision int64     `json:"revision"`
		Operator uint64    `json:"operator"`
		Trace    string    `json:"trace_id"`
		At       time.Time `json:"at"`
	}{action, id, revision, reqctx.GetUserID(ctx), reqctx.GetTraceID(ctx), time.Now().UTC()})
	if writeErr != nil {
		return writeErr
	}
	entries, err := os.ReadDir(filepath.Join(s.cfg.Directory, "audit"))
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries[:max(0, len(entries)-1000)] {
		if !e.IsDir() {
			_ = os.Remove(filepath.Join(s.cfg.Directory, "audit", e.Name()))
		}
	}
	return nil
}

// transferAllowed 每批对象与提交前复核 UI 权威策略，关闭后不再启动新传输。
func (s *Service) transferAllowed(ctx context.Context, j jobRecord) error {
	if j.Target != nil {
		if e := s.remoteExecutionAllowed(); e != nil {
			return e
		}
	}
	if !s.targetControlRequired() && !isLogSource(j.Kind) && (j.Target == nil || j.Target.Driver != "s3") {
		return nil
	}
	if source, err := s.source(j.SourceID); err == nil && isLogSource(source.Kind) && s.custodyStatus().LegacyDetected && !s.ManagedLegacySource(source.Directory) {
		return errors.New("旧临时任务尚未完成来源接管")
	}
	p, err := s.policy(ctx, j.PolicyID)
	if err != nil {
		return err
	}
	if !p.Enabled || p.Revision != j.PolicyRevision || j.Target == nil || p.TargetID != j.Target.ID {
		return errors.New("归档未开启或策略已改变")
	}
	t, err := s.targetContext(ctx, p.TargetID)
	if err != nil {
		return err
	}
	if !s.targetVerified(t) || t.Revision != j.Target.Revision || t.VerifiedFingerprint != j.Target.VerifiedFingerprint || p.TargetRevision != t.Revision || p.TargetFingerprint != t.VerifiedFingerprint {
		return errors.New("归档目标验证失效或目标已改变")
	}
	return nil
}

// LifecycleArchiveReady 给固定来源的清理器提供启用检查，不接受调用者指定远端路径。
func (s *Service) LifecycleArchiveReady(ctx context.Context, directory string) (bool, error) {
	if s != nil {
		if e := s.requireCommandsDrained(true); e != nil {
			return false, e
		}
	}
	if s == nil || s.initErr != nil || s.ctx.Err() != nil {
		return false, nil
	}
	ids, err := s.policyIDs(ctx)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		p, err := s.policy(ctx, id)
		if err != nil {
			return false, err
		}
		if !p.Enabled || p.TargetID == "" {
			continue
		}
		source, err := s.source(p.SourceID)
		if err != nil || source.Directory != directory || !isLogSource(source.Kind) {
			continue
		}
		t, err := s.target(p.TargetID)
		if err == nil && s.targetVerified(t) && p.TargetRevision == t.Revision && p.TargetFingerprint == t.VerifiedFingerprint {
			return true, nil
		}
	}
	return false, nil
}

// refreshPolicyTarget 只续验已明确启用且配置指纹未变化的目标，不能激活新配置。
func (s *Service) refreshPolicyTarget(ctx context.Context, p Policy) error {
	if p.TargetID == "" {
		return nil
	}
	t, err := s.target(p.TargetID)
	if err != nil {
		return err
	}
	source, err := s.source(p.SourceID)
	if err != nil {
		return err
	}
	if !isLogSource(source.Kind) && t.Driver != "s3" {
		return nil
	}
	fp, err := s.targetFingerprint(t)
	if err != nil || p.TargetRevision != t.Revision || p.TargetFingerprint == "" || p.TargetFingerprint != fp {
		return errors.New("归档目标或凭据已改变，需要在 UI 验证并重新开启策略")
	}
	if !s.targetVerified(t) {
		return s.testTarget(ctx, t.ID)
	}
	return nil
}
