package clicker

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"nrc-clicker/internal/interception"
)

// Clicker is the simple auto-click loop. It is independent of the script
// executor: either one may run at a time, but not both (manager enforces).
type Clicker struct {
	sender interception.MouseSender
	config *Store
	logger *logrus.Entry

	mu      sync.Mutex
	loop    *time.Timer
	cancel  context.CancelFunc
	running atomic.Bool
	paused  atomic.Bool

	// Stats
	clickCount atomic.Int64
	startTime  atomic.Int64 // unix nano
}

// New wires the clicker to a MouseSender (normally interception.Core).
// The supplied Store may be shared with the UI.
func New(sender interception.MouseSender, config *Store) *Clicker {
	return &Clicker{
		sender: sender,
		config: config,
		logger: logrus.WithField("subsystem", "clicker"),
	}
}

// Running reports whether the loop is currently active.
func (c *Clicker) Running() bool { return c.running.Load() }

// Paused reports whether the loop is paused (only meaningful when Running).
func (c *Clicker) Paused() bool { return c.paused.Load() }

// ClickCount returns the total clicks issued in the current session.
func (c *Clicker) ClickCount() int64 { return c.clickCount.Load() }

// Start kicks off the loop in a goroutine. Idempotent: calling twice is a
// no-op while the loop is already running.
func (c *Clicker) Start() {
	if !c.running.CompareAndSwap(false, true) {
		return
	}
	c.clickCount.Store(0)
	c.startTime.Store(time.Now().UnixNano())
	c.paused.Store(false)

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	go c.run(ctx)
	c.logger.Info("clicker started")
}

// Stop signals the loop to exit and waits up to 1s for the goroutine.
// Idempotent.
func (c *Clicker) Stop() {
	if !c.running.CompareAndSwap(true, false) {
		return
	}
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.mu.Unlock()
	c.logger.Infof("clicker stopped after %d clicks", c.clickCount.Load())
}

// Pause halts the loop until Resume is called.
func (c *Clicker) Pause() bool {
	if !c.running.Load() {
		return false
	}
	c.paused.Store(true)
	return true
}

// Resume un-pauses a paused loop.
func (c *Clicker) Resume() bool {
	if !c.running.Load() {
		return false
	}
	c.paused.Store(false)
	return true
}

// TogglePause flips the pause state and returns the new value.
// Returns false if the clicker isn't running.
func (c *Clicker) TogglePause() bool {
	if !c.running.Load() {
		return false
	}
	if c.paused.Load() {
		c.paused.Store(false)
		return false
	}
	c.paused.Store(true)
	return true
}

// run is the inner loop. It uses time.Timer + select (rather than Sleep)
// so pause/stop signals are responsive within a few milliseconds.
func (c *Clicker) run(ctx context.Context) {
	defer c.running.Store(false)

	for {
		if ctx.Err() != nil {
			return
		}
		// Honour pause: spin on a 50ms ticker until unpaused or stopped.
		for c.paused.Load() {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}

		cfg := c.config.Load()
		c.doClick(cfg)

		interval := cfg.IntervalMs + c.jitterMs(cfg.JitterRangeMs)
		if interval < 1 {
			interval = 1
		}
		t := time.NewTimer(time.Duration(interval) * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// doClick performs one full click cycle: optional move, down, hold, up.
func (c *Clicker) doClick(cfg Config) {
	x, y := c.randomTarget(cfg.CentreX, cfg.CentreY, cfg.Radius)
	if cfg.MoveMouse {
		if err := c.sender.MoveAbs(x, y); err != nil {
			c.logger.Warnf("move failed: %v", err)
			return
		}
		// Brief settle delay (matches Python's 20ms).
		t := time.NewTimer(20 * time.Millisecond)
		<-t.C
	}
	if err := c.sender.Down(); err != nil {
		c.logger.Warnf("down failed: %v", err)
		return
	}
	hold := cfg.HoldMs
	if hold < 1 {
		hold = 1
	}
	t := time.NewTimer(time.Duration(hold) * time.Millisecond)
	<-t.C
	if err := c.sender.Up(); err != nil {
		c.logger.Warnf("up failed: %v", err)
	}
	c.clickCount.Add(1)
}

// randomTarget picks a random point in the disc of given radius around
// (cx, cy). We use uniform angle + sqrt(uniform(0,1)) * radius for uniform
// area distribution.
func (c *Clicker) randomTarget(cx, cy, radius int) (int, int) {
	if radius <= 0 {
		return cx, cy
	}
	angle := rand.Float64() * 2 * math.Pi
	dist := math.Sqrt(rand.Float64()) * float64(radius)
	return cx + int(dist*math.Cos(angle)), cy + int(dist*math.Sin(angle))
}

// jitterMs returns a value uniformly distributed in [base - jitter, base + jitter].
// Reuses the math/rand default Source; the clicker is non-deterministic
// by design (humanisation).
func (c *Clicker) jitterMs(jitter int) int {
	if jitter <= 0 {
		return 0
	}
	return rand.Intn(2*jitter+1) - jitter
}
