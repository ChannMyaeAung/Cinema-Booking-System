# Cinema Booking System

A concurrent seat-booking app: a Go backend (Redis-backed, single seat per booking) with a React + TypeScript + Vite client. The whole point is the concurrency invariant — for a given movie and seat, only one booking can succeed.

## How to Run

```bash
docker compose up -d redis          # start Redis (also provides redis-commander on :8081)
go run ./cmd                        # Go API on :8080
cd static/client && pnpm dev        # Vite dev server on :5173 (proxies /movies, /sessions to :8080)
```

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
  - `handler.go` — HTTP handlers (hold / list seats / confirm / release).
  - `catalog.go` — in-memory movie catalog + seat layout metadata.
  - `redis_store.go` — the real runtime store. Hold TTL is `defaultHoldTTL = 2 * time.Minute`. Other stores (`memory_store.go`, `concurrent_store.go`) exist for learning/experiments.
- `internal/adapters/redis/` — Redis client setup.
- `internal/utils/` — JSON/error response helpers.

### Client (`static/client/`, React 19 + Vite + pnpm)
- `src/api/api.ts` — typed fetch client + `ApiError`. Endpoints: GET `/movies`, GET `/movies/{id}/seats`, POST `.../hold`, PUT `/sessions/{id}/confirm`, DELETE `/sessions/{id}`.
- `src/api/queries.ts` — TanStack Query hooks. `useSeats` polls every 5s. Mutations (`useHoldSeat`/`useConfirmSession`/`useReleaseSession`) invalidate `['seats', movieID]` on success **and** error so the grid stays fresh.
- `src/session.tsx` / `src/session-context.ts` — owns **local** hold state only (user_id + active holds in localStorage, expiry cleanup). Server calls live in the mutation layer, NOT here.
- `src/pages/Home.tsx` — movie catalog grid via `useMovies`.
- `src/pages/SeatMap.tsx` — the money screen: seat grid, 4 states (available/selected/held/booked), hold → countdown → confirm, sticky checkout bar, success dialog.
- `src/toast.tsx` / `src/toast-context.ts` — snackbar feedback (hand-rolled, no shadcn/ui yet).
- `static/client/index.html` — page title is `CineBook`; the built app is served from `static/client/dist`.

## Conventions & Gotchas

- Seat state logic lives in `SeatMap.tsx:getState`. A seat is `selected` when it's in the user's local holds, matches their user_id, or a hold is pending for it.
- Two tabs in one browser share localStorage, so they share a `user_id` — they will NOT appear as different users. Test concurrency with an incognito window or a second browser.
- Hold expiry: the backend releases on TTL; the client auto-releases + toasts when a hold's `expiresAt` passes (2-min holds, `HOLDS_MS` in `queries.ts`).
- React-router uses `HashRouter` (hash-based routes) — no server-side routing config needed.
- Theming is hand-rolled CSS variables in `src/index.css` (`--bg`, `--surface`, `--accent`, `--held`, …). No Tailwind/shadcn.
- The `git status` convention: never commit unless the user explicitly asks.

## Open Items (current plan)

- Dev "switch user" button (reset localStorage user_id to test concurrency in tabs).
- Optimistic seat selection with rollback (`onMutate` in `useHoldSeat`).
- Frontend tests (Vitest + React Testing Library).
- Known backend gap: no "showtime/time-slot" concept — movies are movie+seat only.
