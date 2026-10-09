package main

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "busbooking/gen/routepb"
)

// Server implements pb.RouteServiceServer.
type Server struct {
	pb.UnimplementedRouteServiceServer
	store *Store
}

func NewServer(s *Store) *Server { return &Server{store: s} }

// toStatus converts store errors into gRPC status errors. Internal details
// are logged but never sent to the caller.
func toStatus(err error, what string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return status.Errorf(codes.NotFound, "%s not found", what)
	}
	log.Printf("internal error: %v", err)
	return status.Error(codes.Internal, "internal database error")
}

// validID rejects non-UUID ids so a bad id gives a clean InvalidArgument
// instead of a Postgres syntax error.
func validID(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return status.Error(codes.InvalidArgument, "id must be a valid UUID")
	}
	return nil
}

func (s *Server) CreateRoute(ctx context.Context, req *pb.CreateRouteRequest) (*pb.Route, error) {
	r := req.GetRoute()
	if err := validateRoute(r); err != nil {
		return nil, err
	}
	r.Id = uuid.NewString()
	if err := s.store.Create(ctx, r); err != nil {
		return nil, toStatus(err, "route")
	}
	log.Printf("created route %s (%s -> %s)", r.Id, r.Origin, r.Destination)
	return r, nil
}

func (s *Server) GetRoute(ctx context.Context, req *pb.GetRouteRequest) (*pb.Route, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	r, err := s.store.Get(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err, "route")
	}
	return r, nil
}

func (s *Server) ListRoutes(ctx context.Context, _ *pb.ListRoutesRequest) (*pb.ListRoutesResponse, error) {
	routes, err := s.store.List(ctx)
	if err != nil {
		return nil, toStatus(err, "route")
	}
	return &pb.ListRoutesResponse{Routes: routes}, nil
}

func (s *Server) UpdateRoute(ctx context.Context, req *pb.UpdateRouteRequest) (*pb.Route, error) {
	r := req.GetRoute()
	if r == nil {
		return nil, status.Error(codes.InvalidArgument, "route is required")
	}
	if err := validID(r.Id); err != nil {
		return nil, err
	}
	if err := validateRoute(r); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, r); err != nil {
		return nil, toStatus(err, "route")
	}
	return r, nil
}

func (s *Server) DeleteRoute(ctx context.Context, req *pb.DeleteRouteRequest) (*pb.DeleteRouteResponse, error) {
	if err := validID(req.GetId()); err != nil {
		return nil, err
	}
	if err := s.store.Delete(ctx, req.GetId()); err != nil {
		return nil, toStatus(err, "route")
	}
	return &pb.DeleteRouteResponse{}, nil
}
