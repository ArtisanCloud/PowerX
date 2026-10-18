package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

// A rehearsal never publishes source.enc into the daily site's snapshot store.
// Ready packages migrate through the verified export/import contract; originals
// and failed historical source records remain at their original location.
func isolateLocalSnapshotStorage(ctx context.Context, p *Profile) error {
	target := filepath.Join(filepath.Dir(p.File), "snapshots")
	if p.SnapshotDirectory == target {
		return nil
	}
	if !p.Local {
		return errors.New("只迁移本地演练快照目录")
	}
	source := snapshots.NewService(ctx, snapshots.Config{Directory: p.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	defer source.Close()
	dest := snapshots.NewService(ctx, snapshots.Config{Directory: target}, snapshots.Options{Coordinator: true})
	defer dest.Close()
	view, err := source.Overview(privileged(ctx))
	if err != nil {
		return err
	}
	for _, snapshot := range view.Snapshots {
		if snapshot.Status != "ready" {
			continue
		}
		if existing, e := dest.Get(privileged(ctx), snapshot.ID); e == nil && existing.Status == "ready" {
			continue
		}
		password := token()
		file, cleanup, e := source.ExportFile(privileged(ctx), snapshot.ID, password)
		if e != nil {
			return e
		}
		f, e := os.Open(file)
		if e != nil {
			cleanup()
			return e
		}
		_, e = dest.Import(privileged(ctx), f, password)
		_ = f.Close()
		cleanup()
		if e != nil {
			return e
		}
	}
	for _, name := range []string{"policies.json", "policy.json"} {
		file := filepath.Join(p.SnapshotDirectory, name)
		if _, e := os.Lstat(file); errors.Is(e, os.ErrNotExist) {
			continue
		}
		var policy map[string]any
		if e := snapshots.ReadCoordinatorJSON(file, &policy); e != nil {
			return e
		}
		if e := snapshots.WriteCoordinatorJSON(filepath.Join(target, name), policy); e != nil {
			return e
		}
	}
	if _, e := dest.Overview(privileged(ctx)); e != nil {
		return e
	}
	p.SnapshotDirectory = target
	fmt.Println("本地演练使用独立快照目录；可用旧快照已复制，原备份包保留。")
	return nil
}
