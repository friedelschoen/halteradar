package task

type Task[S any] interface {
	NeedsRun(state S) (bool, error)

	Execute(state S, progress func(float64)) error
	Cleanup(state S) error

	Dependencies() []string
	Group() string
}

type nodestate uint8

const (
	nodeunseen nodestate = iota

	/* graph */
	nodevisiting
	nodevisited

	/* runner */
	nodepending
	noderunning
	nodeskipped
	nodedone
	nodecleaning
	nodecleaned
	nodefailed
)

type node[S any] struct {
	name     string
	task     Task[S]
	deps     []*node[S]
	rdeps    []*node[S]
	neededby map[*node[S]]struct{}

	state    nodestate
	refcount int
}

func (n *node[S]) ready() bool {
	return n.state == nodedone ||
		n.state == nodecleaned ||
		n.state == nodeskipped
}
