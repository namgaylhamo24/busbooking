package main

import (
	"context"
	"database/sql"

	pb "busbooking/gen/routepb"
)

// Store is the data-access layer for routes. It only touches route_db.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Migrate creates the routes table if it does not exist yet.
func (s *Store) Migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS routes (
			id             UUID PRIMARY KEY,
			origin         TEXT NOT NULL,
			destination    TEXT NOT NULL,
			departure_time TEXT NOT NULL,
			price          NUMERIC(10,2) NOT NULL CHECK (price > 0),
			total_seats    INTEGER NOT NULL CHECK (total_seats > 0)
		)`)
	return err
}

func (s *Store) Create(ctx context.Context, r *pb.Route) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO routes (id, origin, destination, departure_time, price, total_seats)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		r.Id, r.Origin, r.Destination, r.DepartureTime, r.Price, r.TotalSeats)
	return err
}

// Get returns sql.ErrNoRows when the route does not exist.
func (s *Store) Get(ctx context.Context, id string) (*pb.Route, error) {
	r := &pb.Route{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, origin, destination, departure_time, price::float8, total_seats
		 FROM routes WHERE id = $1`, id).
		Scan(&r.Id, &r.Origin, &r.Destination, &r.DepartureTime, &r.Price, &r.TotalSeats)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) List(ctx context.Context) ([]*pb.Route, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, origin, destination, departure_time, price::float8, total_seats
		 FROM routes ORDER BY origin, departure_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*pb.Route
	for rows.Next() {
		r := &pb.Route{}
		if err := rows.Scan(&r.Id, &r.Origin, &r.Destination, &r.DepartureTime, &r.Price, &r.TotalSeats); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Update returns sql.ErrNoRows if no row matched the id.
func (s *Store) Update(ctx context.Context, r *pb.Route) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE routes SET origin=$2, destination=$3, departure_time=$4, price=$5, total_seats=$6
		 WHERE id=$1`,
		r.Id, r.Origin, r.Destination, r.DepartureTime, r.Price, r.TotalSeats)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Delete returns sql.ErrNoRows if no row matched the id.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
