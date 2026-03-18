package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	flightv1 "github.com/flight-booking/flight-service/gen/flight/v1"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SearchFlights(ctx context.Context, origin, destination, date string) ([]*flightv1.Flight, error) {
	query := `
		SELECT id, flight_number, airline, origin, destination, 
		       departure_time, arrival_time, total_seats, available_seats, 
		       price_kopecks, status
		FROM flights
		WHERE origin = $1 AND destination = $2 AND status = 'SCHEDULED'
	`
	args := []interface{}{origin, destination}

	if date != "" {
		query += " AND DATE(departure_time) = $3"
		args = append(args, date)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search flights: %w", err)
	}
	defer rows.Close()

	var flights []*flightv1.Flight
	for rows.Next() {
		f := &flightv1.Flight{}
		var departureTime, arrivalTime time.Time
		var status string

		err := rows.Scan(
			&f.Id, &f.FlightNumber, &f.Airline, &f.Origin, &f.Destination,
			&departureTime, &arrivalTime, &f.TotalSeats, &f.AvailableSeats,
			&f.PriceKopecks, &status,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan flight: %w", err)
		}

		f.DepartureTime = departureTime.Format(time.RFC3339)
		f.ArrivalTime = arrivalTime.Format(time.RFC3339)
		f.Status = dbStatusToProto(status)

		flights = append(flights, f)
	}

	return flights, nil
}

// dbStatusToProto converts DB status string (e.g. "SCHEDULED") to proto enum
func dbStatusToProto(status string) flightv1.FlightStatus {
	mapping := map[string]flightv1.FlightStatus{
		"SCHEDULED": flightv1.FlightStatus_FLIGHT_STATUS_SCHEDULED,
		"DEPARTED":  flightv1.FlightStatus_FLIGHT_STATUS_DEPARTED,
		"CANCELLED": flightv1.FlightStatus_FLIGHT_STATUS_CANCELLED,
		"COMPLETED": flightv1.FlightStatus_FLIGHT_STATUS_COMPLETED,
	}
	if v, ok := mapping[status]; ok {
		return v
	}
	return flightv1.FlightStatus_FLIGHT_STATUS_UNSPECIFIED
}

func (r *Repository) GetFlight(ctx context.Context, flightID int64) (*flightv1.Flight, error) {
	query := `
		SELECT id, flight_number, airline, origin, destination,
		       departure_time, arrival_time, total_seats, available_seats,
		       price_kopecks, status
		FROM flights
		WHERE id = $1
	`

	f := &flightv1.Flight{}
	var departureTime, arrivalTime time.Time
	var status string

	err := r.db.QueryRowContext(ctx, query, flightID).Scan(
		&f.Id, &f.FlightNumber, &f.Airline, &f.Origin, &f.Destination,
		&departureTime, &arrivalTime, &f.TotalSeats, &f.AvailableSeats,
		&f.PriceKopecks, &status,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get flight: %w", err)
	}

	f.DepartureTime = departureTime.Format(time.RFC3339)
	f.ArrivalTime = arrivalTime.Format(time.RFC3339)
	f.Status = dbStatusToProto(status)

	return f, nil
}

