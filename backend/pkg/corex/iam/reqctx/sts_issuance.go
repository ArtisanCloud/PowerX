package reqctx

import "context"

type stsIssuanceKey struct{}

// WithSTSIssuanceStatus 仅由认证中间件写入：0 为经过 Core 签发记录验证。
func WithSTSIssuanceStatus(ctx context.Context, status int) context.Context {
	return context.WithValue(ctx, stsIssuanceKey{}, status)
}
func STSIssuanceStatus(ctx context.Context) int {
	status, ok := ctx.Value(stsIssuanceKey{}).(int)
	if !ok {
		return 401
	}
	return status
}
