# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/api /app/api
COPY --from=builder /out/worker /app/worker
# Migrations stay explicit: run with --entrypoint /app/migrate.
COPY --from=builder /out/migrate /app/migrate

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/api"]
