package controller

import (
	"context"

	authsvc "pointswallet/internal/service/auth"
)

type claimsKey struct{}

func WithClaims(ctx context.Context, claims authsvc.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

func ClaimsFromContext(ctx context.Context) (authsvc.Claims, bool) {
	v, ok := ctx.Value(claimsKey{}).(authsvc.Claims)
	return v, ok
}
