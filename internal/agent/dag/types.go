// Package dag is a tiny, in-process DAG scheduler tuned for LLM agent
// workflows.
//
// Design notes:
//   - Nodes communicate only via the values returned from Run; the scheduler
//     copies those values into the next node's `deps` map so workers never
//     share mutable state.
//   - The coordinator goroutine owns all bookkeeping (states / outputs /
//     unresolved-deps). Workers only execute. This is the classic Go
//     "share by communicating" pattern and avoids the mutex zoo.
//   - First failure is fatal by default (fail-fast); the scheduler cancels
//     the shared context so all in-flight nodes wind down promptly.
//   - Retries are per-node with a configurable backoff and respect ctx.
package dag

import (
	"context"
	"time"
)

// State is the lifecycle state of a node within one scheduler run.
type State string

const (
	StatePending   State = "pending"   // not yet eligible to run
	StateReady     State = "ready"     // all deps resolved, queued for execution
	StateRunning   State = "running"   // a worker has picked it up
	StateSucceeded State = "succeeded" // Run returned nil
	StateFailed    State = "failed"    // Run exhausted retries
	StateCancelled State = "cancelled" // never started; another node failed first
)

// RunFn is the user-supplied work for a node. `deps` maps dependency node IDs
// to whatever value those nodes' RunFn returned, so a Researcher can read its
// upstream Planner's plan without any global state.
type RunFn func(ctx context.Context, deps map[string]any) (any, error)

// Node is the unit of work scheduled by the DAG.
type Node struct {
	ID        string        // unique within a Graph
	Name      string        // human label, surfaced in events
	DependsOn []string      // IDs that must succeed before this node may run
	Run       RunFn         // required
	MaxRetry  int           // additional attempts after the first failure (0 = no retry)
	Timeout   time.Duration // per-attempt timeout; 0 means inherit from ctx
}

// EventType is a coarse-grained signal about node lifecycle, intended for
// surfacing progress in a UI or trace.
type EventType string

const (
	EventNodeStarted   EventType = "node_started"
	EventNodeSucceeded EventType = "node_succeeded"
	EventNodeFailed    EventType = "node_failed"
	EventNodeRetrying  EventType = "node_retrying"
	EventNodeCancelled EventType = "node_cancelled"
)

// Event is what the scheduler emits to subscribers.
type Event struct {
	Type    EventType
	NodeID  string
	Name    string
	Attempt int           // 1-based attempt counter
	Result  any           // populated only for EventNodeSucceeded
	Error   error         // populated for failure/retry events
	Elapsed time.Duration // wall clock for this attempt
}

// Result is the terminal outcome of a Run().
type Result struct {
	Outputs map[string]any   // nodeID -> RunFn return value (succeeded nodes only)
	States  map[string]State // final state per node
}
