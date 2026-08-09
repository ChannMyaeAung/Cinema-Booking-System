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
- Handlers: `internal/booking/handler.go` receives HTTP requests and translates them into booking operations such as hold, list, confirm, and release. The user id always comes from the verified session token, never from the request body.
- Auth: `internal/auth/middleware.go` wraps hold/confirm/release and verifies the `Authorization: Bearer <token>` header against Clerk's JWKS (Clerk Go SDK v2), injecting the verified Clerk user id into the request context.
- Domain: `internal/booking/domain.go` defines the core `Booking` model and the `BookingStore` contract.
- Service: `internal/booking/service.go` acts as the application-layer boundary between handlers and storage.
- Catalog: `internal/booking/catalog.go` provides the in-memory movie catalog and seat layout metadata used to build seat grids.
- Stores: the current runtime path uses a Redis-backed store for persistence, while other stores exist for learning and concurrency experiments.
- Utilities: `internal/utils/utils.go` contains shared helpers for JSON responses and error responses.

The core invariant is: **for a given movie and seat, only one booking can succeed**, even when many users try at the same time.

## Booking Flow

1. The user signs in with Clerk (Google, GitHub, etc.) from the React client.
2. The client calls the hold endpoint for a movie and seat, attaching the Clerk session token as `Authorization: Bearer <token>`.
3. The auth middleware verifies the token against Clerk's JWKS and injects the user id into the request context.
4. The handler builds a booking payload from the verified user id and passes it to the service layer.
5. The service delegates the request to the configured store implementation.
6. The store attempts to create a temporary hold for that seat.
7. If the seat is already taken, the request fails with an already-booked error.
8. If the seat is free, a temporary booking session is created and returned to the client.
9. The client can later confirm or release that session (also authenticated by the same token).

## Current Runtime Behavior

The backend is now more than a scaffold:

- The HTTP server and booking endpoints are wired up.
- Redis-backed persistence is part of the real runtime flow.
- The booking service exposes hold, list, confirm, and release operations.
- Seat listing returns a full seat grid for a movie based on catalog metadata and current booking state.
- hold/confirm/release require a valid Clerk session token; the backend derives the user id from it instead of trusting a client-sent `user_id`.
- Concurrency behavior is exercised through tests and demonstrated by store implementations.

## How to Run

1. Start Redis if needed:

```bash
docker compose up -d redis
```

2. Add your Clerk secret key (from the Clerk dashboard → API Keys → Secret keys) to a root `.env` file. Copy `.env.example` → `.env` and fill it in. The server loads it automatically at startup and exits if it's unset. You can also `export CLERK_SECRET_KEY=sk_test_...` in your shell, which overrides the `.env` value.

3. Run the server:

```bash
go run ./cmd
```

4. For the full booking flow, run the React client (Vite dev server on :5173, proxies `/movies` and `/sessions` to :8080):

```bash
cd static/client
pnpm install
pnpm dev
```

The Clerk publishable key lives in `static/client/.env` (copy `.env.example`). Sign in to hold and confirm seats.
