package main

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

type RateLimiter struct {
	limiter *rate.Limiter
	mutex   sync.Mutex
}

func NewRateLimiter(r float64, b int) *RateLimiter {
	return &RateLimiter{
		limiter: rate.NewLimiter(rate.Limit(r), b),
	}
}

func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	return rl.limiter.Wait(ctx)
}
