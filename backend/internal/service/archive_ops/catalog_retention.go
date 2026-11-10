package archive_ops

import (
	"context"
	"os"
	"time"
)

// catalogCapacity 限定索引增长；仍有效、受保护或待核对的历史不能为了腾空间消失。
func (s *Service) catalogCapacity() error {
	ids, err := s.records("jobs")
	if err != nil {
		return err
	}
	if len(ids) >= 10000 {
		return failure(409, "ARCHIVE_CATALOG_FULL", "历史目录达到 10000 条上限；请完成到期核对和目录导出，停止新归档，不缩短承诺期限")
	}
	return nil
}
func (s *Service) pruneExpiredCatalog(ctx context.Context) {
	if s.requireCommandsDrained(false) != nil {
		return
	}
	ids, err := s.records("jobs")
	if err != nil {
		return
	}
	pending := map[string]bool{}
	runs, e := s.records("remote-gc")
	if e != nil {
		return
	}
	for _, id := range runs {
		var p remoteGCPlan
		if readJSON(s.path("remote-gc", id), &p) != nil {
			return
		}
		if p.State == "queued" || p.State == "running" || p.State == "unconfirmed" {
			for _, job := range p.Jobs {
				pending[job] = true
			}
		}
	}
	for _, id := range ids {
		if pending[id] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		j, e := s.readJob(id)
		if e != nil || j.RemoteState != "expired" || j.Protected || j.Imported || j.Recovered || time.Since(j.UpdatedAt) < 365*24*time.Hour {
			continue
		}
		release, e := s.lock(ctx, j.PolicyID)
		if e != nil {
			continue
		}
		current, e := s.readJob(id)
		if e == nil && current.RemoteState == "expired" && !current.Protected {
			_ = os.Remove(s.path("jobs", id))
		}
		release()
	}
}
