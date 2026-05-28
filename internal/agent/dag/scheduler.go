package dag

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/yourname/go-research/internal/metrics"
	"github.com/yourname/go-research/internal/tracing"
)

// BackoffFn returns the delay before retrying after `attempt` failed attempts
// (attempt is 1-based: 1 means the first failure).
type BackoffFn func(attempt int) time.Duration

// ExponentialBackoff doubles the base delay each attempt, capped at max.
func ExponentialBackoff(base, max time.Duration) BackoffFn {
	return func(attempt int) time.Duration {
		if attempt < 1 {
			attempt = 1
		}
		d := base << (attempt - 1)
		if d <= 0 || d > max {
			d = max
		}
		return d
	}
}

// NoBackoff retries immediately. Useful in unit tests.
func NoBackoff() BackoffFn {
	return func(int) time.Duration { return 0 }
}

// Option mutates a Scheduler at construction time.
type Option func(*Scheduler)

func WithConcurrency(n int) Option {
	return func(s *Scheduler) {
		if n > 0 {
			s.concurrency = n
		}
	}
}

func WithBackoff(fn BackoffFn) Option {
	return func(s *Scheduler) {
		if fn != nil {
			s.backoff = fn
		}
	}
}

// Acquirer is anything that can rate-limit node execution. Pass a
// *ratelimit.Limiter here in production.
type Acquirer interface {
	Acquire(ctx context.Context) (release func(), err error)
}

// WithLimiter throttles every node attempt through the given acquirer.
func WithLimiter(a Acquirer) Option {
	return func(s *Scheduler) { s.limiter = a }
}

// Scheduler runs a Graph honouring dependencies, concurrency, retries and
// timeouts. It is safe to reuse across Runs but each Run is independent.
type Scheduler struct {
	concurrency int
	backoff     BackoffFn
	limiter     Acquirer
}

