package plugininstall

import (
	"context"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	"time"
)

// Discard only removes private upload staging. It never uninstalls a plugin or
// deletes a queued/running execution. Terminal task receipts remain available.
func (s *Service) Discard(ctx context.Context, id string) (View, error) {
	row, e := s.owned(ctx, id)
	if e != nil {
		return View{}, e
	}
	if e = privateDirectory(s.opts.Root); e != nil {
		return View{}, e
	}
	dir := s.taskDirectory(id)
	if e = privateDirectory(dir); e != nil {
		return View{}, e
	}
	unlock, e := fileLock(ctx, dir+"/upload.lock")
	if e != nil {
		return View{}, e
	}
	defer unlock()
	e = s.repo.Update(ctx, nil, row.TenantUUID, id, func(current *m.Installation) error {
		if current.Status != "awaiting_upload" && !terminal(current.Status) {
			return problem(409, "PLUGIN_INSTALL_TASK_ACTIVE")
		}
		if current.Status == "awaiting_upload" {
			current.Status = "cancelled"
			current.Phase = "completed"
			now := time.Now().UTC()
			current.CompletedAt = &now
			current.Revision++
		}
		*row = *current
		return nil
	})
	if e != nil {
		return View{}, e
	}
	if e = removeOwnedSource(dir); e != nil {
		return View{}, e
	}
	s.changed(ctx, row)
	return view(row), nil
}
