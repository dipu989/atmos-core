# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the API binary (statically linked, stripped)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/bin/api ./cmd/api

# Build the migrate binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/bin/migrate ./cmd/migrate

# Build the seed binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /app/bin/seed ./cmd/seed

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binaries
COPY --from=builder /app/bin/api     ./api
COPY --from=builder /app/bin/migrate ./migrate
COPY --from=builder /app/bin/seed    ./seed

# Copy migrations directory (needed by migrate binary at runtime)
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["./api"]
