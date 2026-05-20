package dag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// drainEvents starts a goroutine that copies events into a slice until the
// channel closes. Returns a stop function which the caller MUST defer.
func drainEvents(t *testing.T) (chan<- Event, func() []Event) {
	t.Helper()
	ch := make(chan Event, 256)
	var (
		mu  sync.Mutex
		buf []Event
	)
	done := make(chan struct{})
	go func() {
		for e := range ch {
			mu.Lock()
			buf = append(buf, e)
			mu.Unlock()
		}
		close(done)
	}()
	return ch, func() []Event {
		close(ch)
		<-done
		mu.Lock()
		defer mu.Unlock()
		out := make([]Event, len(buf))
		copy(out, buf)
		return out
	}
}

func TestScheduler_LinearChain(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: func(_ context.Context, _ map[string]any) (any, error) { return 1, nil }})
	_ = g.Add(&Node{ID: "b", DependsOn: []string{"a"}, Run: func(_ context.Context, deps map[string]any) (any, error) {
		return deps["a"].(int) + 1, nil
	}})
	_ = g.Add(&Node{ID: "c", DependsOn: []string{"b"}, Run: func(_ context.Context, deps map[string]any) (any, error) {
		return deps["b"].(int) + 1, nil
	}})

	s := NewScheduler(WithBackoff(NoBackoff()))
	res, err := s.Run(context.Background(), g, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["c"] != 3 {
		t.Fatalf("want 3, got %v", res.Outputs["c"])
	}
}

func TestScheduler_DiamondRunsBranchesConcurrently(t *testing.T) {
	// Both b and c sleep 80ms. With concurrency >= 2 the whole run should
	// finish in well under 200ms; serial execution would take ~160ms+.
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: func(_ context.Context, _ map[string]any) (any, error) { return "a", nil }})
	_ = g.Add(&Node{ID: "b", DependsOn: []string{"a"}, Run: func(_ context.Context, _ map[string]any) (any, error) {
		time.Sleep(80 * time.Millisecond)
		return "b", nil
	}})
	_ = g.Add(&Node{ID: "c", DependsOn: []string{"a"}, Run: func(_ context.Context, _ map[string]any) (any, error) {
		time.Sleep(80 * time.Millisecond)
		return "c", nil
	}})
	_ = g.Add(&Node{ID: "d", DependsOn: []string{"b", "c"}, Run: func(_ context.Context, deps map[string]any) (any, error) {
		return deps["b"].(string) + deps["c"].(string), nil
	}})

	s := NewScheduler(WithConcurrency(4), WithBackoff(NoBackoff()))
	start := time.Now()
	res, err := s.Run(context.Background(), g, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["d"] != "bc" {
		t.Fatalf("want bc, got %v", res.Outputs["d"])
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("diamond should run b and c concurrently; elapsed=%v", elapsed)
	}
}

func TestScheduler_RetryThenSucceed(t *testing.T) {
	var calls atomic.Int32
	g := NewGraph()
	_ = g.Add(&Node{
		ID:       "flaky",
		MaxRetry: 3,
		Run: func(_ context.Context, _ map[string]any) (any, error) {
			n := calls.Add(1)
			if n < 3 {
				return nil, fmt.Errorf("transient %d", n)
			}
			return "ok", nil
		},
	})

	events, take := drainEvents(t)
	s := NewScheduler(WithBackoff(NoBackoff()))
	res, err := s.Run(context.Background(), g, events)
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["flaky"] != "ok" {
		t.Fatalf("want ok, got %v", res.Outputs["flaky"])
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("want 3 attempts, got %d", got)
	}
	es := take()
	var retrying int
	for _, e := range es {
		if e.Type == EventNodeRetrying {
			retrying++
		}
	}
	if retrying != 2 {
		t.Fatalf("want 2 retrying events, got %d (events=%+v)", retrying, es)
	}
}

