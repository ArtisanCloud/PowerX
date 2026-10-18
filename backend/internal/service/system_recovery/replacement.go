package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

type replacementJournal struct {
	Current    Deployment `json:"current"`
	BackupRoot string     `json:"backup_root"`
	Moved      bool       `json:"moved"`
}

func (n *Native) replacementFile(id string) string {
	return filepath.Join(n.Store.Directory, "replacement-"+id+".json")
}
func (n *Native) ownedEnvironment(d Deployment) error {
	if uuid.Validate(d.ID) != nil || d.Root != filepath.Join(n.Profile.CandidateDirectory, d.ID) || !canonicalNewPath(d.Root) {
		return errors.New("拒绝操作未登记的候选目录或软链接")
	}
	return nil
}
func (n *Native) ReplaceCurrent(ctx context.Context, j *snapshots.ManagedJob, current Deployment) (Deployment, []snapshots.RecoveryCheck, error) {
	if (!n.Profile.Local && !n.Profile.InPlace) || current.SourceManaged {
		return Deployment{}, nil, errors.New("原地覆盖只允许协调器拥有的本地开发实例")
	}
	if e := n.ownedEnvironment(current); e != nil {
		return Deployment{}, nil, e
	}
	raw, e := os.ReadFile(current.ConfigFile)
	if e != nil {
		return Deployment{}, nil, e
	}
	var cfg config.Config
	if yaml.Unmarshal(raw, &cfg) != nil || (!n.Profile.InPlace && cfg.Deployment.Env != "dev") {
		return Deployment{}, nil, errors.New("当前环境不是 dev，已阻断原地覆盖")
	}
	if n.Profile.InPlace && (current.ID != n.Profile.Initial.ID || current.Root != n.Profile.Initial.Root || current.ConfigFile != n.Profile.Initial.ConfigFile) {
		return Deployment{}, nil, errors.New("只能覆盖预先登记的固定目标实例")
	}
	journal := replacementJournal{Current: current, BackupRoot: filepath.Join(n.Profile.CandidateDirectory, "retained-"+j.ID)}
	if _, e = os.Lstat(journal.BackupRoot); !errors.Is(e, os.ErrNotExist) {
		return Deployment{}, nil, errors.New("原地覆盖回退目录已存在")
	}
	if e = snapshots.WriteCoordinatorJSON(n.replacementFile(j.ID), journal); e != nil {
		return Deployment{}, nil, e
	}
	if e = snapshots.WriteCoordinatorJSON(filepath.Join(current.Root, ".before-restore-"+j.ID), map[string]string{"job": j.ID, "instance": current.ID}); e != nil {
		return Deployment{}, nil, e
	}
	if e = StopCandidateProcesses(ctx, current.Root); e != nil {
		return Deployment{}, nil, e
	}
	if e = os.Rename(current.Root, journal.BackupRoot); e != nil {
		return Deployment{}, nil, e
	}
	journal.Moved = true
	if e = snapshots.WriteCoordinatorJSON(n.replacementFile(j.ID), journal); e != nil {
		return Deployment{}, nil, e
	}
	return n.prepare(ctx, j, &current)
}
func (n *Native) RevertReplacement(ctx context.Context, parent snapshots.ManagedJob, archiveID string) error {
	var journal replacementJournal
	if e := snapshots.ReadCoordinatorJSON(n.replacementFile(parent.ID), &journal); errors.Is(e, os.ErrNotExist) {
		return nil
	} else if e != nil {
		return e
	}
	if e := n.ownedEnvironment(journal.Current); e != nil {
		return e
	}
	expected := filepath.Join(n.Profile.CandidateDirectory, "retained-"+parent.ID)
	if journal.BackupRoot != expected || !canonicalNewPath(expected) {
		return errors.New("原地覆盖回退身份不匹配")
	}
	if _, e := os.Lstat(expected); errors.Is(e, os.ErrNotExist) {
		if !journal.Moved {
			return nil
		}
		// Power loss after renaming the original back but before updating the
		// journal must be repeatable. The marker was written before the first
		// move and is never included in snapshots or materialized replacements.
		var marker map[string]string
		if snapshots.ReadCoordinatorJSON(filepath.Join(journal.Current.Root, ".before-restore-"+parent.ID), &marker) == nil && marker["job"] == parent.ID && marker["instance"] == journal.Current.ID {
			journal.Moved = false
			return snapshots.WriteCoordinatorJSON(n.replacementFile(parent.ID), journal)
		}
		return errors.New("原地覆盖的原环境目录缺失")
	}
	if e := StopCandidateProcesses(ctx, journal.Current.Root); e != nil {
		return e
	}
	if _, e := os.Lstat(journal.Current.Root); e == nil {
		retained := filepath.Join(n.Profile.CandidateDirectory, "retained-"+archiveID)
		if _, e = os.Lstat(retained); !errors.Is(e, os.ErrNotExist) {
			return errors.New("当前环境保留目录已经存在")
		}
		if e = os.Rename(journal.Current.Root, retained); e != nil {
			return e
		}
	}
	if e := os.Rename(expected, journal.Current.Root); e != nil {
		return e
	}
	journal.Moved = false
	return snapshots.WriteCoordinatorJSON(n.replacementFile(parent.ID), journal)
}
func (n *Native) DeleteEnvironment(ctx context.Context, d Deployment) error {
	if e := n.ownedEnvironment(d); e != nil {
		return e
	}
	if e := StopCandidateProcesses(ctx, d.Root); e != nil {
		return e
	}
	if e := os.RemoveAll(d.Root); e != nil {
		return e
	}
	jobs, e := n.Store.List()
	if e != nil {
		return e
	}
	parents := map[string]bool{}
	for _, j := range jobs {
		if j.CandidateID == d.ID && (j.Kind == "cutover" || j.Kind == "overwrite") {
			parents[j.ID] = true
		}
	}
	for _, j := range jobs {
		if j.Kind != "rollback" || !parents[j.CandidateID] || j.Status != "complete" {
			continue
		}
		root := filepath.Join(n.Profile.CandidateDirectory, "retained-"+j.ID)
		if !canonicalNewPath(root) {
			return errors.New("保留目录身份变化，拒绝清理")
		}
		if e = StopCandidateProcesses(ctx, root); e != nil {
			return e
		}
		if e = os.RemoveAll(root); e != nil {
			return e
		}
	}
	return nil
}
