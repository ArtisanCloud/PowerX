package metadatax

import "context"

func WithMetadata(ctx context.Context, key any, value any) context.Context {
	return context.WithValue(ctx, key, value)
}

func GetMetadataFromCtx(ctx context.Context, key any) any {
	return ctx.Value(key)
}
