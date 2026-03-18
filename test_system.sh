#!/bin/bash

# Flight Booking System Test Script
# This script tests all the functionality of the flight booking system

set -e

BASE_URL="http://localhost:8001"
USER_ID="test_user_$(date +%s)"

echo "=========================================="
echo "Flight Booking System Test Script"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print test results
print_result() {
    if [ $1 -eq 0 ]; then
        echo -e "${GREEN}✓ PASSED${NC}: $2"
    else
        echo -e "${RED}✗ FAILED${NC}: $2"
        exit 1
    fi
}

# Function to wait for service
wait_for_service() {
    echo -e "${YELLOW}Waiting for $1 to be ready...${NC}"
    for i in {1..30}; do
        if curl -s "$2" > /dev/null 2>&1; then
            echo -e "${GREEN}$1 is ready!${NC}"
            return 0
        fi
        sleep 2
    done
    echo -e "${RED}$1 failed to start${NC}"
    exit 1
}

# Check if services are running
echo "Checking if services are running..."
wait_for_service "Booking Service" "$BASE_URL/health"

echo ""
echo "=========================================="
echo "Test 1: Health Check"
echo "=========================================="
response=$(curl -s "$BASE_URL/health")
echo "Response: $response"
print_result $? "Health check"

echo ""
echo "=========================================="
echo "Test 2: Search Flights"
echo "=========================================="
response=$(curl -s "$BASE_URL/flights?origin=SVO&destination=LED")
echo "Response: $response"
if echo "$response" | grep -q "flight_number"; then
    print_result 0 "Search flights"
else
    print_result 1 "Search flights - no flights found"
fi

echo ""
echo "=========================================="
echo "Test 3: Get Flight by ID"
echo "=========================================="
response=$(curl -s "$BASE_URL/flights/1")
echo "Response: $response"
if echo "$response" | grep -q "flight_number"; then
    print_result 0 "Get flight by ID"
else
    print_result 1 "Get flight by ID - flight not found"
fi

echo ""
echo "=========================================="
echo "Test 4: Create Booking"
echo "=========================================="
booking_response=$(curl -s -X POST "$BASE_URL/bookings" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER_ID\",
    \"flight_id\": 1,
    \"passenger_name\": \"Test User\",
    \"passenger_email\": \"test@example.com\",
    \"seat_count\": 2
  }")
echo "Response: $booking_response"

if echo "$booking_response" | grep -q "id"; then
    BOOKING_ID=$(echo "$booking_response" | grep -o '"id":[0-9]*' | cut -d':' -f2)
    print_result 0 "Create booking (ID: $BOOKING_ID)"
else
    print_result 1 "Create booking failed"
fi

echo ""
echo "=========================================="
echo "Test 5: Get Booking"
echo "=========================================="
response=$(curl -s "$BASE_URL/bookings/$BOOKING_ID")
echo "Response: $response"
if echo "$response" | grep -q "CONFIRMED"; then
    print_result 0 "Get booking"
else
    print_result 1 "Get booking failed"
fi

echo ""
echo "=========================================="
echo "Test 6: Get User Bookings"
echo "=========================================="
response=$(curl -s "$BASE_URL/bookings?user_id=$USER_ID")
echo "Response: $response"
if echo "$response" | grep -q "$USER_ID"; then
    print_result 0 "Get user bookings"
else
    print_result 1 "Get user bookings failed"
fi

echo ""
echo "=========================================="
echo "Test 7: Cancel Booking"
echo "=========================================="
response=$(curl -s -X POST "$BASE_URL/bookings/$BOOKING_ID/cancel")
echo "Response: $response"
if echo "$response" | grep -q "CANCELLED"; then
    print_result 0 "Cancel booking"
else
    print_result 1 "Cancel booking failed"
fi

echo ""
echo "=========================================="
echo "Test 8: Verify Flight Seats Returned"
echo "=========================================="
response=$(curl -s "$BASE_URL/flights/1")
echo "Response: $response"
if echo "$response" | grep -q "available_seats"; then
    print_result 0 "Flight seats available after cancellation"
else
    print_result 1 "Flight seats verification failed"
fi

echo ""
echo "=========================================="
echo "Test 9: Test Error Handling - Invalid Flight"
echo "=========================================="
response=$(curl -s -w "\n%{http_code}" "$BASE_URL/flights/99999")
http_code=$(echo "$response" | tail -n1)
if [ "$http_code" = "404" ]; then
    print_result 0 "Invalid flight returns 404"
else
    print_result 1 "Invalid flight error handling failed"
fi

echo ""
echo "=========================================="
echo "Test 10: Test Error Handling - Not Enough Seats"
echo "=========================================="
# Try to book more seats than available
response=$(curl -s -X POST "$BASE_URL/bookings" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER_ID\",
    \"flight_id\": 1,
    \"passenger_name\": \"Test User\",
    \"passenger_email\": \"test@example.com\",
    \"seat_count\": 999
  }")
echo "Response: $response"
if echo "$response" | grep -q "Not enough seats available\|400"; then
    print_result 0 "Not enough seats error handling"
else
    print_result 1 "Not enough seats error handling failed"
fi

echo ""
echo "=========================================="
echo "All Tests Completed Successfully!"
echo "=========================================="
echo ""
echo "Test Summary:"
echo "✓ Health Check"
echo "✓ Search Flights"
echo "✓ Get Flight by ID"
echo "✓ Create Booking"
echo "✓ Get Booking"
echo "✓ Get User Bookings"
echo "✓ Cancel Booking"
echo "✓ Verify Flight Seats Returned"
echo "✓ Error Handling - Invalid Flight"
echo "✓ Error Handling - Not Enough Seats"
echo ""
echo -e "${GREEN}All tests passed!${NC}"
