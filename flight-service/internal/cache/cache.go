package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	flightv1 "github.com/flight-booking/flight-service/gen/flight/v1"
	"github.com/go-redis/redis/v8"
)

type Cache struct {
	client *redis.Client
}

func NewCache(redisURL string) (*Cache, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &Cache{client: client}, nil
}

func (c *Cache) GetFlight(ctx context.Context, flightID int64) (*flightv1.Flight, error) {
	key := fmt.Sprintf("flight:%d", flightID)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get flight from cache: %w", err)
	}

	var flight flightv1.Flight
	if err := json.Unmarshal(data, &flight); err != nil {
		return nil, fmt.Errorf("failed to unmarshal flight: %w", err)
	}

	return &flight, nil
}

func (c *Cache) SetFlight(ctx context.Context, flight *flightv1.Flight, ttl time.Duration) error {
	key := fmt.Sprintf("flight:%d", flight.Id)
	data, err := json.Marshal(flight)
	if err != nil {
		return fmt.Errorf("failed to marshal flight: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set flight in cache: %w", err)
	}

	return nil
}

func (c *Cache) InvalidateFlight(ctx context.Context, flightID int64) error {
	key := fmt.Sprintf("flight:%d", flightID)
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to invalidate flight cache: %w", err)
	}
	return nil
}

func (c *Cache) GetSearchResults(ctx context.Context, origin, destination, date string) ([]*flightv1.Flight, error) {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get search results from cache: %w", err)
	}

	var flights []*flightv1.Flight
	if err := json.Unmarshal(data, &flights); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search results: %w", err)
	}

	return flights, nil
}

func (c *Cache) SetSearchResults(ctx context.Context, origin, destination, date string, flights []*flightv1.Flight, ttl time.Duration) error {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	data, err := json.Marshal(flights)
	if err != nil {
		return fmt.Errorf("failed to marshal search results: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set search results in cache: %w", err)
	}

	return nil
}

func (c *Cache) InvalidateSearch(ctx context.Context, origin, destination, date string) error {
	key := fmt.Sprintf("search:%s:%s:%s", origin, destination, date)
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to invalidate search cache: %w", err)
	}
	return nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}
