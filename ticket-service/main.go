package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"

	routepb "busbooking/gen/routepb"
	ticketpb "busbooking/gen/ticketpb"
)

// NOTE: getenv/getenvInt/connectDB are intentionally duplicated from
// route-service. Each service must be independently buildable and deployable,
// so they share generated proto code but no hand-written packages.

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

// connectDB opens the Postgres connection, retrying while the database
// container is still starting up.
func connectDB() *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_USER", "postgres"),
		getenv("DB_PASSWORD", ""),
		getenv("DB_NAME", "ticket_db"),
	)

	var lastErr error
	for i := 1; i <= 15; i++ {
		db, err := sql.Open("postgres", dsn)
		if err == nil {
			if err = db.Ping(); err == nil {
				log.Println("connected to database")
				return db
			}
		}
		lastErr = err
		log.Printf("database not ready (attempt %d/15): %v", i, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("could not connect to database: %v", lastErr)
	return nil
}

func main() {
	db := connectDB()
	defer db.Close()

	store := NewStore(db)
	if err := store.Migrate(); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// gRPC client connection to route-service (Service B). NewClient is lazy,
	// so ticket-service can start even when route-service is down.
	// The connect backoff is shortened so the channel reconnects quickly
	// after route-service comes back (the default can wait up to 2 minutes).
	routeAddr := getenv("ROUTE_SERVICE_ADDR", "localhost:50051")
	conn, err := grpc.NewClient("passthrough:///"+routeAddr,
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
		log.Fatalf("grpc client: %v", err)
	}
	defer conn.Close()

	// Resilience settings come from environment variables (with defaults).
	routes := NewRouteClient(routepb.NewRouteServiceClient(conn), ClientConfig{
		Timeout:          time.Duration(getenvInt("ROUTE_TIMEOUT_MS", 2000)) * time.Millisecond,
		MaxAttempts:      getenvInt("ROUTE_MAX_ATTEMPTS", 3),
		BaseBackoff:      time.Duration(getenvInt("ROUTE_BACKOFF_MS", 200)) * time.Millisecond,
		FailureThreshold: getenvInt("CB_FAILURE_THRESHOLD", 3),
		OpenDuration:     time.Duration(getenvInt("CB_OPEN_SECONDS", 10)) * time.Second,
	})

	port := getenv("GRPC_PORT", "50052")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	ticketpb.RegisterTicketServiceServer(grpcServer, NewServer(store, routes))

	log.Printf("ticket-service listening on :%s (route-service at %s)", port, routeAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
