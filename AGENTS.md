# Cinema Booking System

A concurrent seat-booking app: a Go backend (Redis-backed, single seat per booking) with a React + TypeScript + Vite client. The whole point is the concurrency invariant — for a given movie and seat, only one booking can succeed.

## How to Run

```bash
docker compose up -d redis          # start Redis (also provides redis-commander on :8081)
go run ./cmd                        # Go API on :8080 (loads CLERK_SECRET_KEY from root .env)
cd static/client && pnpm dev        # Vite dev server on :5173 (proxies /movies, /sessions to :8080)
```

Backend config: `cmd/main.go` loads a root `.env` at startup via `godotenv` (`github.com/joho/godotenv`); shell-exported vars win over the file, and a missing file is fine. `CLERK_SECRET_KEY` (from the Clerk dashboard) is required — the server exits if it's unset. Copy root `.env.example` → `.env` (gitignored).

Open `http://localhost:5173` in dev. For production, `cd static/client && pnpm build` writes to `static/client/dist`, which the Go server serves at `/` (falls back to the legacy `static/index.html` if not built).

## Verify / Test

```bash
go build ./... && go test ./...     # backend (internal/booking/service_test.go covers the invariant)
cd static/client && pnpm run lint   # eslint
cd static/client && pnpm exec tsc -b # typecheck
cd static/client && pnpm build      # production bundle (must succeed)
```

There are no frontend tests yet — the client has zero test framework installed.

## Architecture

### Backend (Go, module `cinema-booking-system`)
- `cmd/main.go` — server bootstrap: routes, Redis client, service/handler wiring, graceful shutdown.
- `internal/booking/` — the domain.
  - `domain.go` — `Booking` model + `BookingStore` contract.
  - `service.go` — app boundary between handlers and stores.
  - `handler.go` — HTTP handlers (hold / list seats / confirm / release). Hold/confirm/release derive the user from the verified token in the request context — no `user_id` in the body.
  - `catalog.go` — in-memory movie catalog + seat layout metadata.
  - `redis_store.go` — the real runtime store. Hold TTL is `defaultHoldTTL = 2 * time.Minute`. Other stores (`memory_store.go`, `concurrent_store.go`) exist for learning/experiments.
- `internal/auth/` — Clerk session-token verification. `auth.Middleware` wraps hold/confirm/release, verifies the `Authorization: Bearer <token>` header against Clerk's JWKS (Clerk Go SDK, `clerkhttp.RequireHeaderAuthorization`), and injects the Clerk user id into the request context (`auth.UserID(ctx)`).
- `internal/adapters/redis/` — Redis client setup.
- `internal/utils/` — JSON/error response helpers.

### Client (`static/client/`, React 19 + Vite + pnpm)
- `src/api/api.ts` — typed fetch client + `ApiError`. Endpoints: GET `/movies`, GET `/movies/{id}/seats`, POST `.../hold`, PUT `/sessions/{id}/confirm`, DELETE `/sessions/{id}`. Attaches `Authorization: Bearer <token>` on every request via a `getToken` getter registered with `setTokenGetter`. No `user_id` in request bodies.
- `src/api/queries.ts` — TanStack Query hooks. `useSeats` polls every 5s. Mutations (`useHoldSeat`/`useConfirmSession`/`useReleaseSession`) invalidate `['seats', movieID]` on success **and** error so the grid stays fresh.
- `src/session.tsx` / `src/session-context.ts` — owns **local** hold state only (active holds in localStorage, expiry cleanup). Registers Clerk's `getToken()` with the api client so the backend can verify identity. Identity comes from Clerk (`useUser()` → `user.id`); empty when signed out. Holds are released + cleared when the identity changes.
- `src/pages/Home.tsx` — movie catalog grid via `useMovies`.
- `src/pages/SeatMap.tsx` — the money screen: seat grid, 4 states (available/selected/held/booked), hold → countdown → confirm, sticky checkout bar, success dialog.
- `src/toast.tsx` / `src/toast-context.ts` — snackbar feedback (hand-rolled, no shadcn/ui yet).
- `static/client/index.html` — page title is `CineBook`; the built app is served from `static/client/dist`.

