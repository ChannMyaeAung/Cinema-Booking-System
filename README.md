## The Problem

Two users click "Book" on seat A1 at the same instant. Only one should win.

```bash
User A ──► read seat A1 → "free" ──► write booking ──► success
User B ──► read seat A1 → "free" ──► write booking ──► ???
```

Without any protection, both succeed. Now two people show up for the same seat.

## Architecture (Current Design)

This project follows a simple layered architecture centered around booking flows:

- Entrypoint: `cmd/main.go` starts the HTTP server, registers routes, creates the Redis client, wires the service and handler layers, and configures the Clerk SDK.
- Handlers: `internal/booking/handler.go` receives HTTP requests and translates them into booking operations such as hold, list, checkout, webhook confirm, release, admin confirm, and admin cancel. The user id always comes from the verified session token, never from the request body.
- Auth: `internal/auth/middleware.go` wraps hold/checkout/release and verifies the `Authorization: Bearer <token>` header against Clerk's JWKS (Clerk Go SDK v2), injecting the verified Clerk user id into the request context. `internal/auth/admin.go` layers an admin check on top (`auth.AdminMiddleware`): the default `auth.ClerkRoleChecker()` resolves staff status from the Clerk user's `public_metadata.role == "admin"` via the Clerk API (cached 2 min, fails closed → 403 for non-admins).
- Payments: `internal/payment/` is a `PaymentGateway` interface — a real `StripeGateway` (hosted Checkout + signature-verified webhooks) and a `FakeGateway` for tests. Seats are confirmed only on payment success, never by the client.
- Domain: `internal/booking/domain.go` defines the core `Booking` model and the `BookingStore` contract.
- Service: `internal/booking/service.go` acts as the application-layer boundary between handlers and storage.
- Catalog: `internal/booking/catalog.go` provides the in-memory movie catalog and seat layout metadata used to build seat grids.
- Stores: the current runtime path uses a Redis-backed store for persistence, while other stores exist for learning and concurrency experiments.
- Utilities: `internal/utils/utils.go` contains shared helpers for JSON responses and error responses.

The core invariant is: **for a given movie and seat, only one booking can succeed**, even when many users try at the same time.

## Booking Flow

### Online (customer pays by card)

1. The user signs in with Clerk (Google, GitHub, etc.) from the React client.
2. The client calls the hold endpoint for a movie and seat, attaching the Clerk session token as `Authorization: Bearer <token>`.
3. The auth middleware verifies the token against Clerk's JWKS and injects the user id into the request context.
4. The handler builds a booking payload from the verified user id and passes it to the service layer.
5. The service delegates the request to the configured store implementation.
6. The store attempts to create a temporary hold for that seat.
7. If the seat is already taken, the request fails with an already-booked error. If it's free, a temporary booking session is returned with a short TTL.
8. To pay, the client posts the held session ids to `/sessions/checkout`, which creates a Stripe Checkout Session (one line item per seat) and redirects the user to Stripe. The hold TTL is extended to ~30 min so the reservation survives payment.
9. Stripe posts a `checkout.session.completed` webhook to `/stripe/webhook`; the handler verifies the `Stripe-Signature` header and confirms the seats. **Seats are confirmed only here** — the client never confirms and is never trusted to say "paid". Confirming an already-confirmed seat is a no-op (idempotent).
10. The client detects the confirmed state via polling and shows the success screen.

### Walk-in (staff books at the counter, no card)

Staff accounts (Clerk `public_metadata` = `{"role": "admin"}`) hold seats exactly like customers, but instead of a Stripe checkout they confirm the held seats directly at the counter — the customer pays cash/there on the spot.

1. A staff user signs in; the client detects admin from `user.publicMetadata.role`.
2. Staff tap free seats (holds are created under the staff member's own account) and press **"Confirm (paid at counter)"**.
3. `POST /admin/sessions/confirm` (auth + admin check, 403 for non-admins) confirms the held sessions directly — no payment.
4. Staff can also tap a confirmed/booked seat to void it via `DELETE /admin/sessions/{sessionID}`, freeing the seat (e.g. a customer backs out).

## Current Runtime Behavior

The backend is now more than a scaffold:

- The HTTP server and booking endpoints are wired up.
- Redis-backed persistence is part of the real runtime flow.
- The booking service exposes hold, list, checkout, confirm, release, admin-confirm, and admin-cancel operations.
- Seat listing returns a full seat grid for a movie based on catalog metadata and current booking state.
- hold/checkout/release/admin require a valid Clerk session token; the backend derives the user id from it instead of trusting a client-sent `user_id`.
- Two payment/confirmation paths: Stripe Checkout → signature-verified webhook (online), or a staff counter confirm (walk-in). The client is never trusted to say "paid".
- Staff (`public_metadata.role == "admin"`) can void any booking — held or confirmed — via `DELETE /admin/sessions/{sessionID}`.
- Concurrency behavior is exercised through tests and demonstrated by store implementations.

## How to Run

1. Start Redis if needed:

```bash
docker compose up -d redis
```

2. Add your Clerk secret key (from the Clerk dashboard → API Keys → Secret keys) to a root `.env` file. Copy `.env.example` → `.env` and fill it in. The server loads it automatically at startup and exits if it's unset. You can also `export CLERK_SECRET_KEY=sk_test_...` in your shell, which overrides the `.env` value. Stripe keys (`STRIPE_SECRET_KEY` + `STRIPE_WEBHOOK_SECRET`) are also required.

3. Run the server:

```bash
go run ./cmd
```

4. For the full booking flow, run the React client (Vite dev server on :5173, proxies `/movies`, `/sessions`, and `/admin` to :8080):

```bash
cd static/client
pnpm install
pnpm dev
```

The Clerk publishable key lives in `static/client/.env` (copy `.env.example`). Sign in to hold and confirm seats.

### Enabling staff / admin accounts

To allow a user to book at the counter without a card, mark them as staff in the Clerk dashboard:

1. **Users** → click the user → **Metadata** (may be under the "..." → "Edit user" menu, or use the API route directly).
2. Set **public_metadata** to:
   ```json
   { "role": "admin" }
   ```
   You can also set it via the API with your secret key and user id:
   ```bash
   curl -XPATCH \
     -H "Authorization: Bearer $CLERK_SECRET_KEY" \
     -H "Content-Type: application/json" \
     -d '{"public_metadata":{"role":"admin"}}' \
     https://api.clerk.com/v1/users/<USER_ID>/metadata
   ```
3. The staff member signs in normally; the UI detects the role and shows **"Confirm (paid at counter)"** instead of "Pay & Confirm". No server restart is needed (backend re-checks via the Clerk API, cached ~2 min).
