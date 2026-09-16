package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"cinema-booking-system/internal/utils"

	"github.com/clerk/clerk-sdk-go/v2/user"
)

// RoleChecker reports whether a user has staff/admin privileges.
type RoleChecker interface {
	IsAdmin(ctx context.Context, userID string) bool
}

// adminCacheTTL bounds how long an admin decision is reused before it is
// re-checked against Clerk, so promoting/revoking staff takes effect quickly.
const adminCacheTTL = 2 * time.Minute

type adminCacheEntry struct {
	isAdmin bool
	at      time.Time
}

var (
	adminMu    sync.Mutex
	adminCache = map[string]adminCacheEntry{}
)

// clerkRoleChecker determines admin status from the user's Clerk public
// metadata: public_metadata.role == "admin". Mark staff accounts as admin in
// the Clerk dashboard (Users → edit user → Metadata → public_metadata).
type clerkRoleChecker struct{}

// ClerkRoleChecker returns a RoleChecker backed by the Clerk API.
func ClerkRoleChecker() RoleChecker {
	return &clerkRoleChecker{}
}

// IsAdmin reports whether the user's Clerk public metadata marks them as an
// admin. It fails closed (any lookup error means "not admin") and caches the
// result briefly to avoid hammering the Clerk API on every admin request.
func (c *clerkRoleChecker) IsAdmin(ctx context.Context, userID string) bool {
	if userID == "" {
		return false
	}

	adminMu.Lock()
	entry, ok := adminCache[userID]
	adminMu.Unlock()
	if ok && time.Since(entry.at) < adminCacheTTL {
		return entry.isAdmin
	}

	clerkUser, err := user.Get(ctx, userID)
	if err != nil || clerkUser == nil {
		return false // fail closed: can't verify, so no admin access
	}

	var meta struct {
		Role string `json:"role"`
	}
	if len(clerkUser.PublicMetadata) > 0 {
		_ = json.Unmarshal(clerkUser.PublicMetadata, &meta)
	}

	isAdmin := meta.Role == "admin"
	adminMu.Lock()
	adminCache[userID] = adminCacheEntry{isAdmin: isAdmin, at: time.Now()}
	adminMu.Unlock()
	return isAdmin
}

// AdminMiddleware requires a valid Clerk session AND staff/admin role. A
// missing/invalid token is a 401; a verified non-admin user is a 403. Downstream
// handlers receive the authenticated user id via UserID(ctx).
func AdminMiddleware(check RoleChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := UserID(r.Context())
			if userID == "" || !check.IsAdmin(r.Context(), userID) {
				utils.WriteError(w, http.StatusForbidden, "admin access required")
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}