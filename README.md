## The Problem

Two users click "Book" on seat A1 at the same instant. Only one should win.

```bash
User A ──► read seat A1 → "free" ──► write booking ──► success
User B ──► read seat A1 → "free" ──► write booking ──► ???
```

Without any protection, both succeed. Now two people show up for the same seat.

## Architecture (Current Design)

This project follows a simple layered architecture:

- Entrypoint: `cmd/main.go` wires dependencies and starts the application.
- Domain: `internal/booking/domain.go` defines core entities and storage contracts.
- Service: `internal/booking/service.go` contains business rules and orchestration.
- Utilities: `internal/utils/utils.go` contains shared helpers (for example JSON responses).

The core invariant is: **for a given movie and seat, only one booking can succeed**, even when many users try at the same time.

## Booking Flow (Intended)

1. A user sends a booking request for `movieID + seatID`.
2. The service validates input and applies booking rules.
3. The service asks the store to claim the seat atomically.
4. If the seat is already taken, the request fails.
5. If free, a booking is persisted and returned.

## Current Status

This repository is currently a scaffold/in-progress implementation:

- Core domain model and interfaces are defined.
- Service wiring is started.
- Concurrency behavior is specified in tests.
- HTTP/server wiring and concrete store implementation are still to be completed.
