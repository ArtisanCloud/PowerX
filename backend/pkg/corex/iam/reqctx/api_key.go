package reqctx

import "context"

type authenticatedAPIKeyHashKey struct{}

// WithAuthenticatedAPIKeyHash is for authentication middleware only. Never
// populate this value from an unverified header or request payload.
func WithAuthenticatedAPIKeyHash(ctx context.Context, hash string) context.Context {
	return context.WithValue(ctx, authenticatedAPIKeyHashKey{}, hash)
}

func AuthenticatedAPIKeyHash(ctx context.Context) string {
	hash, _ := ctx.Value(authenticatedAPIKeyHashKey{}).(string)
	return hash
}
