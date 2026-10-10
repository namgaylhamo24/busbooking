package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	routepb "busbooking/gen/routepb"
	ticketpb "busbooking/gen/ticketpb"
)

// Server implements ticketpb.TicketServiceServer.
type Server struct {
	ticketpb.UnimplementedTicketServiceServer
	store  *Store
	routes *RouteClient
}

func NewServer(s *Store, r *RouteClient) *Server { return &Server{store: s, routes: r} }

func toStatus(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return status.Errorf(codes.NotFound, "%s not found", what)
	}
	log.Printf("internal error: %v", err)
	return status.Error(codes.Internal, "internal database error")
}

func validID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return status.Error(codes.InvalidArgument, "id must be a valid UUID")
	}
	return nil
}

// lookupRoute asks route-service (through the resilience client) for a route.
func (s *Server) lookupRoute(ctx context.Context, id string) (*routepb.Route, error) {
	route, err := s.routes.GetRoute(ctx, id)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, status.Errorf(codes.NotFound, "route %s does not exist", id)
		}
		return nil, err
	}
	return route, nil
}

// priceAndCheck applies the cross-service business rules using route data.
func priceAndCheck(t *ticketpb.Ticket, route *routepb.Route) error {
	if t.Seats > route.TotalSeats {
		return status.Errorf(codes.InvalidArgument,
			"seats (%d) exceed the route's capacity (%d)", t.Seats, route.TotalSeats)
	}
	t.TotalPrice = math.Round(float64(t.Seats)*route.Price*100) / 100
	return nil
}

func summarise(r *routepb.Route) string {
	return fmt.Sprintf("%s -> %s %s", r.Origin, r.Destination, r.DepartureTime)
}

type routeInfo struct {
	summary  string
	degraded bool
}

// enrich fills route_summary/degraded on tickets using route-service.
// These fields are never stored. If route-service is unreachable the tickets
// are still returned (FALLBACK) with degraded=true. Results are cached per
// route for one call, and after the first outage we stop calling, so listing
// many tickets during an outage stays fast.
func (s *Server) enrich(ctx context.Context, tickets ...*ticketpb.Ticket) {
	cache := make(map[string]routeInfo)
	routeServiceDown := false

	for _, t := range tickets {
		info, seen := cache[t.RouteId]
		if !seen {
			if routeServiceDown {
				info = routeInfo{degraded: true}
			} else {
				route, err := s.routes.GetRoute(ctx, t.RouteId)
				switch {
				case err == nil:
					info = routeInfo{summary: summarise(route)}
				case status.Code(err) == codes.NotFound:
					info = routeInfo{summary: "route no longer exists", degraded: true}
				default:
					log.Printf("fallback: serving ticket data without route details: %v", err)
					routeServiceDown = true
					info = routeInfo{degraded: true}
				}
			}
			cache[t.RouteId] = info
		}
		t.RouteSummary, t.Degraded = info.summary, info.degraded
	}
}

func (s *Server) CreateTicket(ctx context.Context, req *ticketpb.CreateTicketRequest) (*ticketpb.Ticket, error) {
	t := req.GetTicket()
	if err := validateTicket(t); err != nil {
		return nil, err
	}

	// Cross-service call: the route must exist (and gives us price/capacity).
	route, err := s.lookupRoute(ctx, t.RouteId)
	if err != nil {
		return nil, err
	}
	if err := priceAndCheck(t, route); err != nil {
		return nil, err
	}

	t.Id = uuid.NewString()
	if err := s.store.Create(ctx, t); err != nil {
		return nil, toStatus(err, "ticket")
	}
	t.RouteSummary = summarise(route)
	log.Printf("created ticket %s on route %s (%d seats, %.2f)", t.Id, t.RouteId, t.Seats, t.TotalPrice)
	return t, nil
}

func (s *Server) GetTicket(ctx context.Context, req *ticketpb.GetTicketRequest) (*ticketpb.Ticket, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	t, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err, "ticket")
	}
	s.enrich(ctx, t)
	return t, nil
}

func (s *Server) ListTickets(ctx context.Context, _ *ticketpb.ListTicketsRequest) (*ticketpb.ListTicketsResponse, error) {
	tickets, err := s.store.List(ctx)
	if err != nil {
		return nil, toStatus(err, "ticket")
	}
	s.enrich(ctx, tickets...)
	return &ticketpb.ListTicketsResponse{Tickets: tickets}, nil
}

func (s *Server) UpdateTicket(ctx context.Context, req *ticketpb.UpdateTicketRequest) (*ticketpb.Ticket, error) {
	t := req.GetTicket()
	if t == nil {
		return nil, status.Error(codes.InvalidArgument, "ticket is required")
	}
	if err := validID(t.Id); err != nil {
		return nil, err
	}
	if err := validateTicket(t); err != nil {
		return nil, err
	}

	route, err := s.lookupRoute(ctx, t.RouteId)
	if err != nil {
		return nil, err
	}
	if err := priceAndCheck(t, route); err != nil {
		return nil, err
	}

	if err := s.store.Update(ctx, t); err != nil {
		return nil, toStatus(err, "ticket")
	}
	t.RouteSummary = summarise(route)
	return t, nil
}

func (s *Server) DeleteTicket(ctx context.Context, req *ticketpb.DeleteTicketRequest) (*ticketpb.DeleteTicketResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if err := s.store.Delete(ctx, req.GetId()); err != nil {
		return nil, toStatus(err, "ticket")
	}
	return &ticketpb.DeleteTicketResponse{}, nil
}
