package main

import (
	"database/sql"
	"flag"
	"log"
	"net"
	"os"

	flightv1 "github.com/flight-booking/flight-service/gen/flight/v1"
	"github.com/flight-booking/flight-service/internal/cache"
	"github.com/flight-booking/flight-service/internal/server"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
)

func main() {
	grpcPort := flag.String("grpc-port", getEnv("GRPC_PORT", "50051"), "gRPC server port")
	databaseURL := flag.String("database-url", getEnv("DATABASE_URL", ""), "Database connection URL")
	redisURL := flag.String("redis-url", getEnv("REDIS_URL", "redis://localhost:6379"), "Redis connection URL")
	apiKey := flag.String("api-key", getEnv("API_KEY", "secret-key"), "API key for authentication")
	flag.Parse()

	if *databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Connect to database
	db, err := sql.Open("postgres", *databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to database")

	// Run migrations
	if err := server.RunMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Connect to Redis
	redisCache, err := cache.NewCache(*redisURL)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()
	log.Println("Connected to Redis")

	// Create gRPC server with auth interceptor
	authInterceptor := server.NewAuthInterceptor(*apiKey)
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(authInterceptor.Unary()),
	)

	// Register flight service
	flightServer := server.NewServer(db, redisCache)
	flightv1.RegisterFlightServiceServer(grpcServer, flightServer)

	// Start server
	lis, err := net.Listen("tcp", ":"+*grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Printf("Flight Service starting on port %s", *grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