## Conventions & Gotchas

- Seat state logic lives in `SeatMap.tsx:getState`. A seat is `selected` when it's in the user's local holds, matches their user_id, or a hold is pending for it.
- Identity: the user id is the Clerk user id. The backend derives it from the verified session token (`auth.UserID(ctx)`) — the client never sends `user_id` in a body. Booking (holding a seat) requires sign-in — `SeatMap` calls `openSignIn()` when a signed-out user taps a seat. The header shows Clerk's SignIn/SignUp/UserButton controls.
- Env: the Clerk publishable key lives in `static/client/.env` (gitignored; copy `.env.example`). Vite needs a restart to pick up new env values. The Go backend reads `CLERK_SECRET_KEY` from the root `.env` (gitignored; copy `.env.example`), loaded by `godotenv` in `cmd/main.go`; a shell-exported var overrides the file. The server exits if the key is unset.
- Two tabs in one browser share localStorage, so they share a `user_id` — they will NOT appear as different users. Test concurrency with an incognito window or a second browser.
- Hold expiry: the backend releases on TTL; the client auto-releases + toasts when a hold's `expiresAt` passes (2-min holds, `HOLDS_MS` in `queries.ts`).
- React-router uses `HashRouter` (hash-based routes) — no server-side routing config needed.
- Theming is hand-rolled CSS variables in `src/index.css` (`--bg`, `--surface`, `--accent`, `--held`, …). No Tailwind/shadcn.
- The `git status` convention: never commit unless the user explicitly asks.

## Open Items (current plan)

### DONE: Backend now verifies Clerk session tokens

The Go backend no longer trusts `user_id` in the request body. `internal/auth` wraps hold/confirm/release and verifies the `Authorization: Bearer <token>` header against Clerk's JWKS (Clerk Go SDK v2, `clerkhttp.RequireHeaderAuthorization`), then injects the verified Clerk user id into the request context (`auth.UserID(ctx)`). The client registers Clerk's `getToken()` with the api client in `session.tsx`, so every request carries the token. `user_id` was removed from all request bodies.

### NEXT: Stripe pay-to-confirm

- Pay-to-confirm flow: `hold → payment → webhook → confirm seat`. Confirm seats ONLY in the Stripe webhook handler (never trust the client saying "paid").
- Consider extending the 2-min hold TTL (`defaultHoldTTL` in `internal/booking/redis_store.go`) or pausing the countdown once checkout starts, since payment adds time to the flow.
- Idempotency: confirming an already-confirmed session must be a no-op.

### Later / smaller

- Frontend tests (Vitest + React Testing Library).
- Known backend gap: no "showtime/time-slot" concept — movies are movie+seat only.
- Design the Stripe logic behind a `PaymentGateway` interface with a fake implementation so the lifecycle is testable without Stripe (mirrors the existing `BookingStore` interface pattern).

## Client Dependencies Notes

- `@clerk/react` v6 (unified Clerk package): exports `ClerkProvider`, `Show` (use `<Show when="signed-in|signed-out">` instead of the older `SignedIn`/`SignedOut` components), `SignInButton`, `SignUpButton`, `UserButton`, `useUser`, `useAuth`, `useClerk` (has `openSignIn()`), `getToken`. `ClerkProvider` reads `VITE_CLERK_PUBLISHABLE_KEY` from env.
- Auth flow today: `userID` (in the session context) = Clerk `user.id`, empty when signed out. Holding a seat requires sign-in (`SeatMap` opens the sign-in modal via `openSignIn()`). Holds are released + cleared when the Clerk identity changes; the `'' -> user.id` transition on page load is intentionally skipped so persisted holds survive refresh.
