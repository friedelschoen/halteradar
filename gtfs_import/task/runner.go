package task

import (
	"fmt"
	"sync"
)

type TaskRunner[S any] struct {
	G TaskGraph[S]

	State   S
	Workers int

	progress ProgressHandler
	mu       sync.Mutex
	cond     *sync.Cond
	running  int
	failed   error
	groups   map[string]*node[S]
}

func (r *TaskRunner[S]) setstate(n *node[S], state nodestate) {
	n.state = state

	switch state {
	case nodecleaning, nodefailed, noderunning:
		/* do nothing */
	default:
		return
	}

	var active []string
	for _, node := range r.G.order {
		switch node.state {
		case noderunning:
			active = append(active, node.name)

		case nodecleaning:
			active = append(active, node.name+"*")

		case nodefailed:
			active = append(active, node.name+"!")
		}
	}
}

func (r *TaskRunner[S]) canrun(n *node[S]) bool {
	for _, dep := range n.deps {
		if !dep.ready() {
			return false
		}
	}

	return true
}

func (r *TaskRunner[S]) cleanup(n *node[S]) error {
	if n.state != nodedone {
		return nil
	}
	if n.refcount != 0 {
		return nil
	}

	r.setstate(n, nodecleaning)

	if err := n.task.Cleanup(r.State); err != nil {
		r.setstate(n, nodefailed)
		return fmt.Errorf("%s: Cleanup: %w", n.task, err)
	}

	r.setstate(n, nodecleaned)
	return nil
}

func (r *TaskRunner[S]) finishdep(dep *node[S], seen map[*node[S]]struct{}) error {
	if _, ok := seen[dep]; ok {
		return nil
	}
	seen[dep] = struct{}{}

	dep.refcount--

	if err := r.cleanup(dep); err != nil {
		return err
	}

	return r.finishdeps(dep, seen)
}

func (r *TaskRunner[S]) finishdeps(n *node[S], seen map[*node[S]]struct{}) error {
	for _, dep := range n.deps {
		if err := r.finishdep(dep, seen); err != nil {
			return err
		}
	}

	return nil
}

func (r *TaskRunner[S]) finishtask(n *node[S]) error {
	if err := r.finishdeps(n, make(map[*node[S]]struct{})); err != nil {
		return err
	}

	return r.cleanup(n)
}
func (r *TaskRunner[S]) groupavailable(n *node[S]) bool {
	group := n.task.Group()
	if group == "" {
		return true
	}

	owner := r.groups[group]
	return owner == nil || owner == n
}
func (r *TaskRunner[S]) init() {
	if r.Workers <= 0 {
		r.Workers = 1
	}

	r.cond = sync.NewCond(&r.mu)
	r.groups = make(map[string]*node[S])

	for _, n := range r.G.order {
		r.setstate(n, nodepending)
		n.refcount = len(n.neededby)
	}
}

func (r *TaskRunner[S]) run(n *node[S]) {
	r.progress.SetProgress(n.name, -1)
	defer r.progress.Done(n.name)

	err := n.task.Execute(r.State, func(f float64) {
		r.progress.SetProgress(n.name, f)
	})

	r.mu.Lock()
	defer r.mu.Unlock()

	r.running--

	if group := n.task.Group(); group != "" && r.groups[group] == n {
		delete(r.groups, group)
	}

	if err != nil {
		r.setstate(n, nodefailed)
		if r.failed == nil {
			r.failed = fmt.Errorf("%s: Execute: %w", n.task, err)
		}
	} else {
		r.setstate(n, nodedone)

		if err := r.finishtask(n); err != nil && r.failed == nil {
			r.failed = err
		}
	}

	r.cond.Broadcast()
}

func (r *TaskRunner[S]) Execute() error {
	r.init()
	go r.progress.Run()

	for {
		r.mu.Lock()

		if r.failed != nil {
			r.mu.Unlock()
			return r.failed
		}

		allDone := true
		progress := false

		for _, n := range r.G.order {
			switch n.state {
			case nodeskipped, nodedone, nodecleaning, nodecleaned:
				continue
			case noderunning:
				allDone = false
				continue
			case nodepending:
				/* handled below */
			default:
				allDone = false
				continue
			}

			allDone = false

			if !r.canrun(n) || !r.groupavailable(n) {
				continue
			}

			needs, err := n.task.NeedsRun(r.State)
			if err != nil {
				r.setstate(n, nodefailed)
				r.mu.Unlock()
				return fmt.Errorf("%s: NeedsRun: %w", n.task, err)
			}

			if !needs {
				r.setstate(n, nodeskipped)

				if err := r.finishtask(n); err != nil {
					r.mu.Unlock()
					return err
				}

				progress = true
				r.cond.Broadcast()
				continue
			}

			for r.running >= r.Workers && r.failed == nil {
				r.cond.Wait()
			}
			if r.failed != nil {
				r.mu.Unlock()
				return r.failed
			}

			if group := n.task.Group(); group != "" {
				r.groups[group] = n
			}

			r.setstate(n, noderunning)
			r.running++
			progress = true

			go r.run(n)
		}

		if allDone {
			r.mu.Unlock()
			return nil
		}

		if !progress {
			r.cond.Wait()
		}

		r.mu.Unlock()
	}
}
