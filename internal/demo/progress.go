package demo

// routeDirection indicates forward or reverse traversal.
type routeDirection int

const (
	directionForward routeDirection = iota
	directionReverse
)

// String returns a human-readable direction label.
func (d routeDirection) String() string {
	if d == directionForward {
		return "forward"
	}
	return "reverse"
}

// routeProgress tracks the current position within a route's forward-reverse loop.
//
// When a connection drops, the progress is preserved so the device resumes
// from where it left off instead of teleporting back to the start.
type routeProgress struct {
	direction  routeDirection
	pointIndex int
	loopCount  int // how many complete forward+reverse cycles have been completed
}

// newRouteProgress creates a new progress tracker starting at the beginning.
func newRouteProgress() *routeProgress {
	return &routeProgress{
		direction:  directionForward,
		pointIndex: 0,
		loopCount:  0,
	}
}

// Advance moves the point index forward by one.
func (p *routeProgress) Advance() {
	p.pointIndex++
}

// FinishDirection marks the current direction as complete and switches to the next.
// If the reverse direction just completed, the loop count increments and we
// go back to forward.
func (p *routeProgress) FinishDirection() {
	if p.direction == directionForward {
		p.direction = directionReverse
		p.pointIndex = 0
	} else {
		p.direction = directionForward
		p.pointIndex = 0
		p.loopCount++
	}
}

// Reset restarts the progress from the beginning of a new loop (forward, index 0).
func (p *routeProgress) Reset() {
	p.direction = directionForward
	p.pointIndex = 0
}
