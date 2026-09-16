# Cinema Booking System

A concurrent seat-booking app: a Go backend (Redis-backed, single seat per booking) with a React + TypeScript + Vite client. The whole point is the concurrency invariant — for a given movie and seat, only one booking can succeed.

## How to Run

```bash
docker compose up -d redis          # start Redis (also provides redis-commander on :8081)
go run ./cmd                        # Go API on :8080 (loads CLERK_SECRET_KEY from root .env)
cd static/client && pnpm dev        # Vite dev server on :5173 (proxies /movies, /sessions, /admin to :8080)
```

Backend config: `cmd/main.go` loads a root `.env` at startup via `godotenv` (`github.com/joho/godotenv`); shell-exported vars win over the file, and a missing file is fine. `CLERK_SECRET_KEY`, `STRIPE_SECRET_KEY`, and `STRIPE_WEBHOOK_SECRET` are required — the server exits if any is unset. Copy root `.env.example` → `.env` (gitignored).

For live Stripe testing, run `stripe listen --forward-to localhost:8080/stripe/webhook` (prints the `whsec_...` value used as `STRIPE_WEBHOOK_SECRET`) and pay with Stripe's test card `4242 4242 4242 4242`.

Open `http://localhost:5173` in dev. For production, `cd static/client && pnpm build` writes to `static/client/dist`, which the Go server serves at `/` (falls back to the legacy `static/index.html` if not built).

## Verify / Test

```bash
go build ./... && go test ./...     # backend (internal/booking/service_test.go covers the invariant)
cd static/client && pnpm run lint   # eslint
cd static/client && pnpm exec tsc -b # typecheck
cd static/client && pnpm build      # production bundle (must succeed)
```

There are no frontend tests yet — the client has zero test framework installed.

### CI / CD

GitHub Actions runs on every push/PR. **`.github/workflows/ci.yml`** checks Go vet/build/test (with a Redis service container so the concurrency-invariant test runs for real) and client lint/typecheck/build. **`.github/workflows/cd.yml`** builds a multi-stage Docker image (`Dockerfile`) on pushes to `main` and version tags, and pushes to GHCR with branch/tag/sha tags (uses `GITHUB_TOKEN` with `packages: write`). The final image (~30 MB Alpine + Go binary + static/client/dist) needs three env vars at runtime (`CLERK_SECRET_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`) and a Redis instance at `localhost:6379`.

## Architecture

### Backend (Go, module `cinema-booking-system`)
- `cmd/main.go` — server bootstrap: routes, Redis client, service/handler wiring, graceful shutdown.
- `internal/booking/` — the domain.
  - `domain.go` — `Booking` model + `BookingStore` contract.
  - `service.go` — app boundary between handlers and stores.
  - `handler.go` — HTTP handlers (hold / list seats / checkout / release / Stripe webhook / admin confirm / admin cancel). Hold/checkout/release derive the user from the verified token in the request context — no `user_id` in the body. `ListSeats` returns each seat's `session_id` too (used by the admin void flow). Confirm happens in the Stripe webhook handler (online payment) OR `AdminConfirmSeats` (walk-in counter booking).
  - `catalog.go` — in-memory movie catalog + seat layout metadata. `Movie.PriceCents` prices a seat.
  - `redis_store.go` — the real runtime store. Hold TTL is `defaultHoldTTL = 2 * time.Minute`; `checkoutHoldTTL = 30 * time.Minute` is applied when a checkout starts. Other stores (`memory_store.go`, `concurrent_store.go`) exist for learning/experiments.
