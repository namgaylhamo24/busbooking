package main

import (
	"context"
	"database/sql"

	ticketpb "busbooking/gen/ticketpb"
)

// Store is the data-access layer for tickets. It only touches ticket_db.
// route_id is a plain column with NO foreign key: the routes table lives in
// another service's database (Database per Service).
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tickets (
			id             UUID PRIMARY KEY,
			route_id       UUID NOT NULL,
			passenger_name TEXT NOT NULL,
			travel_date    DATE NOT NULL,
			seats          INTEGER NOT NULL CHECK (seats > 0),
			total_price    NUMERIC(10,2) NOT NULL CHECK (total_price >= 0)
		)`)
	return err
}

const ticketColumns = `id, route_id, passenger_name, to_char(travel_date, 'YYYY-MM-DD'), seats, total_price::float8`

func (s *Store) Create(ctx context.Context, t *ticketpb.Ticket) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tickets (id, route_id, passenger_name, travel_date, seats, total_price)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		t.Id, t.RouteId, t.PassengerName, t.TravelDate, t.Seats, t.TotalPrice)
	return err
}

// Get returns sql.ErrNoRows when the ticket does not exist.
func (s *Store) Get(ctx context.Context, id string) (*ticketpb.Ticket, error) {
	t := &ticketpb.Ticket{}
	err := s.db.QueryRowContext(ctx,
		`SELECT `+ticketColumns+` FROM tickets WHERE id = $1`, id).
		Scan(&t.Id, &t.RouteId, &t.PassengerName, &t.TravelDate, &t.Seats, &t.TotalPrice)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) List(ctx context.Context) ([]*ticketpb.Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+ticketColumns+` FROM tickets ORDER BY travel_date, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ticketpb.Ticket
	for rows.Next() {
		t := &ticketpb.Ticket{}
		if err := rows.Scan(&t.Id, &t.RouteId, &t.PassengerName, &t.TravelDate, &t.Seats, &t.TotalPrice); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Update returns sql.ErrNoRows if no row matched the id.
func (s *Store) Update(ctx context.Context, t *ticketpb.Ticket) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET route_id=$2, passenger_name=$3, travel_date=$4, seats=$5, total_price=$6
		 WHERE id=$1`,
		t.Id, t.RouteId, t.PassengerName, t.TravelDate, t.Seats, t.TotalPrice)
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
	res, err := s.db.ExecContext(ctx, `DELETE FROM tickets WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
