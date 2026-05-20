package dag

import (
	"context"
	"strings"
	"testing"
)

func okRun(context.Context, map[string]any) (any, error) { return nil, nil }

func TestGraph_Add_DuplicateID(t *testing.T) {
	g := NewGraph()
	if err := g.Add(&Node{ID: "a", Run: okRun}); err != nil {
		t.Fatal(err)
	}
	if err := g.Add(&Node{ID: "a", Run: okRun}); err == nil {
		t.Fatal("expected duplicate ID error")
	}
}

func TestGraph_Add_NilRun(t *testing.T) {
	g := NewGraph()
	if err := g.Add(&Node{ID: "a"}); err == nil {
		t.Fatal("expected nil-Run error")
	}
}

func TestGraph_Add_EmptyID(t *testing.T) {
	g := NewGraph()
	if err := g.Add(&Node{ID: "", Run: okRun}); err == nil {
		t.Fatal("expected empty-ID error")
	}
}

func TestGraph_Validate_MissingDep(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: okRun, DependsOn: []string{"ghost"}})
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("expected missing-dep error, got %v", err)
	}
}

func TestGraph_Validate_SelfLoop(t *testing.T) {
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: okRun, DependsOn: []string{"a"}})
	if err := g.Validate(); err == nil {
		t.Fatal("expected self-loop error")
	}
}

func TestGraph_Validate_Cycle(t *testing.T) {
	// a -> b -> c -> a
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: okRun, DependsOn: []string{"c"}})
	_ = g.Add(&Node{ID: "b", Run: okRun, DependsOn: []string{"a"}})
	_ = g.Add(&Node{ID: "c", Run: okRun, DependsOn: []string{"b"}})
	err := g.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestGraph_Validate_OK_Diamond(t *testing.T) {
	// a -> b, a -> c, b -> d, c -> d
	g := NewGraph()
	_ = g.Add(&Node{ID: "a", Run: okRun})
	_ = g.Add(&Node{ID: "b", Run: okRun, DependsOn: []string{"a"}})
	_ = g.Add(&Node{ID: "c", Run: okRun, DependsOn: []string{"a"}})
	_ = g.Add(&Node{ID: "d", Run: okRun, DependsOn: []string{"b", "c"}})
	if err := g.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}
