package auth

import (
	"context"
	"net/http"

	"cinema-booking-system/internal/utils"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
)

type userIDKey struct{}

// Middleware protects a handler by requiring a valid Clerk session token in
// the Authorization header (Bearer <token>). The authenticated user id is
// verified and injected into the request context for downstream handlers.
func Middleware(next http.Handler) http.Handler {
	return clerkhttp.RequireHeaderAuthorization()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := clerk.SessionClaimsFromContext(r.Context())
		if !ok || claims == nil || claims.Subject == "" {
			utils.WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey{}, claims.Subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	}))
}

// UserID returns the authenticated user id carried in the request context,
// or the empty string if the request was not authenticated.
func UserID(ctx context.Context) string {
	v, ok := ctx.Value(userIDKey{}).(string)
	if !ok {
		return ""
	}
	return v
}

// WithUser returns a copy of ctx carrying the given authenticated user id.
// It mirrors what Middleware does internally and exists so handlers (and
// handler tests) can build an authenticated context without a real token.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}
