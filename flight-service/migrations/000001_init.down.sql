DROP INDEX IF EXISTS idx_seat_reservations_status;
DROP INDEX IF EXISTS idx_seat_reservations_booking_id;
DROP INDEX IF EXISTS idx_seat_reservations_flight_id;
DROP TABLE IF EXISTS seat_reservations;

DROP INDEX IF EXISTS idx_flights_status;
DROP INDEX IF EXISTS idx_flights_destination;
DROP INDEX IF EXISTS idx_flights_origin;
DROP INDEX IF EXISTS idx_flights_number_date;
DROP TABLE IF EXISTS flights;
