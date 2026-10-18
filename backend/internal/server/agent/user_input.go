package agent

import "context"

type originalUserMessageKey struct{}

// WithOriginalUserMessage carries the request's authoritative source text. Call
// only at request ingress or after restoring and validating persisted input,
// never from planner parameters. Nested invocations retain the original source.
func WithOriginalUserMessage(ctx context.Context, message string) context.Context {
	if _, exists := originalUserMessage(ctx); exists {
		return ctx
	}
	return context.WithValue(ctx, originalUserMessageKey{}, message)
}

func originalUserMessage(ctx context.Context) (string, bool) {
	message, ok := ctx.Value(originalUserMessageKey{}).(string)
	return message, ok
}
