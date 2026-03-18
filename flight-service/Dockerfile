FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install protoc and tools
RUN apk add --no-cache protobuf-dev git

RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.31.0
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0
ENV PATH="/root/go/bin:${PATH}"

# Copy go.mod and download dependencies first (for caching)
COPY flight-service/go.mod flight-service/go.sum ./
RUN go mod download

# Copy proto files and generate Go code
COPY proto /proto
RUN mkdir -p /tmp/proto_out gen/flight/v1 && \
  protoc -I /proto \
  --go_out=/tmp/proto_out \
  --go_opt=paths=source_relative \
  --go-grpc_out=/tmp/proto_out \
  --go-grpc_opt=paths=source_relative \
  flight/v1/flight.proto && \
  mv /tmp/proto_out/flight/v1/*.pb.go gen/flight/v1/

# Copy source code
COPY flight-service/cmd ./cmd
COPY flight-service/internal ./internal
COPY flight-service/migrations ./migrations

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o flight-service ./cmd/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/flight-service .
COPY --from=builder /app/migrations ./migrations

EXPOSE 50051

CMD ["./flight-service"]
