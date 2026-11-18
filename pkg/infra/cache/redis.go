package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bsm/redislock"
	"github.com/redis/go-redis/v9"
)

func init() {
	Register("redis", func(ctx context.Context, cfg *Config) (Interface, error) {
		client := redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		})
		if err := client.Ping(ctx).Err(); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("redis ping (%s): %w", cfg.Addr, err)
		}
		return &redisCache{client: client, locker: redislock.New(client)}, nil
	})
}

type redisCache struct {
	client *redis.Client
	locker *redislock.Client
}

func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", err
	}
	return val, nil
}

func (r *redisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *redisCache) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *redisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *redisCache) Close() error {
	return r.client.Close()
}

func (r *redisCache) RedisClient() *redis.Client {
	return r.client
}

func (r *redisCache) TryLock(ctx context.Context, key string, ttl time.Duration) (func(), bool) {
	if r.locker == nil {
		return func() {}, true
	}
	lockCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	lock, err := r.locker.Obtain(lockCtx, key, ttl, nil)
	if err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			return nil, false
		}
		return func() {}, true
	}
	return func() {
		relCtx, relCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer relCancel()
		_ = lock.Release(relCtx)
	}, true
}
