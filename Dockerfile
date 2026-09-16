# Builds the whole app into a single image: the React client is bundled in the
# frontend stage, then copied into the Go server image (served from
# static/client/dist) so one container runs the entire application.

# ---------- Frontend: build static/client -> static/client/dist ----------
FROM node:22-alpine AS client
WORKDIR /client
RUN corepack enable
COPY static/client/package.json static/client/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY static/client/ ./
RUN pnpm build

# ---------- Backend: compile the Go server ----------
FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=client /client/dist ./static/client/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cinebook ./cmd

# ---------- Runtime: Alpine + CA certs (for Clerk/Stripe TLS) ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S -G app -u 10001 app
WORKDIR /app
COPY --from=builder /out/cinebook /app/cinebook
COPY --from=builder /src/static /app/static
USER app
EXPOSE 8080
ENTRYPOINT ["/app/cinebook"]