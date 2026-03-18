import os
import logging
import grpc
from typing import Optional
import flight.v1.flight_pb2 as flight_pb2
import flight.v1.flight_pb2_grpc as flight_pb2_grpc

logger = logging.getLogger(__name__)

FLIGHT_SERVICE_URL = os.getenv("FLIGHT_SERVICE_URL", "flight-service:50051")
API_KEY = os.getenv("API_KEY", "secret-key")


class FlightServiceClient:
    def __init__(self):
        self.channel = grpc.insecure_channel(FLIGHT_SERVICE_URL)
        self.stub = flight_pb2_grpc.FlightServiceStub(self.channel)
    
    def _add_metadata(self):
        return [("x-api-key", API_KEY)]
    
    def search_flights(self, origin: str, destination: str, date: Optional[str] = None):
        """Search flights by route and optional date"""
        try:
            request = flight_pb2.SearchFlightsRequest(
                origin=origin,
                destination=destination,
                date=date or ""
            )
            response = self.stub.SearchFlights(request, metadata=self._add_metadata())
            return response.flights
        except grpc.RpcError as e:
            logger.error(f"Failed to search flights: {e}")
            raise
    
    def get_flight(self, flight_id: int):
        """Get flight by ID"""
        try:
            request = flight_pb2.GetFlightRequest(flight_id=flight_id)
            response = self.stub.GetFlight(request, metadata=self._add_metadata())
            return response.flight
        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.NOT_FOUND:
                return None
            logger.error(f"Failed to get flight: {e}")
            raise
    
    def reserve_seats(self, flight_id: int, seat_count: int, booking_id: str):
        """Reserve seats for a booking"""
        try:
            request = flight_pb2.ReserveSeatsRequest(
                flight_id=flight_id,
                seat_count=seat_count,
                booking_id=booking_id
            )
            response = self.stub.ReserveSeats(request, metadata=self._add_metadata())
            return response.reservation
        except grpc.RpcError as e:
            logger.error(f"Failed to reserve seats: {e}")
            raise
    
    def release_reservation(self, booking_id: str):
        """Release a reservation"""
        try:
            request = flight_pb2.ReleaseReservationRequest(booking_id=booking_id)
            response = self.stub.ReleaseReservation(request, metadata=self._add_metadata())
            return response.reservation
        except grpc.RpcError as e:
            logger.error(f"Failed to release reservation: {e}")
            raise
    
    def close(self):
        self.channel.close()


# Global client instance
flight_client = FlightServiceClient()
