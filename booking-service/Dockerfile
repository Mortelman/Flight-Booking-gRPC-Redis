FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Copy proto files and generate Python code
COPY proto /proto
RUN pip install grpcio-tools==1.59.3
RUN python -m grpc_tools.protoc -I /proto \
    --python_out=. \
    --grpc_python_out=. \
    /proto/flight/v1/flight.proto

# Copy requirements and install
COPY booking-service/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source code
COPY booking-service/ .

# Run migrations on startup
COPY booking-service/migrations /migrations

EXPOSE 8000

CMD ["python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
