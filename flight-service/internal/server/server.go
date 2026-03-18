package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	flightv1 "github.com/flight-booking/flight-service/gen/flight/v1"
	"github.com/flight-booking/flight-service/internal/cache"
	"github.com/flight-booking/flight-service/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	flightv1.UnimplementedFlightServiceServer
	repo  *repository.Repository
	cache *cache.Cache
}

func NewServer(db *sql.DB, cache *cache.Cache) *Server {
	return &Server{
		repo:  repository.NewRepository(db),
		cache: cache,
	}
}

func (s *Server) SearchFlights(ctx context.Context, req *flightv1.SearchFlightsRequest) (*flightv1.SearchFlightsResponse, error) {
	log.Printf("SearchFlights: origin=%s, destination=%s, date=%s", req.Origin, req.Destination, req.Date)

	// Try cache first
	flights, err := s.cache.GetSearchResults(ctx, req.Origin, req.Destination, req.Date)
	if err != nil {
		log.Printf("Cache error: %v", err)
	}

	if flights != nil {
		log.Printf("Cache HIT for search: %s:%s:%s", req.Origin, req.Destination, req.Date)
		return &flightv1.SearchFlightsResponse{Flights: flights}, nil
	}

	log.Printf("Cache MISS for search: %s:%s:%s", req.Origin, req.Destination, req.Date)

	// Query database
	flights, err = s.repo.SearchFlights(ctx, req.Origin, req.Destination, req.Date)
	if err != nil {
		log.Printf("Failed to search flights: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to search flights")
	}

	// Set cache with TTL
	if err := s.cache.SetSearchResults(ctx, req.Origin, req.Destination, req.Date, flights, 10*time.Minute); err != nil {
		log.Printf("Failed to cache search results: %v", err)
	}

	return &flightv1.SearchFlightsResponse{Flights: flights}, nil
}

func (s *Server) GetFlight(ctx context.Context, req *flightv1.GetFlightRequest) (*flightv1.GetFlightResponse, error) {
	log.Printf("GetFlight: flight_id=%d", req.FlightId)

	// Try cache first
	flight, err := s.cache.GetFlight(ctx, req.FlightId)
	if err != nil {
		log.Printf("Cache error: %v", err)
	}

	if flight != nil {
		log.Printf("Cache HIT for flight: %d", req.FlightId)
		return &flightv1.GetFlightResponse{Flight: flight}, nil
	}

	log.Printf("Cache MISS for flight: %d", req.FlightId)

	// Query database
	flight, err = s.repo.GetFlight(ctx, req.FlightId)
	if err != nil {
		log.Printf("Failed to get flight: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to get flight")
	}

	if flight == nil {
		return nil, status.Errorf(codes.NotFound, "flight not found")
	}

	// Set cache with TTL
	if err := s.cache.SetFlight(ctx, flight, 10*time.Minute); err != nil {
		log.Printf("Failed to cache flight: %v", err)
	}

	return &flightv1.GetFlightResponse{Flight: flight}, nil
}

func (s *Server) ReserveSeats(ctx context.Context, req *flightv1.ReserveSeatsRequest) (*flightv1.ReserveSeatsResponse, error) {
	log.Printf("ReserveSeats: flight_id=%d, seat_count=%d, booking_id=%s", req.FlightId, req.SeatCount, req.BookingId)

	if req.SeatCount <= 0 {
		return nil, status.Errorf(codes.InvalidArgument, "seat_count must be positive")
	}

	if req.BookingId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "booking_id is required")
	}

	// Reserve seats in database
	reservation, err := s.repo.ReserveSeats(ctx, req.FlightId, req.SeatCount, req.BookingId)
	if err != nil {
		log.Printf("Failed to reserve seats: %v", err)
		if err.Error() == "flight not found" {
			return nil, status.Errorf(codes.NotFound, "flight not found")
		}
		if err.Error() == "not enough seats available" {
			return nil, status.Errorf(codes.ResourceExhausted, "not enough seats available")
		}
		return nil, status.Errorf(codes.Internal, "failed to reserve seats")
	}

	// Invalidate cache
	if err := s.cache.InvalidateFlight(ctx, req.FlightId); err != nil {
		log.Printf("Failed to invalidate flight cache: %v", err)
	}

	log.Printf("Seats reserved successfully: reservation_id=%d", reservation.Id)
	return &flightv1.ReserveSeatsResponse{Reservation: reservation}, nil
}

