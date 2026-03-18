import os
import uuid
import logging
from datetime import datetime
from typing import Optional, List
from fastapi import FastAPI, HTTPException, Query
from pydantic import BaseModel
import grpc

from app.database import get_db_connection, run_migrations
from app.grpc_client import flight_client
from app.retry_interceptor import retry_with_backoff
from app.circuit_breaker import circuit_breaker

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="Flight Booking Service")


def flight_status_name(status_int: int) -> str:
    """Convert protobuf FlightStatus integer to string name"""
    mapping = {
        0: "FLIGHT_STATUS_UNSPECIFIED",
        1: "FLIGHT_STATUS_SCHEDULED",
        2: "FLIGHT_STATUS_DEPARTED",
        3: "FLIGHT_STATUS_CANCELLED",
        4: "FLIGHT_STATUS_COMPLETED",
    }
    return mapping.get(status_int, str(status_int))


# Pydantic models
class BookingCreate(BaseModel):
    user_id: str
    flight_id: int
    passenger_name: str
    passenger_email: str
    seat_count: int


class BookingResponse(BaseModel):
    id: int
    booking_uuid: str
    user_id: str
    flight_id: int
    passenger_name: str
    passenger_email: str
    seat_count: int
    total_price_kopecks: int
    status: str
    created_at: str
    updated_at: str


class FlightResponse(BaseModel):
    id: int
    flight_number: str
    airline: str
    origin: str
    destination: str
    departure_time: str
    arrival_time: str
    total_seats: int
    available_seats: int
    price_kopecks: int
    status: str


@app.on_event("startup")
async def startup_event():
    """Run migrations on startup"""
    logger.info("Starting Booking Service...")
    run_migrations()
    logger.info("Booking Service started successfully")


@app.get("/flights", response_model=List[FlightResponse])
async def search_flights(
    origin: str = Query(..., description="Origin airport IATA code"),
    destination: str = Query(..., description="Destination airport IATA code"),
    date: Optional[str] = Query(None, description="Date in YYYY-MM-DD format")
):
    """Search flights by route and optional date"""
    try:
        @retry_with_backoff(max_retries=3, initial_delay=0.1)
        def _search_flights():
            return circuit_breaker.call(
                flight_client.search_flights,
                origin,
                destination,
                date or ""
            )
        
        flights = _search_flights()
        
        return [
            FlightResponse(
                id=f.id,
                flight_number=f.flight_number,
                airline=f.airline,
                origin=f.origin,
                destination=f.destination,
                departure_time=f.departure_time,
                arrival_time=f.arrival_time,
                total_seats=f.total_seats,
                available_seats=f.available_seats,
                price_kopecks=f.price_kopecks,
                status=flight_status_name(f.status)
            )
            for f in flights
        ]
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.UNAVAILABLE:
            logger.error(f"Flight service unavailable: {e}")
            raise HTTPException(status_code=503, detail="Flight service unavailable")
        logger.error(f"Failed to search flights: {e}")
        raise HTTPException(status_code=500, detail="Failed to search flights")


@app.get("/flights/{flight_id}", response_model=FlightResponse)
async def get_flight(flight_id: int):
    """Get flight by ID"""
    try:
        @retry_with_backoff(max_retries=3, initial_delay=0.1)
        def _get_flight():
            return circuit_breaker.call(
                flight_client.get_flight,
                flight_id
            )
        
        flight = _get_flight()
        
        if flight is None:
            raise HTTPException(status_code=404, detail="Flight not found")
        
        return FlightResponse(
            id=flight.id,
            flight_number=flight.flight_number,
            airline=flight.airline,
            origin=flight.origin,
            destination=flight.destination,
            departure_time=flight.departure_time,
            arrival_time=flight.arrival_time,
            total_seats=flight.total_seats,
            available_seats=flight.available_seats,
            price_kopecks=flight.price_kopecks,
            status=flight_status_name(flight.status)
        )
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.NOT_FOUND:
            raise HTTPException(status_code=404, detail="Flight not found")
        if e.code() == grpc.StatusCode.UNAVAILABLE:
            logger.error(f"Flight service unavailable: {e}")
            raise HTTPException(status_code=503, detail="Flight service unavailable")
        logger.error(f"Failed to get flight: {e}")
        raise HTTPException(status_code=500, detail="Failed to get flight")


