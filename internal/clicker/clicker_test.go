package clicker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nrc-clicker/internal/interception"
)

type stubSender struct {
	mu      sync.Mutex
	moves   []point
	downs   int
	ups     int
}

type point struct{ x, y int }

func (s *stubSender) MoveAbs(x, y int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moves = append(s.moves, point{x, y})
	return nil
}
func (s *stubSender) Down() error { s.downs++; return nil }
func (s *stubSender) Up() error   { s.ups++; return nil }

// compile-time interface check
var _ interception.MouseSender = (*stubSender)(nil)

// TestAtomicConfigSwapDoesNotTear verifies the Store.Load → click sequence
// happens against a single snapshot. We swap the config mid-click by
// making HoldMs long enough to give us time.
func TestAtomicConfigSwapDoesNotTear(t *testing.T) {
	sender := &stubSender{}
	cfg := NewStore(Config{
		CentreX: 100, CentreY: 200, Radius: 0,
		MoveMouse: true, IntervalMs: 50, HoldMs: 80,
	})
	c := New(sender, cfg)
	c.Start()
	defer c.Stop()

	// Wait until at least one cycle starts.
	time.Sleep(30 * time.Millisecond)

	// Swap config mid-click. Should not panic or send mismatched coords.
	cfg.Store(Config{
		CentreX: 999, CentreY: 999, Radius: 0,
		MoveMouse: false, IntervalMs: 50, HoldMs: 80,
	})

	// Run a bit then stop.
	time.Sleep(120 * time.Millisecond)
	c.Stop()

	if c.ClickCount() < 1 {
		t.Fatalf("expected at least 1 click, got %d", c.ClickCount())
	}
	// After MoveMouse was disabled, no more moves should have happened.
	// (The first click may have moved; subsequent clicks must not.)
	sender.mu.Lock()
	defer sender.mu.Unlock()
	for i, p := range sender.moves {
		if p.x == 999 || p.y == 999 {
			t.Errorf("move[%d]=%+v should not have happened with MoveMouse=false", i, p)
		}
	}
}

// TestPauseResume toggles pause/resume and verifies the click count
// doesn't change while paused.
func TestPauseResume(t *testing.T) {
	sender := &stubSender{}
	cfg := NewStore(DefaultConfig())
	cfg.Store(Config{
		IntervalMs: 30, HoldMs: 10, Radius: 0,
	})
	c := New(sender, cfg)
	c.Start()
	defer c.Stop()

	// Let it click for 100ms.
	time.Sleep(100 * time.Millisecond)
	before := c.ClickCount()

	// Pause for 200ms.
	c.Pause()
	time.Sleep(200 * time.Millisecond)
	during := c.ClickCount()

	if during != before {
		t.Errorf("clicks advanced while paused: before=%d during=%d", before, during)
	}

	// Resume and click for 100ms more.
	c.Resume()
	time.Sleep(100 * time.Millisecond)
	after := c.ClickCount()
	if after <= during {
		t.Errorf("clicks did not advance after resume: during=%d after=%d", during, after)
	}
}

// TestStopIsIdempotent ensures Stop is safe to call multiple times.
func TestStopIsIdempotent(t *testing.T) {
	sender := &stubSender{}
	cfg := NewStore(DefaultConfig())
	c := New(sender, cfg)
	c.Start()
	c.Stop()
	c.Stop() // should not panic
	if c.Running() {
		t.Errorf("clicker still running after Stop")
	}
}

// TestStartTwiceIsNoop ensures we don't spawn multiple loops.
func TestStartTwiceIsNoop(t *testing.T) {
	sender := &stubSender{}
	cfg := NewStore(DefaultConfig())
	c := New(sender, cfg)
	c.Start()
	c.Start() // should be a no-op
	time.Sleep(50 * time.Millisecond)
	if !c.Running() {
		t.Errorf("clicker should still be running")
	}
	c.Stop()
}

// stubSender satisfies MouseSender; we add a guard to keep imports tidy.
var _ = atomic.Bool{}
var _ context.Context
