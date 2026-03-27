package auth

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter tracks failed login attempts per IP and per username using Redis.
// Uses exponential backoff: delay = min(2^failures, maxDelaySec) seconds.
// After maxAttempts failures, hard-blocks for blockDuration.
type RateLimiter struct {
	rdb           *redis.Client
	maxAttempts   int
	maxDelaySec   int
	blockDuration time.Duration
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{
		rdb:           rdb,
		maxAttempts:   10,
		maxDelaySec:   300,
		blockDuration: 1 * time.Hour,
	}
}

func (rl *RateLimiter) ipKey(ip string) string {
	return "packalares:ratelimit:ip:" + ip
}

func (rl *RateLimiter) userKey(username string) string {
	return "packalares:ratelimit:user:" + username
}

// Check returns the delay (in seconds) the caller must wait, or an error if blocked.
func (rl *RateLimiter) Check(ctx context.Context, ip, username string) (delaySec int, err error) {
	ipFails := rl.getFailures(ctx, rl.ipKey(ip))
	userFails := rl.getFailures(ctx, rl.userKey(username))

	maxFails := ipFails
	if userFails > maxFails {
		maxFails = userFails
	}

	if maxFails >= rl.maxAttempts {
		return 0, fmt.Errorf("too many failed attempts, try again later")
	}

	if maxFails == 0 {
		return 0, nil
	}

	delay := int(math.Min(math.Pow(2, float64(maxFails)), float64(rl.maxDelaySec)))
	return delay, nil
}

// RecordFailure increments failure counters for both IP and username.
func (rl *RateLimiter) RecordFailure(ctx context.Context, ip, username string) {
	rl.increment(ctx, rl.ipKey(ip))
	rl.increment(ctx, rl.userKey(username))
}

// Reset clears failure counters on successful login.
func (rl *RateLimiter) Reset(ctx context.Context, ip, username string) {
	rl.rdb.Del(ctx, rl.ipKey(ip))
	rl.rdb.Del(ctx, rl.userKey(username))
}

func (rl *RateLimiter) getFailures(ctx context.Context, key string) int {
	val, err := rl.rdb.Get(ctx, key).Int()
	if err != nil {
		return 0
	}
	return val
}

func (rl *RateLimiter) increment(ctx context.Context, key string) {
	rl.rdb.Incr(ctx, key)
	rl.rdb.Expire(ctx, key, rl.blockDuration)
}
