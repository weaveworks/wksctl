package plan

// This topological sort implementation from:
//   https://github.com/philopon/go-toposort
//   Copyright 2017 Hirotomo Moriwaki
//   MIT licensed.

type graph struct {
	nodes   map[string]bool
	edges   map[string]map[string]bool
	outputs map[string]map[string]int
	inputs  map[string]int
}

func newGraph(cap int) *graph {
	return &graph{
		nodes:   make(map[string]bool),
		edges:   make(map[string]map[string]bool),
		inputs:  make(map[string]int),
		outputs: make(map[string]map[string]int),
	}
}

func (g *graph) AddNode(name string) bool {
	if g.nodes[name] {
		return false
	}
	g.nodes[name] = true
	g.outputs[name] = make(map[string]int)
	g.inputs[name] = 0
	return true
}

func (g *graph) AddNodes(names ...string) bool {
	updated := false
	for _, name := range names {
		if ok := g.AddNode(name); ok {
			updated = true
		}
	}
	return updated
}

func (g *graph) AddEdge(from, to string) bool {
	edgesFrom := g.edges[from]
	if edgesFrom == nil {
		edgesFrom = make(map[string]bool)
		g.edges[from] = edgesFrom
	} else if edgesFrom[to] {
		return false
	}
	edgesFrom[to] = true
	g.AddNodes(from, to)
	m := g.outputs[from]
	m[to] = len(m) + 1
	g.inputs[to]++
	return true
}

func (g *graph) nodeCopy() *graph {
	newg := newGraph(len(g.nodes))
	for n := range g.nodes {
		newg.AddNode(n)
	}
	return newg
}

func (g *graph) Copy() *graph {
	newg := g.nodeCopy()
	for from, tos := range g.edges {
		for to := range tos {
			newg.AddEdge(from, to)
		}
	}
	return newg
}

func (g *graph) Invert() *graph {
	newg := g.nodeCopy()
	for from, tos := range g.edges {
		for to := range tos {
			newg.AddEdge(to, from)
		}
	}
	return newg
}

func (g *graph) Targets(source string) []string {
	m := g.outputs[source]
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}

func (g *graph) unsafeRemoveEdge(from, to string) {
	edgesFrom := g.edges[from]
	if edgesFrom == nil {
		return
	}
	delete(edgesFrom, to)
	delete(g.outputs[from], to)
	g.inputs[to]--
}

func (g *graph) RemoveEdge(from, to string) bool {
	if _, ok := g.outputs[from]; !ok {
		return false
	}
	g.unsafeRemoveEdge(from, to)
	return true
}

func (g *graph) Toposort() ([]string, bool) {
	newg := g.Copy()
	L := make([]string, 0, len(newg.nodes))
	S := make([]string, 0, len(newg.nodes))

	for n := range newg.nodes {
		if newg.inputs[n] == 0 {
			S = append(S, n)
		}
	}

	for len(S) > 0 {
		var n string
		n, S = S[0], S[1:]
		L = append(L, n)

		ms := make([]string, len(newg.outputs[n]))
		for m, i := range newg.outputs[n] {
			ms[i-1] = m
		}

		for _, m := range ms {
			newg.unsafeRemoveEdge(n, m)

			if newg.inputs[m] == 0 {
				S = append(S, m)
			}
		}
	}

	N := 0
	for _, v := range newg.inputs {
		N += v
	}

	if N > 0 {
		return L, false
	}

	return L, true
}
