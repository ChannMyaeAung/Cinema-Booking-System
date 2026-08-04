## The Problem

Two users click "Book" on seat A1 at the same instant. Only one should win.

```bash
User A ──► read seat A1 → "free" ──► write booking ──► success
User B ──► read seat A1 → "free" ──► write booking ──► ???
```

Without any protection, both succeed. Now two people show up for the same seat.

## Architecture (Current Design)

This project follows a simple layered architecture centered around booking flows:

- Entrypoint: `cmd/main.go` starts the HTTP server, registers routes, creates the Redis client, and wires the service and handler layers.
- Handlers: `internal/booking/handler.go` receives HTTP requests and translates them into booking operations such as hold, list, confirm, and release.
- Domain: `internal/booking/domain.go` defines the core `Booking` model and the `BookingStore` contract.
- Service: `internal/booking/service.go` acts as the application-layer boundary between handlers and storage.
- Catalog: `internal/booking/catalog.go` provides the in-memory movie catalog and seat layout metadata used to build seat grids.
- Stores: the current runtime path uses a Redis-backed store for persistence, while other stores exist for learning and concurrency experiments.
- Utilities: `internal/utils/utils.go` contains shared helpers for JSON responses and error responses.

The core invariant is: **for a given movie and seat, only one booking can succeed**, even when many users try at the same time.

## Booking Flow

1. A client calls the hold endpoint for a movie and seat.
2. The handler creates a booking payload and passes it to the service layer.
3. The service delegates the request to the configured store implementation.
4. The store attempts to create a temporary hold for that seat.
5. If the seat is already taken, the request fails with an already-booked error.
6. If the seat is free, a temporary booking session is created and returned to the client.
7. The client can later confirm or release that session.

## Current Runtime Behavior

The backend is now more than a scaffold:

- The HTTP server and booking endpoints are wired up.
- Redis-backed persistence is part of the real runtime flow.
- The booking service exposes hold, list, confirm, and release operations.
- Seat listing returns a full seat grid for a movie based on catalog metadata and current booking state.
- Concurrency behavior is exercised through tests and demonstrated by store implementations.

## How to Run

1. Start Redis if needed.
2. Run the server:

```bash
go run ./cmd
```

3. Visit the movie list endpoint or the static UI served from the `static` folder.
