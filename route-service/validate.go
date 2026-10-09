package main

import (
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "busbooking/gen/routepb"
)

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// validateRoute returns an InvalidArgument error describing the first problem
// found, or nil if the route is valid. It runs BEFORE any database write, so
// invalid data is never stored.
func validateRoute(r *pb.Route) error {
	if r == nil {
		return status.Error(codes.InvalidArgument, "route is required")
	}
	origin := strings.TrimSpace(r.Origin)
	dest := strings.TrimSpace(r.Destination)

	switch {
	case len(origin) < 2 || len(origin) > 60:
		return status.Error(codes.InvalidArgument, "origin is required (2-60 characters)")
	case len(dest) < 2 || len(dest) > 60:
		return status.Error(codes.InvalidArgument, "destination is required (2-60 characters)")
	case strings.EqualFold(origin, dest):
		return status.Error(codes.InvalidArgument, "origin and destination must differ")
	case !hhmm.MatchString(r.DepartureTime):
		return status.Error(codes.InvalidArgument, "departure_time must be HH:MM (24-hour)")
	case r.Price <= 0:
		return status.Error(codes.InvalidArgument, "price must be greater than 0")
	case r.TotalSeats < 1 || r.TotalSeats > 100:
		return status.Error(codes.InvalidArgument, "total_seats must be between 1 and 100")
	}

	// Store trimmed values so we never save stray whitespace.
	r.Origin, r.Destination = origin, dest
	return nil
}