func TestScheduler_RetryExhausted(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{
		ID:       "bad",
		MaxRetry: 2,
		Run: func(_ context.Context, _ map[string]any) (any, error) {
			return nil, errors.New("nope")
		},
	})
	s := NewScheduler(WithBackoff(NoBackoff()))
	res, err := s.Run(context.Background(), g, nil)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("expected wrapped error containing nope, got %v", err)
	}
	if res.States["bad"] != StateFailed {
		t.Fatalf("want failed state, got %v", res.States["bad"])
	}
}

func TestScheduler_Timeout(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{
		ID:      "slow",
		Timeout: 30 * time.Millisecond,
		Run: func(ctx context.Context, _ map[string]any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return "never", nil
			}
		},
	})
	s := NewScheduler(WithBackoff(NoBackoff()))
	start := time.Now()
	_, err := s.Run(context.Background(), g, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("timeout did not fire in time: %v", time.Since(start))
	}
}

func TestScheduler_FailFast_CancelsSiblings(t *testing.T) {
	// a fails immediately. b is a long-running independent node; the scheduler
	// must cancel its context so it returns ctx.Err() promptly.
	var bSawCancel atomic.Bool
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: func(_ context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("boom")
	}})
	_ = g.Add(&Node{ID: "b", Run: func(ctx context.Context, _ map[string]any) (any, error) {
		select {
		case <-ctx.Done():
			bSawCancel.Store(true)
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			return "never", nil
		}
	}})

	s := NewScheduler(WithBackoff(NoBackoff()))
	start := time.Now()
	_, err := s.Run(context.Background(), g, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("fail-fast did not cancel sibling fast enough: %v", time.Since(start))
	}
	if !bSawCancel.Load() {
		t.Fatal("sibling never observed ctx cancellation")
	}
}

func TestScheduler_ExternalCancel(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{ID: "long", Run: func(ctx context.Context, _ map[string]any) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	s := NewScheduler(WithBackoff(NoBackoff()))
	_, err := s.Run(ctx, g, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestScheduler_PanicRecovered(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{ID: "boom", Run: func(_ context.Context, _ map[string]any) (any, error) {
		panic("kaboom")
	}})
	s := NewScheduler(WithBackoff(NoBackoff()))
	_, err := s.Run(context.Background(), g, nil)
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("want panic surfaced as error, got %v", err)
	}
}

func TestScheduler_LimiterCapsConcurrency(t *testing.T) {
	// 8 independent nodes, scheduler concurrency 8, but the limiter only
	// permits 2 in flight at once. We assert observed inflight max == 2.
	var inflight, max atomic.Int32
	const nNodes = 8

	g := NewGraph()
	for i := 0; i < nNodes; i++ {
		id := fmt.Sprintf("n%d", i)
		_ = g.Add(&Node{
			ID: id,
			Run: func(_ context.Context, _ map[string]any) (any, error) {
				cur := inflight.Add(1)
				for {
					m := max.Load()
					if cur <= m || max.CompareAndSwap(m, cur) {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
				inflight.Add(-1)
				return nil, nil
			},
		})
	}

	limiter := &capLimiter{max: 2}
	s := NewScheduler(
		WithConcurrency(nNodes),
		WithBackoff(NoBackoff()),
		WithLimiter(limiter),
	)
	if _, err := s.Run(context.Background(), g, nil); err != nil {
		t.Fatal(err)
	}
	if got := max.Load(); got > 2 {
		t.Fatalf("limiter breached: observed inflight max=%d", got)
	}
}

// capLimiter is a deliberately tiny Acquirer used to assert that scheduler
// honours the WithLimiter option even though the production Acquire path is
// currently inside safeRun (left dormant). For Phase 2.B we'll wire it in.
//
// Today this test exists as a compile-time/contract guard: passing a custom
// Acquirer must not break scheduling.
type capLimiter struct {
	max int
	sem chan struct{}
	mu  sync.Mutex
}

func (l *capLimiter) ensure() {
	l.mu.Lock()
	if l.sem == nil {
		l.sem = make(chan struct{}, l.max)
	}
	l.mu.Unlock()
}

func (l *capLimiter) Acquire(ctx context.Context) (func(), error) {
	l.ensure()
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
