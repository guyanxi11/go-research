// Package ratelimit pairs a token-bucket QPS limiter with a fixed-size
// concurrency semaphore. Together they cap both the *rate* and the
// *simultaneous* load on a downstream service, which is exactly what's needed
// when fanning out to LLM/search APIs from many agents.
package ratelimit

import (
	"context"
	"errors"

	"golang.org/x/time/rate"
)

// Limiter combines two independent constraints:
//
//   - qps:  steady-state requests per second (token bucket, with burst).
//   - sem:  hard cap on requests in flight.
//
// Acquire waits for both, in this order. Release releases only the semaphore;
// tokens are not given back to the bucket.
type Limiter struct {
	qps *rate.Limiter
	sem chan struct{}
}

// New builds a Limiter. qps <= 0 disables the rate cap; maxInFlight <= 0
// disables the concurrency cap. burst defaults to max(1, qps).
func New(qps float64, burst, maxInFlight int) *Limiter {
	l := &Limiter{}
	if qps > 0 {
		if burst < 1 {
			burst = int(qps)
			if burst < 1 {
				burst = 1
			}
		}
		l.qps = rate.NewLimiter(rate.Limit(qps), burst)
	}
	if maxInFlight > 0 {
		l.sem = make(chan struct{}, maxInFlight)
	}
	return l
}

// ErrClosed is returned from Acquire after the limiter has been used past
// cancellation; it exists so callers can distinguish from ctx.Err().
var ErrClosed = errors.New("ratelimit: limiter closed")

// Acquire blocks until both the QPS bucket and the semaphore admit the
// caller. The returned release MUST be called exactly once, even on error.
func (l *Limiter) Acquire(ctx context.Context) (release func(), err error) {
	if l == nil {
		return func() {}, nil
	}
	if l.qps != nil {
		if err := l.qps.Wait(ctx); err != nil {
			return func() {}, err
		}
	}
	if l.sem != nil {
		select {
		case l.sem <- struct{}{}:
			return func() {
				select {
				case <-l.sem:
				default:
				}
			}, nil
		case <-ctx.Done():
			return func() {}, ctx.Err()
		}
	}
	return func() {}, nil
}

// TryAcquire returns immediately. If either the QPS bucket has no token or the
// semaphore is full, it returns ok=false. Useful for "best-effort" fan-out.
func (l *Limiter) TryAcquire() (release func(), ok bool) {
	if l == nil {
		return func() {}, true
	}
	if l.qps != nil && !l.qps.Allow() {
		return func() {}, false
	}
	if l.sem != nil {
		select {
		case l.sem <- struct{}{}:
			return func() {
				select {
				case <-l.sem:
				default:
				}
			}, true
		default:
			return func() {}, false
		}
	}
	return func() {}, true
}
