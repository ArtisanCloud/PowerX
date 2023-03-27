package powerx

import (
	"context"
	"github.com/pkg/errors"
)

type MetadataCtx struct {
}

type authMetadataKey struct{}

type AuthMetadata struct {
	UID int64
}

func NewMetadataCtx() *MetadataCtx {
	return &MetadataCtx{}
}

func (m *MetadataCtx) WithAuthMetadataCtxValue(ctx context.Context, md *AuthMetadata) context.Context {
	return context.WithValue(ctx, authMetadataKey{}, md)
}

func (m *MetadataCtx) AuthMetadataFromContext(ctx context.Context) (*AuthMetadata, error) {
	v, ok := ctx.Value(authMetadataKey{}).(*AuthMetadata)
	if !ok {
		return nil, errors.New("无法获取AuthMetadata")
	}
	return v, nil
}
