package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"

	pb "busbooking/gen/routepb"
)

// getenv returns the environment variable, or the fallback if it is unset.
func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
		getenv("DB_NAME", "route_db"),
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

	port := getenv("GRPC_PORT", "50051")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterRouteServiceServer(grpcServer, NewServer(store))

	log.Printf("route-service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
