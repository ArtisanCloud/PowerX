package archive_ops

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"time"
)

func isLogSource(kind string) bool {
	return kind == "postgresql_logs" || kind == "system_logs" || kind == "audit_logs" || kind == "application_logs" || kind == "journal"
}
func sealedLogPlan(ctx context.Context, s Source) (lifecycle.Preview, error) {
	var st unix.Stat_t
	if unix.Lstat(s.Directory, &st) != nil {
		return lifecycle.Preview{}, errors.New("日志来源目录不可用")
	}
	owner := st.Uid
	if s.FileOwnerUID != nil {
		owner = *s.FileOwnerUID
	}
	return lifecycle.InspectSealedSource(ctx, lifecycle.Source{ID: s.ID, Kind: s.Kind, Directory: s.Directory, Format: s.RotationFormat, ActiveFiles: s.ActiveFiles, OwnerUID: owner, CurrentLogfiles: s.CurrentLogfiles})
}
func (s *Service) captureSealedLogs(ctx context.Context, j *jobRecord, source Source, entries map[string]Entry) error {
	plan, err := sealedLogPlan(ctx, source)
	if err != nil || !plan.Complete {
		return errors.New("无法完整识别封存日志，拒绝猜测活动文件")
	}
	for _, c := range plan.Candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		path := filepath.Join(source.Directory, c.Name)
		var before unix.Stat_t
		if unix.Lstat(path, &before) != nil || before.Ino != c.Inode || uint64(before.Dev) != c.Device || before.Mtim.Nano() != c.ModifiedNS || before.Ctim.Nano() != c.ChangedNS {
			return errors.New("封存日志身份变化，请重试")
		}
		name := source.Kind + "/" + c.Name
		entry, added, err := s.captureFile(ctx, path, name)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil || info.Size() != c.Bytes || !info.ModTime().Equal(time.Unix(0, c.ModifiedNS)) {
			return errors.New("封存日志在归档中变化")
		}
		entry.SourceSnapshot = sealedSnapshot(source, c)
		entries[name] = entry
		j.NewBytes += added
	}
	if source.RotationFormat == "postgresql" {
		// A collector rotation/reopen during capture must not publish a snapshot
		// containing a file that has become active or the latest sealed segment.
		current, err := sealedLogPlan(ctx, source)
		if err != nil || !current.Complete {
			return errors.New("PostgreSQL 活动日志清单在归档中不可确认")
		}
		sealed := map[string]string{}
		for _, c := range current.Candidates {
			sealed[source.Kind+"/"+c.Name] = sealedSnapshot(source, c)
		}
		for name, entry := range entries {
			if sealed[name] != entry.SourceSnapshot {
				return errors.New("PostgreSQL 日志在归档中轮转或重新打开")
			}
		}
	}
	if len(entries) == 0 {
		return errors.New("没有可归档的稳定封存日志，最新轮转始终保留")
	}
	return nil
}

func sealedSnapshot(source Source, c lifecycle.Candidate) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d:%d:%d:%d:%d", source.ID, source.Directory, source.Kind, c.Device, c.Inode, c.Bytes, c.ModifiedNS, c.ChangedNS))))
}
func (s *Service) sealedUnchanged(ctx context.Context, source Source, last jobRecord, p Policy) (bool, error) {
	if !isLogSource(source.Kind) || source.RotationFormat == "" {
		return false, nil
	}
	plan, err := sealedLogPlan(ctx, source)
	if err != nil || !plan.Complete {
		return false, errors.New("封存日志扫描未完成")
	}
	if len(plan.Candidates) == 0 {
		return true, nil
	}
	if last.RemoteState != "success" || last.Imported || last.Recovered || last.PolicyRevision != p.Revision || len(last.Files) != len(plan.Candidates) {
		return false, nil
	}
	previous := map[string]string{}
	for _, f := range last.Files {
		previous[f.Path] = f.SourceSnapshot
	}
	for _, c := range plan.Candidates {
		if previous[source.Kind+"/"+c.Name] != sealedSnapshot(source, c) {
			return false, nil
		}
	}
	return true, nil
}
