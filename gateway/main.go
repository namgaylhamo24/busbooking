package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	routepb "busbooking/gen/routepb"
	ticketpb "busbooking/gen/ticketpb"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("invalid %s=%q, using default %d", key, v, fallback)
	}
	return fallback
}

// dial creates a gRPC client connection. NewClient connects lazily, so the
// gateway can start even if a backend is down. The reconnect backoff is
// shortened so a restarted backend is picked up within a few seconds.
func dial(addr string) *grpc.ClientConn {
	conn, err := grpc.NewClient("passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  500 * time.Millisecond,
				Multiplier: 1.6,
				Jitter:     0.2,
				MaxDelay:   3 * time.Second,
			},
			MinConnectTimeout: 2 * time.Second,
		}),
	)
	if err != nil {
		log.Fatalf("grpc client for %s: %v", addr, err)
	}
	return conn
}

func main() {
	routeAddr := getenv("ROUTE_SERVICE_ADDR", "localhost:50051")
	ticketAddr := getenv("TICKET_SERVICE_ADDR", "localhost:50052")

	routeConn := dial(routeAddr)
	defer routeConn.Close()
	ticketConn := dial(ticketAddr)
	defer ticketConn.Close()

	// The per-request timeout must be longer than ticket-service's worst case
	// (3 attempts x 2s timeout + backoff = ~6.6s), otherwise the gateway would
	// give up before ticket-service can return its own meaningful error.
	h := &Handler{
		routes:  routepb.NewRouteServiceClient(routeConn),
		tickets: ticketpb.NewTicketServiceClient(ticketConn),
		timeout: time.Duration(getenvInt("GATEWAY_TIMEOUT_MS", 10000)) * time.Millisecond,
	}

	mux := http.NewServeMux()
	h.Register(mux)

	port := getenv("HTTP_PORT", "8080")
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           withMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
	}

	log.Printf("gateway listening on :%s (routes=%s, tickets=%s)", port, routeAddr, ticketAddr)
	log.Fatal(srv.ListenAndServe())
}
