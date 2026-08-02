package redis

import (
	"context"
	"fmt"
	"log"

	goredis "github.com/redis/go-redis/v9"
)

// NewClient connects to Redis at addr and pings it to verify connectivity.
func NewClient(addr string) (*goredis.Client, error) {
	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	log.Printf("connected to redis at %s", addr)

	return rdb, nil
}
