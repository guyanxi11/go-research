package dag

import (
	"errors"
	"fmt"
)

// Graph is an append-only collection of Nodes. It is not safe for concurrent
// modification, but reads are safe after Validate succeeds.
type Graph struct {
	nodes []*Node
	byID  map[string]*Node
}

func NewGraph() *Graph {
	return &Graph{byID: make(map[string]*Node)}
}

// Add registers a node. Returns an error on duplicate ID or missing Run.
func (g *Graph) Add(n *Node) error {
	if n == nil {
		return errors.New("dag: nil node")
	}
	if n.ID == "" {
		return errors.New("dag: node ID is empty")
	}
	if n.Run == nil {
		return fmt.Errorf("dag: node %q has nil Run", n.ID)
	}
	if _, dup := g.byID[n.ID]; dup {
		return fmt.Errorf("dag: duplicate node ID %q", n.ID)
	}
	g.nodes = append(g.nodes, n)
	g.byID[n.ID] = n
	return nil
}

// Nodes returns a defensive copy of the registered nodes preserving insertion
// order. The slice may be iterated freely without locking.
func (g *Graph) Nodes() []*Node {
	out := make([]*Node, len(g.nodes))
	copy(out, g.nodes)
	return out
}

// Validate checks for missing deps and cycles. It should be called once before
// scheduling; the scheduler also calls it as defence in depth.
func (g *Graph) Validate() error {
	for _, n := range g.nodes {
		for _, dep := range n.DependsOn {
			if dep == n.ID {
				return fmt.Errorf("dag: node %q depends on itself", n.ID)
			}
			if _, ok := g.byID[dep]; !ok {
				return fmt.Errorf("dag: node %q has unknown dep %q", n.ID, dep)
			}
		}
	}
	return g.detectCycle()
}

// detectCycle is iterative DFS with three colours (white/grey/black). Choice
// of iteration over recursion keeps very tall graphs from blowing the stack.
func (g *Graph) detectCycle() error {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := make(map[string]int, len(g.nodes))
	for _, n := range g.nodes {
		if colour[n.ID] != white {
			continue
		}
		stack := []string{n.ID}
		// Track entry vs. finish on the same stack via a parallel marker slice.
		marker := []bool{false} // false = first visit, true = finishing
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			finishing := marker[len(marker)-1]
			if finishing {
				colour[id] = black
				stack = stack[:len(stack)-1]
				marker = marker[:len(marker)-1]
				continue
			}
			marker[len(marker)-1] = true
			colour[id] = grey
			for _, dep := range g.byID[id].DependsOn {
				switch colour[dep] {
				case grey:
					return fmt.Errorf("dag: cycle detected involving %q -> %q", id, dep)
				case white:
					stack = append(stack, dep)
					marker = append(marker, false)
				}
			}
		}
	}
	return nil
}
