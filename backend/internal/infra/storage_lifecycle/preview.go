package storagelifecycle

import "context"

// PreviewPolicy 可测算尚未保存的目标，不写策略、执行账本或文件。
func (e *Engine) PreviewPolicy(ctx context.Context, id string, p Policy) (Preview, error) {
	s, ok := e.sources[id]
	if !ok || !s.Supported || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.MaxBytes < 1 || p.MaxBytes > 1<<50 {
		return Preview{}, ErrInvalid
	}
	d, err := e.load(ctx)
	if err != nil {
		return Preview{}, err
	}
	p.Revision = d.Policies[id].Revision
	p.SourceID = id
	p.Fingerprint = s.Fingerprint
	p.RequireArchive = p.RequireArchive || s.RequireArchive
	return e.inspect(ctx, id, p)
}
