## The Problem

Two users click "Book" on seat A1 at the same instant. Only one should win.

```bash
User A ──► read seat A1 → "free" ──► write booking ──► success
User B ──► read seat A1 → "free" ──► write booking ──► ???
```

Without any protection, both succeed. Now two people show up for the same seat.

## Architecture (Current Design)

This project follows a simple layered architecture:

- Entrypoint: `cmd/main.go` wires the HTTP server, routes, and dependency setup.
- Handlers: `internal/booking/handler.go` receives HTTP requests and translates them into service calls.
- Domain: `internal/booking/domain.go` defines the booking model and the storage contract.
- Service: `internal/booking/service.go` acts as the application-layer boundary between handlers and storage.
- Stores: the project currently uses a Redis-backed store for persistence, with other store implementations used for learning and concurrency experiments.
- Utilities: `internal/utils/utils.go` contains shared response helpers.

The core invariant is: **for a given movie and seat, only one booking can succeed**, even when many users try at the same time.

## Booking Flow

1. A user sends a booking request for `movieID + seatID`.
2. The handler parses the request and forwards it to the booking service.
3. The service delegates the operation to the configured store implementation.
4. If the seat is already taken, the request fails with an already-booked error.
5. If free, a booking hold is created and returned to the client.

## Current Status

The project is now moving beyond a pure scaffold:

- The HTTP server and booking handlers are wired up.
- Redis-backed persistence is part of the current flow.
- The booking service exposes hold/list/confirm/release operations.
- Concurrency behavior is covered by tests and demonstrated through store implementations.
