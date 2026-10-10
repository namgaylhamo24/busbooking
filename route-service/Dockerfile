# syntax=docker/dockerfile:1

# ---------- Stage 1: build ----------
# Full Go toolchain; used only to compile, never shipped.
FROM golang:1.26-alpine AS builder
# GOTOOLCHAIN=auto fetches the exact Go version named in go.mod if needed.
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src

# Copy module files first so the dependency layer is cached between builds.
COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY route-service ./route-service
RUN go build -trimpath -ldflags="-s -w" -o /out/route-service ./route-service

# ---------- Stage 2: runtime ----------
# Tiny image containing only the static binary, running as a non-root user.
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=builder /out/route-service /usr/local/bin/route-service
EXPOSE 50051
ENTRYPOINT ["route-service"]
