package archive_ops

import (
	"context"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
)

// BindRecoveryAnchor 复用安装维护根，必须在调度器启动前绑定。
func (s *Service) BindRecoveryAnchor(anchor *storagerecovery.Store) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if anchor == nil || s.recoveryAnchor != nil || s.schedulerRunning {
		return failure(503, "ARCHIVE_RECOVERY_UNAVAILABLE", "归档恢复证明不可用")
	}
	s.recoveryAnchor = anchor
	return nil
}
func (s *Service) recoveryEpoch() (string, error) {
	if s.recoveryAnchor == nil {
		return "", nil
	}
	e, e2 := s.recoveryAnchor.Read(s.ctx)
	if e2 != nil {
		return "", failure(503, "ARCHIVE_RECOVERY_UNAVAILABLE", "归档恢复证明不可用")
	}
	return e.Epoch, nil
}

// CheckRecoveryTarget 只核对本机完整验证回执，不发起网络请求或读取到 UI 的密钥。
func (s *Service) CheckRecoveryTarget(ctx context.Context, t Target) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	t = s.targetWithProof(t)
	if !s.targetVerified(t) {
		return failure(409, "ARCHIVE_RECOVERY_TARGET_REQUIRED", "恢复后须重新验证归档目标和凭据")
	}
	return nil
}
