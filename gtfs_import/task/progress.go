package task

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type ProgressStatus struct {
	progress   float64
	unsurePos  float64
	unsureLast time.Time
}

type ProgressHandler struct {
	prevLen int
	bars    []string
	status  map[string]*ProgressStatus
	signal  chan struct{}
	done    []string
	mu      sync.Mutex
}

func (h *ProgressHandler) Done(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.status, name)
	newBars := slices.DeleteFunc(h.bars, func(n string) bool {
		return n == name
	})
	if len(newBars) != len(h.bars) {
		h.done = append(h.done, name)
		h.update()
	}
	h.bars = newBars
}

func (h *ProgressHandler) update() {
	select {
	case h.signal <- struct{}{}:
	default:
	}
}

func (h *ProgressHandler) SetProgress(name string, progress float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.status == nil {
		h.status = make(map[string]*ProgressStatus)
	}
	if h.signal == nil {
		h.signal = make(chan struct{}, 1)
	}

	status, ok := h.status[name]
	if !ok {
		h.bars = append(h.bars, name)
		status = &ProgressStatus{
			progress:   progress,
			unsureLast: time.Now(),
		}
		h.status[name] = status
	}

	if progress < 0 && status.progress >= 0 {
		status.unsurePos = 0
		status.unsureLast = time.Now()
	}

	status.progress = progress

	h.update()
}

func winWidth() int {
	ws, err := unix.IoctlGetWinsize(
		int(os.Stdout.Fd()),
		unix.TIOCGWINSZ,
	)
	if err != nil {
		return 80
	}
	return int(ws.Col)
}

const UnsureSpeed = 20   // chars per millisecond
const UnsureWidth = 0.10 // percent of total width

func (h *ProgressHandler) cleanRows() {
	if h.prevLen > 0 {
		fmt.Printf("\033[%dA", h.prevLen)
	}

	for i := 0; i < h.prevLen; i++ {
		fmt.Print("\r\033[0K")
		if i+1 < h.prevLen {
			fmt.Print("\n")
		}
	}

	if h.prevLen > 1 {
		fmt.Printf("\033[%dA", h.prevLen-1)
	}

	h.prevLen = len(h.bars)
}

func (h *ProgressHandler) print() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cleanRows()

	for i := len(h.done) - 1; i >= 0; i-- {
		fmt.Printf("\r\033[0K%s\n", h.done[i])
	}
	h.done = nil

	cols := winWidth() - 20 - 5 - 5 - 1
	for _, name := range h.bars {
		var barContent string
		status := h.status[name]
		if status.progress >= 0 {
			numCols := int(float64(cols) * status.progress)

			fillDone := strings.Repeat("#", numCols)
			fillTodo := strings.Repeat(" ", cols-numCols)
			barContent = fillDone + fillTodo
		} else {
			now := time.Now()
			delta := now.Sub(status.unsureLast).Seconds()
			status.unsureLast = now

			barCols := min(max(int(UnsureWidth*float64(cols)), 1), cols)

			span := max(cols-barCols, 1)

			status.unsurePos += delta * UnsureSpeed
			numCols := int(status.unsurePos) % (span + 1)

			barContent =
				strings.Repeat(" ", numCols) +
					strings.Repeat("#", barCols) +
					strings.Repeat(" ", cols-numCols-barCols)
		}

		if status.progress >= 0 {
			fmt.Printf("\r\033[0K%-20s [%s] %5.1f%%\n", name, barContent, status.progress*100)
		} else {
			fmt.Printf("\r\033[0K%-20s [%s]  ???\n", name, barContent)
		}
	}
}

func (h *ProgressHandler) Run() {
	h.mu.Lock()
	if h.signal == nil {
		h.signal = make(chan struct{}, 1)
	}
	signal := h.signal
	h.mu.Unlock()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	lastVisit := time.Now()

	for {
		select {
		case <-signal:
		case <-ticker.C:
		}

		if time.Since(lastVisit) < 30*time.Millisecond {
			continue
		}

		lastVisit = time.Now()
		h.print()
	}
}