func (r *Repository) ReserveSeats(ctx context.Context, flightID int64, seatCount int32, bookingID string) (*flightv1.SeatReservation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if reservation already exists (idempotency)
	var existingID int64
	err = tx.QueryRowContext(ctx,
		"SELECT id FROM seat_reservations WHERE booking_id = $1", bookingID).Scan(&existingID)
	if err == nil {
		// Reservation already exists, return it
		return r.getReservationByID(ctx, tx, existingID)
	}

	// Lock the flight row and check availability
	var availableSeats int32
	var priceKopecks int64
	err = tx.QueryRowContext(ctx,
		"SELECT available_seats, price_kopecks FROM flights WHERE id = $1 FOR UPDATE", flightID).
		Scan(&availableSeats, &priceKopecks)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("flight not found")
		}
		return nil, fmt.Errorf("failed to get flight: %w", err)
	}

	if availableSeats < seatCount {
		return nil, fmt.Errorf("not enough seats available")
	}

	// Update available seats
	_, err = tx.ExecContext(ctx,
		"UPDATE flights SET available_seats = available_seats - $1, updated_at = NOW() WHERE id = $2",
		seatCount, flightID)
	if err != nil {
		return nil, fmt.Errorf("failed to update flight: %w", err)
	}

	// Create reservation
	query := `
		INSERT INTO seat_reservations (flight_id, booking_id, seat_count, status, price_kopecks)
		VALUES ($1, $2, $3, 'ACTIVE', $4)
		RETURNING id, created_at
	`
	var id int64
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, query, flightID, bookingID, seatCount, priceKopecks*int64(seatCount)).
		Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create reservation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &flightv1.SeatReservation{
		Id:           id,
		FlightId:     flightID,
		BookingId:    bookingID,
		SeatCount:    seatCount,
		Status:       flightv1.ReservationStatus_RESERVATION_STATUS_ACTIVE,
		CreatedAt:    createdAt.Format(time.RFC3339),
		PriceKopecks: priceKopecks * int64(seatCount),
	}, nil
}

func (r *Repository) ReleaseReservation(ctx context.Context, bookingID string) (*flightv1.SeatReservation, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get reservation
	var flightID int64
	var seatCount int32
	var status string
	var id int64
	var createdAt time.Time
	var priceKopecks int64

	err = tx.QueryRowContext(ctx, `
		SELECT id, flight_id, seat_count, status, created_at, price_kopecks
		FROM seat_reservations
		WHERE booking_id = $1 FOR UPDATE
	`, bookingID).Scan(&id, &flightID, &seatCount, &status, &createdAt, &priceKopecks)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("reservation not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get reservation: %w", err)
	}

	if status != "ACTIVE" {
		return nil, fmt.Errorf("reservation is not active")
	}

	// Update flight available seats
	_, err = tx.ExecContext(ctx,
		"UPDATE flights SET available_seats = available_seats + $1, updated_at = NOW() WHERE id = $2",
		seatCount, flightID)
	if err != nil {
		return nil, fmt.Errorf("failed to update flight: %w", err)
	}

	// Update reservation status
	_, err = tx.ExecContext(ctx,
		"UPDATE seat_reservations SET status = 'RELEASED', updated_at = NOW() WHERE id = $1",
		id)
	if err != nil {
		return nil, fmt.Errorf("failed to update reservation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &flightv1.SeatReservation{
		Id:           id,
		FlightId:     flightID,
		BookingId:    bookingID,
		SeatCount:    seatCount,
		Status:       flightv1.ReservationStatus_RESERVATION_STATUS_RELEASED,
		CreatedAt:    createdAt.Format(time.RFC3339),
		PriceKopecks: priceKopecks,
	}, nil
}

func (r *Repository) getReservationByID(ctx context.Context, tx *sql.Tx, id int64) (*flightv1.SeatReservation, error) {
	var flightID int64
	var seatCount int32
	var status string
	var createdAt time.Time
	var priceKopecks int64
	var bookingID string

	err := tx.QueryRowContext(ctx, `
		SELECT id, flight_id, booking_id, seat_count, status, created_at, price_kopecks
		FROM seat_reservations
		WHERE id = $1
	`, id).Scan(&id, &flightID, &bookingID, &seatCount, &status, &createdAt, &priceKopecks)

	if err != nil {
		return nil, fmt.Errorf("failed to get reservation: %w", err)
	}

	return &flightv1.SeatReservation{
		Id:           id,
		FlightId:     flightID,
		BookingId:    bookingID,
		SeatCount:    seatCount,
		Status:       flightv1.ReservationStatus(flightv1.ReservationStatus_value[status]),
		CreatedAt:    createdAt.Format(time.RFC3339),
		PriceKopecks: priceKopecks,
	}, nil
}
