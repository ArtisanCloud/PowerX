package snapshot_ops

import (
	"path/filepath"
)

// recoverInterrupted 只在执行锁空闲时标记旧进程的未完成任务，不接管另一活跃进程。
func (s *Service) recoverInterrupted() error {
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "operation.lock"))
	if e != nil {
		return nil
	}
	defer unlock()
	ids, e := s.ids("snapshots")
	if e != nil {
		return e
	}
	for _, id := range ids {
		j, e := s.snapshot(id)
		if e != nil {
			return e
		}
		if j.Status == "queued" || j.Status == "capturing" || j.Status == "verifying" {
			j.Status = "interrupted"
			j.Phase = "interrupted"
			j.ErrorCode = "SNAPSHOT_INTERRUPTED"
			j.Error = "采集进程已中断，请重新创建快照"
			j.UpdatedAt = s.opts.Now()
			if e = atomicJSON(s.record("snapshots", id), j); e != nil {
				return e
			}
		}
	}
	ids, e = s.ids("restores")
	if e != nil {
		return e
	}
	for _, id := range ids {
		var r Restore
		if e = readJSON(s.record("restores", id), &r); e != nil {
			return e
		}
		if r.Status == "queued" || r.Status == "running" {
			r.Status = "interrupted"
			r.Phase = "interrupted"
			r.Error = "恢复进程已中断；独立候选数据保留供核对"
			r.UpdatedAt = s.opts.Now()
			if e = atomicJSON(s.record("restores", id), r); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Service) findCaptureRequest(key string) (Snapshot, bool, error) {
	ids, e := s.ids("snapshots")
	if e != nil {
		return Snapshot{}, false, e
	}
	hash := digest([]byte("capture:" + key))
	for _, id := range ids {
		j, e := s.snapshot(id)
		if e != nil {
			return Snapshot{}, false, e
		}
		if j.Imported {
			continue
		}
		var token struct {
			Hash string `json:"hash"`
		}
		if e = readJSON(s.record("requests", id), &token); e != nil {
			if j.Status == "pruned" {
				continue
			}
			return Snapshot{}, false, e
		}
		if token.Hash == hash {
			return j, true, nil
		}
	}
	return Snapshot{}, false, nil
}

func (s *Service) findRestoreRequest(id, key string) (Restore, bool, error) {
	ids, e := s.ids("restores")
	if e != nil {
		return Restore{}, false, e
	}
	hash := digest([]byte("restore:" + id + ":" + key))
	for _, rid := range ids {
		var r Restore
		if e = readJSON(s.record("restores", rid), &r); e != nil {
			return r, false, e
		}
		if r.Directory != filepath.Join(s.cfg.Directory, "restores", r.ID) {
			return r, false, failure(503, "SNAPSHOT_STORE_CORRUPT", "恢复目录身份不符")
		}
		var token struct {
			Hash string `json:"hash"`
		}
		if e = readJSON(filepath.Join(r.Directory, "request.json"), &token); e != nil {
			return r, false, e
		}
		if token.Hash == hash {
			return r, true, nil
		}
	}
	return Restore{}, false, nil
}
