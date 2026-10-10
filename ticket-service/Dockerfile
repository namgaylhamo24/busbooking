# syntax=docker/dockerfile:1

# ---------- Stage 1: build ----------
FROM golang:1.26-alpine AS builder
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY ticket-service ./ticket-service
RUN go build -trimpath -ldflags="-s -w" -o /out/ticket-service ./ticket-service

# ---------- Stage 2: runtime ----------
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=builder /out/ticket-service /usr/local/bin/ticket-service
EXPOSE 50052
ENTRYPOINT ["ticket-service"]