- `internal/payment/` — `PaymentGateway` interface (checkout creation + webhook parsing) with a real `StripeGateway` and a `FakeGateway` for tests.
- `internal/auth/` — Clerk session-token verification. `auth.Middleware` wraps hold/checkout/release, verifies the `Authorization: Bearer <token>` header against Clerk's JWKS (Clerk Go SDK, `clerkhttp.RequireHeaderAuthorization`), and injects the Clerk user id into the request context (`auth.UserID(ctx)`). `auth.AdminMiddleware(check)` layers an admin check on top (403 for non-admins). The default `auth.ClerkRoleChecker()` resolves admin status from the user's Clerk **public_metadata.role == "admin"** via the Clerk API, cached for 2 minutes (`internal/auth/admin.go`). `auth.WithUser` builds an authenticated context (used by handler tests).
- `internal/adapters/redis/` — Redis client setup.
- `internal/utils/` — JSON/error response helpers.

### Client (`static/client/`, React 19 + Vite + pnpm)
- `src/api/api.ts` — typed fetch client + `ApiError`. Endpoints: GET `/movies`, GET `/movies/{id}/seats`, POST `.../hold`, POST `/sessions/checkout`, DELETE `/sessions/{id}`, POST `/admin/sessions/confirm`, DELETE `/admin/sessions/{id}`. Attaches `Authorization: Bearer <token>` on every request via a `getToken` getter registered with `setTokenGetter`. No `user_id` in request bodies.
- `src/api/queries.ts` — TanStack Query hooks. `useSeats` polls every 5s. Mutations (`useHoldSeat`/`useCreateCheckout`/`useReleaseSession`/`useAdminConfirm`/`useAdminCancel`) invalidate `['seats', movieID]` on success **and** error so the grid stays fresh.
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
- Hold expiry: the backend releases on TTL; the client auto-releases + toasts when a hold's `expiresAt` passes (2-min holds, `HOLDS_MS` in `queries.ts`). When a checkout starts, both sides extend to ~30 min (`checkoutHoldTTL` / `CHECKOUT_HOLD_MS`).
- React-router uses `HashRouter` (hash-based routes) — no server-side routing config needed.
- Dev proxy gotcha: `vite.config.ts` proxies `/movies`, `/sessions`, and `/admin` to :8080 — if you add a new backend path prefix in dev, add it to the proxy too (an unproxied prefix returns Vite's 404, not Go's).
- Theming is hand-rolled CSS variables in `src/index.css` (`--bg`, `--surface`, `--accent`, `--held`, …). No Tailwind/shadcn.
- The `git status` convention: never commit unless the user explicitly asks.

## Open Items (current plan)

### DONE: Stripe pay-to-confirm

`hold → Stripe Checkout → webhook → confirm seat`. Seats are confirmed ONLY in the Stripe webhook handler (`POST /stripe/webhook`) on `checkout.session.completed` — the client never confirms and is never trusted to say "paid".

- `internal/payment/` — `PaymentGateway` interface (`CreateCheckoutSession` + `ParseWebhookEvent`), a real `StripeGateway` (stripe-go v81, hosted Checkout), and a `FakeGateway` for tests. Mirrors the `BookingStore` interface pattern. `WebhookEvent` carries `Type`, `SessionIDs`, `UserID`, and `PaymentStatus`. `StripeGateway.ParseWebhookEvent` verifies via `webhook.ConstructEventWithOptions(..., IgnoreAPIVersionMismatch: true)` (the stripe-go SDK lags the CLI's API version, e.g. `2025-11-17.clover` vs the SDK's `acacia` — without this the signature check fails and confirmation silently no-ops); on failure it falls back to `manualParseWebhookEvent` (a JSON decode that reads `data.object.metadata` for real Stripe envelopes and top-level `metadata` for test/simulated payloads). The `FakeGateway` mirrors that same dual-shape decode. The checkout URL is `https://checkout.stripe.com/...`; the client `window.location.href`s to it.
- New endpoint `POST /sessions/checkout` (auth) takes `{session_ids, success_url, cancel_url}` and creates a Checkout Session with one line item per seat (priced from `Movie.PriceCents`). It does NOT confirm.
- Webhook handler verifies the `Stripe-Signature` header, then calls `ConfirmSeat` per session id. Confirming an already-confirmed session is a no-op (idempotent — safe against Stripe webhook retries).
- Hold TTL is extended to `checkoutHoldTTL = 30 * time.Minute` when checkout starts so the reservation survives payment (client mirrors this with `CHECKOUT_HOLD_MS`). Confirmed bookings are permanent: `Release` on a confirmed session is a no-op.
- `BookingStore` gained `GetSession` and `ExtendHold` (all three stores).
- Client: `useCreateCheckout` replaces `useConfirmSession`; `SeatMap` redirects to Stripe, tracks a pending checkout in `sessionStorage` (`cinema.pendingCheckout`), and shows the success dialog once the polled seat state flips to confirmed+booked.