@app.post("/bookings", response_model=BookingResponse, status_code=201)
async def create_booking(booking: BookingCreate):
    """Create a new booking"""
    booking_uuid = str(uuid.uuid4())
    
    try:
        # Step 1: Get flight information
        @retry_with_backoff(max_retries=3, initial_delay=0.1)
        def _get_flight():
            return circuit_breaker.call(
                flight_client.get_flight,
                booking.flight_id
            )
        
        flight = _get_flight()
        
        if flight is None:
            raise HTTPException(status_code=404, detail="Flight not found")
        
        # Step 2: Reserve seats
        @retry_with_backoff(max_retries=3, initial_delay=0.1)
        def _reserve_seats():
            return circuit_breaker.call(
                flight_client.reserve_seats,
                booking.flight_id,
                booking.seat_count,
                booking_uuid
            )
        
        reservation = _reserve_seats()
        
        # Step 3: Calculate total price (snapshot at booking time)
        total_price = flight.price_kopecks * booking.seat_count
        
        # Step 4: Create booking in database
        with get_db_connection() as conn:
            with conn.cursor() as cur:
                cur.execute("""
                    INSERT INTO bookings (
                        booking_uuid, user_id, flight_id, passenger_name, passenger_email,
                        seat_count, total_price_kopecks, status
                    ) VALUES (%s, %s, %s, %s, %s, %s, %s, 'CONFIRMED')
                    RETURNING id, created_at, updated_at
                """, (
                    booking_uuid,
                    booking.user_id,
                    booking.flight_id,
                    booking.passenger_name,
                    booking.passenger_email,
                    booking.seat_count,
                    total_price
                ))
                result = cur.fetchone()
                conn.commit()
        
        logger.info(f"Booking created: id={result['id']}, booking_uuid={booking_uuid}")
        
        return BookingResponse(
            id=result['id'],
            booking_uuid=booking_uuid,
            user_id=booking.user_id,
            flight_id=booking.flight_id,
            passenger_name=booking.passenger_name,
            passenger_email=booking.passenger_email,
            seat_count=booking.seat_count,
            total_price_kopecks=total_price,
            status="CONFIRMED",
            created_at=result['created_at'].isoformat(),
            updated_at=result['updated_at'].isoformat()
        )
    
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.NOT_FOUND:
            raise HTTPException(status_code=404, detail="Flight not found")
        if e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
            raise HTTPException(status_code=400, detail="Not enough seats available")
        if e.code() == grpc.StatusCode.UNAVAILABLE:
            logger.error(f"Flight service unavailable: {e}")
            raise HTTPException(status_code=503, detail="Flight service unavailable")
        logger.error(f"Failed to create booking: {e}")
        raise HTTPException(status_code=500, detail="Failed to create booking")


@app.get("/bookings/{booking_id}", response_model=BookingResponse)
async def get_booking(booking_id: int):
    """Get booking by ID"""
    with get_db_connection() as conn:
        with conn.cursor() as cur:
            cur.execute("SELECT * FROM bookings WHERE id = %s", (booking_id,))
            booking = cur.fetchone()
    
    if booking is None:
        raise HTTPException(status_code=404, detail="Booking not found")
    
    return BookingResponse(
        id=booking['id'],
        booking_uuid=booking['booking_uuid'],
        user_id=booking['user_id'],
        flight_id=booking['flight_id'],
        passenger_name=booking['passenger_name'],
        passenger_email=booking['passenger_email'],
        seat_count=booking['seat_count'],
        total_price_kopecks=booking['total_price_kopecks'],
        status=booking['status'],
        created_at=booking['created_at'].isoformat(),
        updated_at=booking['updated_at'].isoformat()
    )


@app.post("/bookings/{booking_id}/cancel", response_model=BookingResponse)
async def cancel_booking(booking_id: int):
    """Cancel a booking"""
    with get_db_connection() as conn:
        with conn.cursor() as cur:
            # Get booking
            cur.execute("SELECT * FROM bookings WHERE id = %s", (booking_id,))
            booking = cur.fetchone()
            
            if booking is None:
                raise HTTPException(status_code=404, detail="Booking not found")
            
            if booking['status'] != 'CONFIRMED':
                raise HTTPException(status_code=400, detail="Booking is not in CONFIRMED status")
            
            # Release reservation in flight service using the UUID booking_id
            @retry_with_backoff(max_retries=3, initial_delay=0.1)
            def _release_reservation():
                return circuit_breaker.call(
                    flight_client.release_reservation,
                    booking['booking_uuid']
                )
            
            try:
                _release_reservation()
            except grpc.RpcError as e:
                logger.error(f"Failed to release reservation: {e}")
                # Continue with booking cancellation even if release fails
            
            # Update booking status
            cur.execute("""
                UPDATE bookings 
                SET status = 'CANCELLED', updated_at = NOW() 
                WHERE id = %s
            """, (booking_id,))
            conn.commit()
    
    logger.info(f"Booking cancelled: id={booking_id}")
    
    return BookingResponse(
        id=booking['id'],
        booking_uuid=booking['booking_uuid'],
        user_id=booking['user_id'],
        flight_id=booking['flight_id'],
        passenger_name=booking['passenger_name'],
        passenger_email=booking['passenger_email'],
        seat_count=booking['seat_count'],
        total_price_kopecks=booking['total_price_kopecks'],
        status='CANCELLED',
        created_at=booking['created_at'].isoformat(),
        updated_at=datetime.now().isoformat()
    )


@app.get("/bookings", response_model=List[BookingResponse])
async def get_bookings(user_id: str = Query(..., description="User ID")):
    """Get all bookings for a user"""
    with get_db_connection() as conn:
        with conn.cursor() as cur:
            cur.execute(
                "SELECT * FROM bookings WHERE user_id = %s ORDER BY created_at DESC",
                (user_id,)
            )
            bookings = cur.fetchall()
    
    return [
        BookingResponse(
            id=b['id'],
            booking_uuid=b['booking_uuid'],
            user_id=b['user_id'],
            flight_id=b['flight_id'],
            passenger_name=b['passenger_name'],
            passenger_email=b['passenger_email'],
            seat_count=b['seat_count'],
            total_price_kopecks=b['total_price_kopecks'],
            status=b['status'],
            created_at=b['created_at'].isoformat(),
            updated_at=b['updated_at'].isoformat()
        )
        for b in bookings
    ]


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {"status": "healthy"}
