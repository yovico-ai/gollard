package gollard

import (
	"fmt"
	"slices"
	"sort"
)

type MigrationGraph struct {
	nodes []MigrationID
	// adj[i] holds indexes of migrations that depend on node i.
	// (Edge direction: required → dependent.)
	adj   [][]int
	index map[MigrationID]int
}

type CircularDependencyError struct {
	Cycle []MigrationID
}

func (e *CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependency among migrations: %v", e.Cycle)
}

type MissingDependencyError struct {
	Migration MigrationID
	Requires  MigrationID
}

func (e *MissingDependencyError) Error() string {
	return fmt.Sprintf("migration %q requires %q which does not exist", e.Migration, e.Requires)
}

func MakeMigrationGraph(table MigrationTable) (*MigrationGraph, error) {
	g := &MigrationGraph{index: make(map[MigrationID]int, len(table))}
	ids := make([]MigrationID, 0, len(table))
	for id := range table {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		g.index[id] = len(g.nodes)
		g.nodes = append(g.nodes, id)
	}
	g.adj = make([][]int, len(g.nodes))
	for _, id := range ids {
		m := table[id]
		for _, req := range m.Requires {
			ri, ok := g.index[req]
			if !ok {
				return nil, &MissingDependencyError{Migration: id, Requires: req}
			}
			g.adj[ri] = append(g.adj[ri], g.index[id])
		}
	}
	if cycle := g.findCycle(); cycle != nil {
		return nil, &CircularDependencyError{Cycle: cycle}
	}
	return g, nil
}

// findCycle does a DFS looking for a back-edge; returns the cycle's IDs or nil.
func (g *MigrationGraph) findCycle() []MigrationID {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make([]int, len(g.nodes))
	parent := make([]int, len(g.nodes))
	for i := range parent {
		parent[i] = -1
	}
	var cycle []MigrationID
	var visit func(u int) bool
	visit = func(u int) bool {
		color[u] = gray
		for _, v := range g.adj[u] {
			if color[v] == gray {
				cycle = append(cycle, g.nodes[v])
				for x := u; x != -1 && x != v; x = parent[x] {
					cycle = append(cycle, g.nodes[x])
				}
				return true
			}
			if color[v] == white {
				parent[v] = u
				if visit(v) {
					return true
				}
			}
		}
		color[u] = black
		return false
	}
	for u := range g.nodes {
		if color[u] == white {
			if visit(u) {
				return cycle
			}
		}
	}
	return nil
}

// GetUnappliedMigrations returns ids from this graph that are absent from
// `applied`, in topological (dependency-first) order. Ties broken alphabetically.
func (g *MigrationGraph) GetUnappliedMigrations(applied []MigrationID) []MigrationID {
	appliedSet := make(map[MigrationID]bool, len(applied))
	for _, id := range applied {
		appliedSet[id] = true
	}

	n := len(g.nodes)
	excluded := make([]bool, n)
	for i, id := range g.nodes {
		if appliedSet[id] {
			excluded[i] = true
		}
	}

	indeg := make([]int, n)
	for u := range n {
		if excluded[u] {
			continue
		}
		for _, v := range g.adj[u] {
			if excluded[v] {
				continue
			}
			indeg[v]++
		}
	}

	var ready []int
	for u := range n {
		if !excluded[u] && indeg[u] == 0 {
			ready = append(ready, u)
		}
	}

	result := make([]MigrationID, 0, n)
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			return g.nodes[ready[i]] < g.nodes[ready[j]]
		})
		u := ready[0]
		ready = ready[1:]
		result = append(result, g.nodes[u])
		for _, v := range g.adj[u] {
			if excluded[v] {
				continue
			}
			indeg[v]--
			if indeg[v] == 0 {
				ready = append(ready, v)
			}
		}
	}

	return result
}
