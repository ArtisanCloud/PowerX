package storagelifecycle

import "context"

func (e *Engine) legacyBlocked(ctx context.Context, s Source) (bool, error) {
	if e.opts.LegacySourceOwned != nil && e.opts.LegacySourceOwned(ctx, s) {
		return false, nil
	}
	if e.opts.LegacyBlocked != nil {
		return e.opts.LegacyBlocked(ctx)
	}
	return false, nil
}
