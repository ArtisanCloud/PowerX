package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"gorm.io/gorm"
)

func probeControlSnapshot(scope ControlScope, targetID string, pr, tr int64, policies archivePolicyDocument, targets archiveTargetDocument) archive.ProbeControlSnapshot {
	raw, _ := json.Marshal([]any{scope, pr, tr, policies, targets})
	out := archive.ProbeControlSnapshot{Authority: "database", Digest: controlDigest(raw), PolicyRevision: pr, TargetRevision: tr, Policies: []archive.Policy{}}
	if entry, ok := targetEntry(targets, targetID); ok {
		target := effectiveArchiveTarget(entry, scope)
		out.Target = &target
	}
	for _, entry := range policies.Entries {
		if entry.Settings.TargetID == targetID {
			out.Policies = append(out.Policies, effectiveArchivePolicy(entry, scope))
		}
	}
	sort.Slice(out.Policies, func(i, j int) bool { return out.Policies[i].ID < out.Policies[j].ID })
	return out
}
func equalProbeDocuments(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// ObserveProbeCommand 只接受两份完全发布的权威快照；未发布引用不会被当成已暂停。
func (c *archivePolicyControl) ObserveProbeCommand(ctx context.Context, targetID string) (archive.ProbeControlSnapshot, error) {
	pc, pd, pa, e := c.load(ctx)
	if e != nil {
		return archive.ProbeControlSnapshot{}, e
	}
	tc, td, ta, e := (&archiveTargetControl{c.service, c.archives}).load(ctx)
	if e != nil {
		return archive.ProbeControlSnapshot{}, e
	}
	if pc.Authority != "database" || tc.Authority != "database" || !pc.Ready || !tc.Ready || pc.Problem != "" || tc.Problem != "" || pc.AppliedRevision < 1 || tc.AppliedRevision < 1 || pc.DesiredRevision != pc.AppliedRevision || tc.DesiredRevision != tc.AppliedRevision || pc.SnapshotRevision == nil || tc.SnapshotRevision == nil || *pc.SnapshotRevision != pc.AppliedRevision || *tc.SnapshotRevision != tc.AppliedRevision || !equalProbeDocuments(pd, pa) || !equalProbeDocuments(td, ta) {
		return archive.ProbeControlSnapshot{}, controlError(409, "ARCHIVE_PROBE_PUBLICATION_PENDING")
	}
	return probeControlSnapshot(c.service.opts.Control.opts.Scope, targetID, pc.AppliedRevision, tc.AppliedRevision, pd, td), nil
}

// GuardProbeCommand 在同一事务中锁策略→目标，保护完整引用集合，包括新插入的引用。
func (c *archivePolicyControl) GuardProbeCommand(ctx context.Context, targetID string, expected archive.ProbeControlSnapshot, action func(archive.ProbeControlSnapshot) error) error {
	current, e := c.ObserveProbeCommand(ctx, targetID)
	if e != nil {
		return e
	}
	if current.Digest != expected.Digest {
		return controlError(409, "ARCHIVE_PROBE_SCOPE_CHANGED")
	}
	ctrl := c.service.opts.Control
	scope := ctrl.opts.Scope
	return ctrl.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ctrl.WithExecutionTx(ctx, tx, ControlArchivePolicies, func(p *m.ControlConfiguration) error {
			if p == nil || p.Revision != expected.PolicyRevision || p.AppliedRevision != expected.PolicyRevision || p.HostID != scope.HostID || p.FilesystemID != scope.FilesystemID || p.InstanceID != scope.InstanceID {
				return ErrStateUnavailable
			}
			if _, e := verifiedControlDocument(p.Document, p.Digest); e != nil {
				return e
			}
			pd, e := decodeArchivePolicies(p.Document)
			if e != nil {
				return e
			}
			return ctrl.repo.WithCurrentTx(ctx, tx, scope.ID(), string(ControlArchiveTargets), func(t *m.ControlConfiguration) error {
				if t == nil || t.Revision != expected.TargetRevision || t.AppliedRevision != expected.TargetRevision || t.HostID != scope.HostID || t.FilesystemID != scope.FilesystemID || t.InstanceID != scope.InstanceID {
					return ErrStateUnavailable
				}
				if _, e := verifiedControlDocument(t.Document, t.Digest); e != nil {
					return e
				}
				td, e := decodeArchiveTargets(t.Document)
				if e != nil {
					return e
				}
				frozen := probeControlSnapshot(scope, targetID, p.AppliedRevision, t.AppliedRevision, pd, td)
				if frozen.Digest != expected.Digest {
					return controlError(409, "ARCHIVE_PROBE_SCOPE_CHANGED")
				}
				for _, policy := range frozen.Policies {
					if policy.Enabled || policy.RemoteCleanupEnabled {
						return controlError(409, "ARCHIVE_PAUSE_REQUIRED")
					}
				}
				return action(frozen)
			})
		})
	})
}
