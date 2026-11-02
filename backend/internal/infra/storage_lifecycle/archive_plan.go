package storagelifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"golang.org/x/sys/unix"
	"path/filepath"
	"strings"
	"time"
)

// InspectSealedSource 供归档器复用与回收相同的文件边界；不读取或改写策略。
// 保留每个日志流的最新轮转及 5 分钟稳定窗口，不猜测原生 journal 的活动文件。
func InspectSealedSource(ctx context.Context, s Source) (Preview, error) {
	if !filepath.IsAbs(s.Directory) || s.ID == "" {
		return Preview{}, ErrInvalid
	}
	if s.Kind != "postgresql_logs" && s.Kind != "system_logs" && s.Kind != "audit_logs" && s.Kind != "application_logs" && s.Kind != "journal" {
		return Preview{}, ErrInvalid
	}
	if s.Format != "numeric" && s.Format != "lumberjack" && s.Format != "dated" && s.Format != "journal" && s.Format != "postgresql" {
		return Preview{}, ErrInvalid
	}
	if s.Format == "postgresql" && !validPostgresSource(s) {
		return Preview{}, ErrInvalid
	}
	if len(s.ActiveFiles) > 16 || (s.Format != "journal" && len(s.ActiveFiles) == 0) {
		return Preview{}, ErrInvalid
	}
	for _, name := range s.ActiveFiles {
		if name == "" || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
			return Preview{}, ErrInvalid
		}
	}
	var st unix.Stat_t
	if unix.Lstat(s.Directory, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Mode&0002 != 0 {
		return Preview{}, ErrBlocked
	}
	h := sha256.Sum256([]byte(sourceDefinition(s, st)))
	s.Fingerprint = hex.EncodeToString(h[:])
	s.Supported = true
	e := Engine{opts: Options{Now: time.Now}, sources: map[string]Source{s.ID: s}}
	return e.inspect(ctx, s.ID, Policy{Fingerprint: s.Fingerprint, MaxBytes: 1 << 50})
}
