package archive_ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CaptureSystemComponent 复用同 MVCC 的媒体／数据库采集，但使用完全独立的对象目录。
// 数据库及已就绪媒体引用共享同一 MVCC；未登记或仍在上传的文件不进入恢复点。
func CaptureSystemComponent(ctx context.Context, directory string, source Source) (Job, error) {
	component := filepath.Join(directory, ".media-component")
	service := NewService(ctx, Config{Directory: component, Instance: "system-snapshot", Sources: []Source{source}}, Options{})
	defer service.Close()
	if service.initErr != nil {
		return Job{}, service.initErr
	}
	j := jobRecord{Job: Job{ID: uuid.NewString(), PolicyID: uuid.NewString(), SourceID: source.ID, Kind: "media", CreatedAt: time.Now().UTC()}}
	if e := service.captureWithMode(ctx, &j, Policy{IncludeDatabase: true}, source, true); e != nil {
		return Job{}, e
	}
	for _, entry := range j.Files {
		if !strings.HasPrefix(entry.Path, "media/") && entry.Path != "database/database.dump" {
			return Job{}, failure(422, "SYSTEM_MEDIA_DRIVER_UNSUPPORTED", "系统快照首版仅支持本地媒体")
		}
		dest := filepath.Join(directory, filepath.FromSlash(entry.Path))
		if e := os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return Job{}, e
		}
		in, e := os.Open(service.objectPath(entry.SHA256))
		if e != nil {
			return Job{}, e
		}
		out, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			in.Close()
			return Job{}, e
		}
		_, e = io.Copy(out, &contextReader{ctx, in})
		in.Close()
		if e == nil {
			e = out.Sync()
		}
		ce := out.Close()
		if e != nil {
			return Job{}, e
		}
		if ce != nil {
			return Job{}, ce
		}
	}
	// 空媒体目录仍有显式清单，不能把未采集误认为“恰好没有文件”。
	if e := os.MkdirAll(filepath.Join(directory, "metadata"), 0700); e != nil {
		return Job{}, e
	}
	if e := writeJSON(filepath.Join(directory, "metadata", "media-inventory.json"), j.Files); e != nil {
		return Job{}, e
	}
	if e := os.RemoveAll(component); e != nil {
		return Job{}, e
	}
	return j.Job, nil
}
