CREATE TABLE bookings (
    id BIGSERIAL PRIMARY KEY,
    booking_uuid VARCHAR(100) NOT NULL UNIQUE,
    user_id VARCHAR(100) NOT NULL,
    flight_id BIGINT NOT NULL,
    passenger_name VARCHAR(200) NOT NULL,
    passenger_email VARCHAR(200) NOT NULL,
    seat_count INTEGER NOT NULL CHECK (seat_count > 0),
    total_price_kopecks BIGINT NOT NULL CHECK (total_price_kopecks > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'CONFIRMED' CHECK (status IN ('CONFIRMED', 'CANCELLED')),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bookings_user_id ON bookings(user_id);
CREATE INDEX idx_bookings_flight_id ON bookings(flight_id);
CREATE INDEX idx_bookings_status ON bookings(status);
CREATE INDEX idx_bookings_booking_uuid ON bookings(booking_uuid);
