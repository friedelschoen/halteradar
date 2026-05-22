package task

import "fmt"

type TaskGraph[S any] struct {
	Targets []*node[S]

	nodes map[string]*node[S]
	order []*node[S]
}

func (g *TaskGraph[S]) Add(tasks map[string]Task[S], targets ...string) error {
	if g.nodes == nil {
		g.nodes = make(map[string]*node[S])
	}

	for _, target := range targets {
		n, err := g.visit(tasks, target)
		if err != nil {
			return err
		}

		g.Targets = append(g.Targets, n)
	}

	for _, n := range g.order {
		g.collect(n, n)
	}

	return nil
}

func (g *TaskGraph[S]) visit(tasks map[string]Task[S], name string) (*node[S], error) {
	n, ok := g.nodes[name]
	if ok {
		switch n.state {
		case nodevisiting:
			return nil, fmt.Errorf("dependency cycle involving %q", name)
		case nodevisited:
			return n, nil
		}
	} else {
		t, ok := tasks[name]
		if !ok {
			return nil, fmt.Errorf("unknown task %q", name)
		}

		n = &node[S]{
			name: name,
			task: t,
		}

		g.nodes[name] = n
	}

	n.state = nodevisiting

	for _, depname := range n.task.Dependencies() {
		dep, err := g.visit(tasks, depname)
		if err != nil {
			return nil, err
		}

		n.deps = append(n.deps, dep)
		dep.rdeps = append(dep.rdeps, n)
	}

	n.state = nodevisited
	g.order = append(g.order, n)

	return n, nil
}

func (g *TaskGraph[S]) collect(root, n *node[S]) {
	if root.neededby == nil {
		root.neededby = make(map[*node[S]]struct{})
	}

	for _, dependent := range n.rdeps {
		if _, ok := root.neededby[dependent]; ok {
			continue
		}

		root.neededby[dependent] = struct{}{}
		g.collect(root, dependent)
	}
}
