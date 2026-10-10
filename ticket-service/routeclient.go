package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/sony/gobreaker"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	routepb "busbooking/gen/routepb"
)

// ClientConfig holds the resilience settings (all read from env vars in main).
type ClientConfig struct {
	Timeout          time.Duration // per-attempt deadline
	MaxAttempts      int           // total tries, including the first
	BaseBackoff      time.Duration // wait before 2nd attempt; doubles each time
	FailureThreshold int           // consecutive failed calls that open the breaker
	OpenDuration     time.Duration // how long the breaker stays open before probing
}

// RouteClient wraps the generated gRPC client with three resilience patterns:
//
//  1. TIMEOUT          - every attempt gets its own deadline, so a slow or hung
//                        route-service can never block a request forever.
//  2. RETRY + BACKOFF  - transient failures are retried with exponentially
//                        increasing waits (200ms, 400ms, ...).
//  3. CIRCUIT BREAKER  - after repeated failures, calls are rejected instantly
//                        (no waiting) until the breaker lets a probe through.
type RouteClient struct {
	client  routepb.RouteServiceClient
	breaker *gobreaker.CircuitBreaker
	cfg     ClientConfig
}

func NewRouteClient(client routepb.RouteServiceClient, cfg ClientConfig) *RouteClient {
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        "route-service",
		MaxRequests: 1,            // probe requests allowed while half-open
		Timeout:     cfg.OpenDuration, // open -> half-open after this long
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= uint32(cfg.FailureThreshold)
		},
		// Business answers (not found / bad input / caller gave up) prove that
		// route-service is healthy, so they must NOT count as failures.
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			switch status.Code(err) {
			case codes.NotFound, codes.InvalidArgument, codes.Canceled:
				return true
			}
			return false
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Printf("CIRCUIT BREAKER %q: %s -> %s", name, from, to)
		},
	})
	return &RouteClient{client: client, breaker: breaker, cfg: cfg}
}

// GetRoute fetches a route from route-service through the circuit breaker.
func (c *RouteClient) GetRoute(ctx context.Context, id string) (*routepb.Route, error) {
	result, err := c.breaker.Execute(func() (interface{}, error) {
		return c.getWithRetry(ctx, id)
	})
	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			log.Printf("route-service call rejected: circuit breaker is open")
			return nil, status.Error(codes.Unavailable,
				"route service is temporarily unavailable (circuit breaker open), please try again shortly")
		}
		return nil, err
	}
	return result.(*routepb.Route), nil
}

// getWithRetry makes up to MaxAttempts calls, each with its own timeout.
func (c *RouteClient) getWithRetry(ctx context.Context, id string) (*routepb.Route, error) {
	wait := c.cfg.BaseBackoff
	var lastErr error

	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
		route, err := c.client.GetRoute(attemptCtx, &routepb.GetRouteRequest{Id: id})
		cancel()

		if err == nil {
			return route, nil
		}
		if !isRetryable(err) {
			return nil, err // e.g. NotFound: retrying cannot help
		}

		lastErr = err
		log.Printf("route-service call failed (attempt %d/%d): %v", attempt, c.cfg.MaxAttempts, err)
		if attempt == c.cfg.MaxAttempts {
			break
		}

		log.Printf("retrying in %v", wait)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, status.FromContextError(ctx.Err()).Err()
		}
		wait *= 2 // exponential backoff
	}

	return nil, status.Errorf(codes.Unavailable,
		"route service is unreachable after %d attempts: %s",
		c.cfg.MaxAttempts, status.Convert(lastErr).Message())
}

// isRetryable reports whether an error is a transient infrastructure problem.
func isRetryable(err error) bool {
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded:
		return true
	}
	return false
}
