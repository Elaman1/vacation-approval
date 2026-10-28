package middleware

import "context"

type userIDKey struct{}

// UserID returns the employee identity set by JWTAuth.
func UserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey{}).(string)
	return userID, ok && userID != ""
}

// WithUserID stores a validated identity in the request context.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}
