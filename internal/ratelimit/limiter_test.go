package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiter_NilSafe(t *testing.T) {
	var l *Limiter
	release, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestLimiter_SemaphoreCapsInFlight(t *testing.T) {
	// 20 goroutines try to acquire; limiter allows max 3 in flight.
	const N = 20
	const cap = 3
	l := New(0, 0, cap)
	var (
		inflight atomic.Int32
		peak     atomic.Int32
		wg       sync.WaitGroup
	)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.Acquire(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			defer release()
			cur := inflight.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			inflight.Add(-1)
		}()
	}
	wg.Wait()
	if got := peak.Load(); int(got) > cap {
		t.Fatalf("inflight exceeded cap: peak=%d cap=%d", got, cap)
	}
}

func TestLimiter_QPSCapsRate(t *testing.T) {
	// 5 QPS, burst 1, no concurrency cap. Asking for 6 tokens should take ~1s.
	l := New(5, 1, 0)
	start := time.Now()
	for i := 0; i < 6; i++ {
		release, err := l.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	elapsed := time.Since(start)
	// 5 QPS means tokens regenerate every 200ms. After the first burst, the
	// remaining 5 acquires should each wait ~200ms => ~1s total.
	if elapsed < 800*time.Millisecond {
		t.Fatalf("QPS not enforced: elapsed=%v", elapsed)
	}
}

func TestLimiter_CtxCancel(t *testing.T) {
	l := New(0, 0, 1)
	rel, err := l.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = l.Acquire(ctx)
	if err == nil {
		t.Fatal("expected ctx cancel error")
	}
}

func TestLimiter_TryAcquire(t *testing.T) {
	l := New(0, 0, 1)
	rel, ok := l.TryAcquire()
	if !ok {
		t.Fatal("first TryAcquire should succeed")
	}
	_, ok2 := l.TryAcquire()
	if ok2 {
		t.Fatal("second TryAcquire should fail (semaphore full)")
	}
	rel()
	rel, ok = l.TryAcquire()
	if !ok {
		t.Fatal("TryAcquire should succeed after release")
	}
	rel()
}
