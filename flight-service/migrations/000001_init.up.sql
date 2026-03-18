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
