package backup_ops

import (
	"context"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"go.uber.org/zap"
)

func normalizeOperator(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "system"
	}
	return v
}

func logOp(ctx context.Context, level string, msg string, fields ...zap.Field) {
	if level == "error" {
		logger.Error(ctx, msg, fields...)
		return
	}
	logger.Info(ctx, msg, fields...)
}