func NewScheduler(opts ...Option) *Scheduler {
	s := &Scheduler{
		concurrency: 4,
		backoff:     ExponentialBackoff(50*time.Millisecond, 2*time.Second),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// task is the unit handed to workers. deps is a snapshot taken by the
// coordinator at dispatch time so workers never touch shared state.
type task struct {
	node *Node
	deps map[string]any
}

// doneMsg is what a worker sends back when an attempt sequence finishes.
type doneMsg struct {
	nodeID  string
	out     any
	err     error
	attempt int
	elapsed time.Duration
}

// Run executes the graph. Events are emitted on `events` if non-nil; the
// channel is NOT closed by the scheduler (the caller may want to multiplex).
// On context cancellation, in-flight nodes are signalled via ctx and the
// function returns ctx.Err().
func (s *Scheduler) Run(ctx context.Context, g *Graph, events chan<- Event) (*Result, error) {
	if g == nil {
		return nil, errors.New("dag: nil graph")
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	nodes := g.Nodes()
	if len(nodes) == 0 {
		return &Result{Outputs: map[string]any{}, States: map[string]State{}}, nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	states := make(map[string]State, len(nodes))
	outputs := make(map[string]any, len(nodes))
	pending := make(map[string]map[string]bool, len(nodes))
	for _, n := range nodes {
		states[n.ID] = StatePending
		pending[n.ID] = make(map[string]bool, len(n.DependsOn))
		for _, d := range n.DependsOn {
			pending[n.ID][d] = true
		}
	}

	taskCh := make(chan task, len(nodes))
	doneCh := make(chan doneMsg, len(nodes))

	var workerWg sync.WaitGroup
	for i := 0; i < s.concurrency; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			s.worker(runCtx, taskCh, doneCh, events)
		}()
	}
	// IMPORTANT: cancel() must run *before* close(taskCh) so any worker stuck
	// inside execute() observes ctx.Done() promptly. Then close(taskCh) lets
	// the for-range in worker() exit, and Wait() guarantees that no goroutine
	// outlives Run(). Deferreds fire LIFO, so the order below is correct.
	defer workerWg.Wait()
	defer close(taskCh)

	// Seed the ready queue.
	for _, n := range nodes {
		if len(pending[n.ID]) == 0 {
			states[n.ID] = StateReady
			taskCh <- task{node: n, deps: map[string]any{}}
		}
	}

	remaining := len(nodes)
	var firstErr error

	for remaining > 0 {
		select {
		case <-runCtx.Done():
			// Mark anything still pending as cancelled for the final Result.
			for id, st := range states {
				if st == StatePending || st == StateReady {
					states[id] = StateCancelled
					emit(events, Event{Type: EventNodeCancelled, NodeID: id, Name: g.byID[id].Name, Error: runCtx.Err()})
				}
			}
			if firstErr == nil {
				firstErr = runCtx.Err()
			}
			return &Result{Outputs: outputs, States: states}, firstErr

		case msg := <-doneCh:
			remaining--
			if msg.err != nil {
				states[msg.nodeID] = StateFailed
				if firstErr == nil {
					firstErr = fmt.Errorf("node %q: %w", msg.nodeID, msg.err)
				}
				cancel() // fail-fast: stop the rest
				continue
			}
			states[msg.nodeID] = StateSucceeded
			outputs[msg.nodeID] = msg.out
			// Promote dependants whose last dep just resolved.
			for _, n := range nodes {
				if states[n.ID] != StatePending {
					continue
				}
				if _, depends := pending[n.ID][msg.nodeID]; depends {
					delete(pending[n.ID], msg.nodeID)
				}
				if len(pending[n.ID]) == 0 {
					states[n.ID] = StateReady
					depSnap := make(map[string]any, len(n.DependsOn))
					for _, d := range n.DependsOn {
						depSnap[d] = outputs[d]
					}
					select {
					case taskCh <- task{node: n, deps: depSnap}:
					case <-runCtx.Done():
					}
				}
			}
		}
	}
	return &Result{Outputs: outputs, States: states}, firstErr
}

// worker pops tasks until the channel is closed.
func (s *Scheduler) worker(ctx context.Context, in <-chan task, out chan<- doneMsg, events chan<- Event) {
	for t := range in {
		s.execute(ctx, t, out, events)
	}
}

// execute drives the retry loop for a single node. It writes exactly one
// doneMsg, regardless of outcome.
func (s *Scheduler) execute(ctx context.Context, t task, out chan<- doneMsg, events chan<- Event) {
	n := t.node
	var (
		lastErr error
		final   doneMsg
	)
	final.nodeID = n.ID

	metrics.DAGNodesInFlight.Inc()
	// Defer the outcome counter so every exit path (success / failure /
	// limiter error / ctx cancel) reports exactly once.
	defer func() {
		metrics.DAGNodesInFlight.Dec()
		outcome := "ok"
		if final.err != nil {
			if errors.Is(final.err, context.Canceled) || errors.Is(final.err, context.DeadlineExceeded) {
				outcome = "canceled"
			} else {
				outcome = "failed"
			}
		}
		metrics.DAGNodesTotal.WithLabelValues(outcome).Inc()
	}()

	for attempt := 1; attempt <= 1+n.MaxRetry; attempt++ {
		select {
		case <-ctx.Done():
			final.err = ctx.Err()
			final.attempt = attempt
			out <- final
			return
		default:
		}

		// Throttle through the scheduler's limiter, if any. Tokens are held
		// for the duration of one attempt and released even on panic.
		var release func()
		if s.limiter != nil {
			r, lerr := s.limiter.Acquire(ctx)
			if lerr != nil {
				final.err = lerr
				final.attempt = attempt
				out <- final
				return
			}
			release = r
		}

		emit(events, Event{Type: EventNodeStarted, NodeID: n.ID, Name: n.Name, Attempt: attempt})
		start := time.Now()

		runCtx := ctx
		var cancel context.CancelFunc
		if n.Timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, n.Timeout)
		}

		result, err := safeRun(runCtx, n, t.deps)
		if cancel != nil {
			cancel()
		}
		if release != nil {
			release()
		}
		elapsed := time.Since(start)

		if err == nil {
			emit(events, Event{Type: EventNodeSucceeded, NodeID: n.ID, Name: n.Name, Attempt: attempt, Result: result, Elapsed: elapsed})
			final.out = result
			final.attempt = attempt
			final.elapsed = elapsed
			out <- final
			return
		}

		lastErr = err
		if attempt > n.MaxRetry {
			emit(events, Event{Type: EventNodeFailed, NodeID: n.ID, Name: n.Name, Attempt: attempt, Error: err, Elapsed: elapsed})
			break
		}
		emit(events, Event{Type: EventNodeRetrying, NodeID: n.ID, Name: n.Name, Attempt: attempt, Error: err, Elapsed: elapsed})

		// Backoff with ctx-aware sleep.
		d := s.backoff(attempt)
		if d > 0 {
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				final.err = ctx.Err()
				final.attempt = attempt
				out <- final
				return
			case <-t.C:
			}
		}
	}

	final.err = lastErr
	final.attempt = 1 + n.MaxRetry
	out <- final
}

// safeRun wraps RunFn so a panic in user code becomes an error instead of
// killing the scheduler goroutine, and stamps a per-node OTel span so the
// trace timeline shows one bar per scatter-gather worker.
func safeRun(ctx context.Context, n *Node, deps map[string]any) (result any, err error) {
	ctx, span := tracing.Tracer(tracing.SubsystemDAG).Start(ctx, "dag.node "+n.ID,
		trace.WithAttributes(
			attribute.String("node.id", n.ID),
			attribute.Int("node.deps", len(n.DependsOn)),
		),
	)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("node %q panicked: %v", n.ID, r)
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	return n.Run(ctx, deps)
}

func emit(ch chan<- Event, e Event) {
	if ch == nil {
		return
	}
	// Non-blocking emit: dropped events are preferable to a hung scheduler when
	// the consumer is slow. Callers wanting lossless delivery should buffer
	// generously or run a dedicated drain goroutine.
	select {
	case ch <- e:
	default:
	}
}
