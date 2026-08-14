package domain

import "context"

type holderCtxKey struct{}

func WithHolder(ctx context.Context, holderID string) context.Context {
	if holderID == "" {
		return ctx
	}
	return context.WithValue(ctx, holderCtxKey{}, holderID)
}

func HolderIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(holderCtxKey{}).(string)
	return s
}
