package demo

import "testing"

func TestRouteProgress_InitialState(t *testing.T) {
	p := newRouteProgress()
	if p.direction != directionForward {
		t.Errorf("initial direction = %v, want forward", p.direction)
	}
	if p.pointIndex != 0 {
		t.Errorf("initial pointIndex = %d, want 0", p.pointIndex)
	}
	if p.loopCount != 0 {
		t.Errorf("initial loopCount = %d, want 0", p.loopCount)
	}
}

func TestRouteProgress_Advance(t *testing.T) {
	p := newRouteProgress()
	p.Advance()
	if p.pointIndex != 1 {
		t.Errorf("pointIndex after Advance = %d, want 1", p.pointIndex)
	}
	p.Advance()
	p.Advance()
	if p.pointIndex != 3 {
		t.Errorf("pointIndex after 3 advances = %d, want 3", p.pointIndex)
	}
}

func TestRouteProgress_FinishDirection_ForwardToReverse(t *testing.T) {
	p := newRouteProgress()
	p.pointIndex = 100 // Simulate being partway through.

	p.FinishDirection()
	if p.direction != directionReverse {
		t.Errorf("direction after finishing forward = %v, want reverse", p.direction)
	}
	if p.pointIndex != 0 {
		t.Errorf("pointIndex after finishing forward = %d, want 0", p.pointIndex)
	}
	if p.loopCount != 0 {
		t.Errorf("loopCount should not increment after forward, got %d", p.loopCount)
	}
}

func TestRouteProgress_FinishDirection_ReverseToForward(t *testing.T) {
	p := newRouteProgress()
	p.direction = directionReverse
	p.pointIndex = 50

	p.FinishDirection()
	if p.direction != directionForward {
		t.Errorf("direction after finishing reverse = %v, want forward", p.direction)
	}
	if p.pointIndex != 0 {
		t.Errorf("pointIndex after finishing reverse = %d, want 0", p.pointIndex)
	}
	if p.loopCount != 1 {
		t.Errorf("loopCount should be 1 after completing full cycle, got %d", p.loopCount)
	}
}

func TestRouteProgress_FullCycle(t *testing.T) {
	p := newRouteProgress()

	// Simulate a full forward + reverse cycle.
	for i := 0; i < 10; i++ {
		p.Advance()
	}
	p.FinishDirection() // Forward done.

	for i := 0; i < 10; i++ {
		p.Advance()
	}
	p.FinishDirection() // Reverse done.

	if p.loopCount != 1 {
		t.Errorf("loopCount = %d, want 1 after one full cycle", p.loopCount)
	}
	if p.direction != directionForward {
		t.Errorf("direction = %v, want forward at start of second cycle", p.direction)
	}
	if p.pointIndex != 0 {
		t.Errorf("pointIndex = %d, want 0 at start of second cycle", p.pointIndex)
	}
}

func TestRouteProgress_Reset(t *testing.T) {
	p := newRouteProgress()
	p.direction = directionReverse
	p.pointIndex = 42
	p.loopCount = 5

	p.Reset()
	if p.direction != directionForward {
		t.Errorf("direction after Reset = %v, want forward", p.direction)
	}
	if p.pointIndex != 0 {
		t.Errorf("pointIndex after Reset = %d, want 0", p.pointIndex)
	}
	// loopCount is preserved across Reset (tracks total completions).
	if p.loopCount != 5 {
		t.Errorf("loopCount after Reset = %d, want 5 (preserved)", p.loopCount)
	}
}

func TestRouteDirection_String(t *testing.T) {
	if directionForward.String() != "forward" {
		t.Errorf("forward.String() = %q", directionForward.String())
	}
	if directionReverse.String() != "reverse" {
		t.Errorf("reverse.String() = %q", directionReverse.String())
	}
}