Test the flow: `go test ./...` includes `internal/booking/payment_test.go` (fake gateway, no Stripe needed). For a live test, run `stripe listen --forward-to localhost:8080/stripe/webhook`, then pay with Stripe's test card `4242 4242 4242 4242`.

### DONE: Admin walk-in bookings (no card)

Staff accounts book seats for walk-in customers who pay at the counter — no Visa, no Stripe. Admin status comes from the Clerk user's **public_metadata** (`{"role": "admin"}`), set in the Clerk dashboard (Users → edit user → Metadata → public_metadata). The client toggles staff mode from `user.publicMetadata?.role`; the backend never trusts the client and re-verifies via the Clerk API (`auth.AdminMiddleware(auth.ClerkRoleChecker())`, cached 2 min, fails closed). No `ADMIN_USER_IDS` env list needed.

- New endpoints (both behind auth + admin check; verified non-admin → 403):
  - `POST /admin/sessions/confirm` — body `{session_ids: [...]}`. Confirms held seats directly (idempotent), reusing the existing ownership-checked `Confirm`. The holder is the staff member's own Clerk account (staff hold seats exactly like customers).
  - `DELETE /admin/sessions/{sessionID}` — voids a booking (held or confirmed) so the seat is bookable again (customer backs out). Implemented as `AdminCancel` on every store.
- `BookingStore` gained `AdminCancel` (all three stores) — unlike `Release` it has no ownership requirement and deletes confirmed bookings too.
- `ListSeats` now also returns each seat's `session_id` so the staff UI can void confirmed seats.
- Client: `SeatMap` detects staff mode via `useUser().publicMetadata.role === "admin"`. Staff see "Confirm (paid at counter)" instead of "Pay & Confirm" (calls `useAdminConfirm`, no Stripe redirect, success dialog shown immediately). Tapping a confirmed/booked seat as staff prompts `window.confirm` then voids it via `useAdminCancel`.
- Tests: `internal/booking/admin_test.go` covers direct confirm, unauthenticated reject, ownership enforcement (admin cannot confirm another user's hold), void confirmed, and 404 paths.

Test: mark a Clerk user as admin in the dashboard, sign in as them, hold a seat and confirm — the seat flips straight to booked. `go test ./...` runs `internal/booking/admin_test.go`.

### NEXT: (todo)

### Later / smaller

- Frontend tests (Vitest + React Testing Library).
- Known backend gap: no "showtime/time-slot" concept — movies are movie+seat only.
- Real-time seat push (WebSockets/SSE) instead of 5s polling (`useSeats`).

## Client Dependencies Notes

- `@clerk/react` v6 (unified Clerk package): exports `ClerkProvider`, `Show` (use `<Show when="signed-in|signed-out">` instead of the older `SignedIn`/`SignedOut` components), `SignInButton`, `SignUpButton`, `UserButton`, `useUser`, `useAuth`, `useClerk` (has `openSignIn()`), `getToken`. `ClerkProvider` reads `VITE_CLERK_PUBLISHABLE_KEY` from env.
- Auth flow today: `userID` (in the session context) = Clerk `user.id`, empty when signed out. Holding a seat requires sign-in (`SeatMap` opens the sign-in modal via `openSignIn()`). Holds are released + cleared when the Clerk identity changes; the `'' -> user.id` transition on page load is intentionally skipped so persisted holds survive refresh.
