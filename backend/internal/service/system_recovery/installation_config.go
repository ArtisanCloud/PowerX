package system_recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// writeInstallationConfig durably replaces the single authoritative config.
// It never edits the developer's source config or inherits ambient .env values.
func writeInstallationConfig(file string, cfg *config.Config) (result error) {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), ".config-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), file); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// adoptLocalInstallation upgrades only the owned rehearsal. Existing switched
// environments need explicit legacy recovery first; no source process is stopped.
func adoptLocalInstallation(p *Profile) error {
	if p.InPlace {
		return nil
	}
	var a Active
	err := snapshots.ReadCoordinatorJSON(filepath.Join(p.Directory, "active.json"), &a)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && (a.Maintenance || a.Deployment.ID != p.Initial.ID || a.LastSwitchID != "") {
		return errors.New("旧演练仍有切换或维护现场，请先处理原任务，或用新的演练目录启动；不会擅自迁移或删除环境")
	}
	if p.Initial.SourceManaged || p.Initial.Root != filepath.Join(p.CandidateDirectory, p.Initial.ID) {
		return errors.New("只迁移工具自己创建的本地实例")
	}
	raw, err := os.ReadFile(p.Initial.ConfigFile)
	if err != nil {
		return err
	}
	var cfg config.Config
	if err = yaml.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	file := filepath.Join(p.Initial.Root, "etc/config.yaml")
	if err = writeInstallationConfig(file, &cfg); err != nil {
		return err
	}
	p.Initial.ConfigFile = file
	p.InPlace = true
	for _, target := range p.Targets {
		if target.AllowOverwrite {
			p.Targets = []snapshots.RecoveryTarget{target}
			break
		}
	}
	if err = p.Validate(); err != nil {
		return err
	}
	if err = snapshots.WriteCoordinatorJSON(p.File, p); err != nil {
		return err
	}
	if a.Deployment.ID != "" {
		a.Deployment = p.Initial
		if err = snapshots.WriteCoordinatorJSON(filepath.Join(p.Directory, "active.json"), a); err != nil {
			return err
		}
	}
	return nil
}

// A local developer explicitly rebuilt Core by invoking make dev-recovery.
// Upgrade only an idle legacy rehearsal before introducing the new profile
// format; otherwise its old packaged Core cannot read restore_in_place at all.
// The original snapshot stays unchanged and the override is pinned by SHA.
func upgradeLegacyLocalCore(p *Profile, binary, version string) error {
	if p.InPlace {
		return nil
	}
	var a Active
	err := snapshots.ReadCoordinatorJSON(filepath.Join(p.Directory, "active.json"), &a)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if a.Maintenance || (a.Deployment.ID != "" && a.Deployment.ID != p.Initial.ID) || a.LastSwitchID != "" {
		return errors.New("旧演练存在恢复现场，不能升级程序；请先处理任务或使用新目录")
	}
	store, err := snapshots.NewCoordinatorStore(p.Directory, p.Targets)
	if err != nil {
		return err
	}
	jobs, err := store.List()
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status == "queued" || j.Status == "running" || j.Status == "attention_required" {
			return errors.New("旧演练存在待处理任务，不能迁移；请先处理任务或使用新目录")
		}
	}
	for _, name := range []string{"instance", "postgres"} {
		var record processRecord
		if snapshots.ReadCoordinatorJSON(filepath.Join(p.Initial.Root, "processes", name+".json"), &record) == nil {
			if identity, e := processIdentity(record.PID); e == nil && identity == record.Started {
				return errors.New("旧演练进程仍在运行，不能替换其程序")
			}
		}
	}
	if !p.Local || !canonicalNewPath(p.Initial.Root) || p.Initial.CoreBinary != filepath.Join(p.Initial.Root, "programs/core/powerx") {
		return errors.New("本地程序升级目标不属于受管演练")
	}
	raw, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	artifact := filepath.Join(filepath.Dir(p.File), "local-artifacts", sha, "powerx")
	if err = os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(artifact, raw, 0700); err != nil {
		return err
	}
	if err = os.WriteFile(p.Initial.CoreBinary, raw, 0700); err != nil {
		return err
	}
	override := ArtifactOverride{Path: "core/powerx", Source: artifact, SHA256: sha, Version: version}
	for i := range p.Overrides {
		if p.Overrides[i].Path == override.Path {
			p.Overrides[i] = override
			return nil
		}
	}
	p.Overrides = append(p.Overrides, override)
	return nil
}

func upgradeLegacyLocalWeb(ctx context.Context, p *Profile, web, version string) error {
	if p.InPlace {
		return nil
	}
	if !p.Local || p.Initial.WebOutput != filepath.Join(p.Initial.Root, "programs/core/web-admin") {
		return errors.New("本地 Web 升级目标不属于受管演练")
	}
	artifact := filepath.Join(filepath.Dir(p.File), "local-artifacts", "web-"+uuid.NewString())
	if err := snapshots.CopyLocalProgram(ctx, web, artifact); err != nil {
		return err
	}
	temporary := filepath.Join(p.Initial.Root, "programs/core", ".web-"+uuid.NewString())
	if err := snapshots.CopyLocalProgram(ctx, artifact, temporary); err != nil {
		return err
	}
	old := filepath.Join(filepath.Dir(p.File), "local-artifacts", "old-web-"+uuid.NewString())
	if err := os.Rename(p.Initial.WebOutput, old); err != nil {
		return err
	}
	if err := os.Rename(temporary, p.Initial.WebOutput); err != nil {
		_ = os.Rename(old, p.Initial.WebOutput)
		return err
	}
	retained := p.Overrides[:0]
	for _, o := range p.Overrides {
		if !strings.HasPrefix(o.Path, "core/web-admin/") {
			retained = append(retained, o)
		}
	}
	p.Overrides = retained
	return filepath.WalkDir(artifact, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(artifact, file)
		if err != nil {
			return err
		}
		sha, err := hashFile(file)
		if err != nil {
			return err
		}
		p.Overrides = append(p.Overrides, ArtifactOverride{Path: "core/web-admin/" + filepath.ToSlash(rel), Source: file, SHA256: sha, Version: version})
		return nil
	})
}
