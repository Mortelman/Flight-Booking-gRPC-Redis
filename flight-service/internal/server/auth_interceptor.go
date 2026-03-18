package server

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const apiKeyHeader = "x-api-key"

type authInterceptor struct {
	apiKey string
}

func NewAuthInterceptor(apiKey string) *authInterceptor {
	return &authInterceptor{apiKey: apiKey}
}

func (a *authInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if err := a.authorize(ctx); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

func (a *authInterceptor) authorize(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "missing context metadata")
	}

	apiKeys := md[apiKeyHeader]
	if len(apiKeys) == 0 {
		return status.Errorf(codes.Unauthenticated, "missing API key")
	}

	if apiKeys[0] != a.apiKey {
		log.Printf("Invalid API key attempt: %s", apiKeys[0])
		return status.Errorf(codes.Unauthenticated, "invalid API key")
	}

	return nil
}
