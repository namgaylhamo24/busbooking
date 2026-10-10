package main

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ticketpb "busbooking/gen/ticketpb"
)

// validateTicket checks the fields this service owns. It runs BEFORE the call
// to route-service and before any database write, so invalid data is never
// stored. Checks that need route data (capacity) happen later in server.go.
func validateTicket(t *ticketpb.Ticket) error {
	if t == nil {
		return status.Error(codes.InvalidArgument, "ticket is required")
	}
	name := strings.TrimSpace(t.PassengerName)

	if _, err := uuid.Parse(t.RouteId); err != nil {
		return status.Error(codes.InvalidArgument, "route_id must be a valid UUID")
	}
	if len(name) < 2 || len(name) > 80 {
		return status.Error(codes.InvalidArgument, "passenger_name is required (2-80 characters)")
	}
	if t.Seats < 1 {
		return status.Error(codes.InvalidArgument, "seats must be at least 1")
	}

	date, err := time.Parse("2006-01-02", t.TravelDate)
	if err != nil {
		return status.Error(codes.InvalidArgument, "travel_date must be a valid date in YYYY-MM-DD format")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if date.Before(today) {
		return status.Error(codes.InvalidArgument, "travel_date cannot be in the past")
	}

	t.PassengerName = name // store the trimmed value
	return nil
}