func (s *Server) ReleaseReservation(ctx context.Context, req *flightv1.ReleaseReservationRequest) (*flightv1.ReleaseReservationResponse, error) {
	log.Printf("ReleaseReservation: booking_id=%s", req.BookingId)

	if req.BookingId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "booking_id is required")
	}

	// Release reservation in database
	reservation, err := s.repo.ReleaseReservation(ctx, req.BookingId)
	if err != nil {
		log.Printf("Failed to release reservation: %v", err)
		if err.Error() == "reservation not found" {
			return nil, status.Errorf(codes.NotFound, "reservation not found")
		}
		if err.Error() == "reservation is not active" {
			return nil, status.Errorf(codes.FailedPrecondition, "reservation is not active")
		}
		return nil, status.Errorf(codes.Internal, "failed to release reservation")
	}

	// Invalidate cache
	if err := s.cache.InvalidateFlight(ctx, reservation.FlightId); err != nil {
		log.Printf("Failed to invalidate flight cache: %v", err)
	}

	log.Printf("Reservation released successfully: reservation_id=%d", reservation.Id)
	return &flightv1.ReleaseReservationResponse{Reservation: reservation}, nil
}

func RunMigrations(db *sql.DB) error {
	log.Println("Running database migrations...")

	// Check if flights table exists
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'flights'
		)
	`).Scan(&exists)

	if err != nil {
		return fmt.Errorf("failed to check if migrations needed: %w", err)
	}

	if exists {
		log.Println("Migrations already applied")
		return nil
	}

	// Create tables
	_, err = db.Exec(`
		CREATE TABLE flights (
			id BIGSERIAL PRIMARY KEY,
			flight_number VARCHAR(20) NOT NULL,
			airline VARCHAR(100) NOT NULL,
			origin VARCHAR(3) NOT NULL,
			destination VARCHAR(3) NOT NULL,
			departure_time TIMESTAMP NOT NULL,
			arrival_time TIMESTAMP NOT NULL,
			total_seats INTEGER NOT NULL CHECK (total_seats > 0),
			available_seats INTEGER NOT NULL CHECK (available_seats >= 0 AND available_seats <= total_seats),
			price_kopecks BIGINT NOT NULL CHECK (price_kopecks > 0),
			status VARCHAR(20) NOT NULL DEFAULT 'SCHEDULED' CHECK (status IN ('SCHEDULED', 'DEPARTED', 'CANCELLED', 'COMPLETED')),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE UNIQUE INDEX idx_flights_number_date ON flights(flight_number, departure_time);
		CREATE INDEX idx_flights_origin ON flights(origin);
		CREATE INDEX idx_flights_destination ON flights(destination);
		CREATE INDEX idx_flights_status ON flights(status);

		CREATE TABLE seat_reservations (
			id BIGSERIAL PRIMARY KEY,
			flight_id BIGINT NOT NULL REFERENCES flights(id) ON DELETE CASCADE,
			booking_id VARCHAR(100) NOT NULL UNIQUE,
			seat_count INTEGER NOT NULL CHECK (seat_count > 0),
			status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'RELEASED', 'EXPIRED')),
			price_kopecks BIGINT NOT NULL CHECK (price_kopecks > 0),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX idx_seat_reservations_flight_id ON seat_reservations(flight_id);
		CREATE INDEX idx_seat_reservations_booking_id ON seat_reservations(booking_id);
		CREATE INDEX idx_seat_reservations_status ON seat_reservations(status);
	`)

	if err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("Migrations completed successfully")
	return nil
}
