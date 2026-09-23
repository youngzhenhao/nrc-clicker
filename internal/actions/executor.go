package actions

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"nrc-clicker/internal/interception"
)

// ErrIgnoredMove is returned by internal accounting when the global
// move_mouse flag suppresses an action that would otherwise move the
// cursor. Callers should increment the ignored-moves counter and continue.
var ErrIgnoredMove = errors.New("move ignored: global move_mouse=false")

// MouseMoveGate abstracts the global "should we move the cursor" config.
// The manager supplies an implementation backed by an atomic.Pointer so the
// executor can read the latest value at every click without taking a lock.
type MouseMoveGate interface {
	MoveMouse() bool
}

// Sender bundles the two interfaces the executor needs. Both halves are
// satisfied by interception.Core; tests pass a stub that records calls.
type Sender interface {
	interception.MouseSender
	interception.KeyboardSender
}

// Executor walks a slice of actions and drives the input device. It is
// stateless apart from the ignored-moves counter and the gate / sender it
// was constructed with.
//
// All public methods honour the provided context: cancellation triggers an
// immediate stop (no cleanup beyond returning). Pause is signalled via the
// Resume channel — closed means "running", opened means "paused".
type Executor struct {
	sender Sender
	gate   MouseMoveGate
	rng    *rand.Rand
	logger *logrus.Entry

	ignoredMoves atomic.Int64
}

// NewExecutor wires the executor to a real sender and a mouse-move gate.
// Pass rand.New(rand.NewSource(time.Now().UnixNano())) for non-deterministic
// jitter (production); tests use a fixed seed.
func NewExecutor(sender Sender, gate MouseMoveGate, rng *rand.Rand) *Executor {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &Executor{
		sender: sender,
		gate:   gate,
		rng:    rng,
		logger: logrus.WithField("subsystem", "executor"),
	}
}

// IgnoredMoves returns the running count of move actions that were
// suppressed because the global move_mouse flag was off.
func (e *Executor) IgnoredMoves() int64 { return e.ignoredMoves.Load() }

// ResetIgnoredMoves clears the counter (called when starting a new session).
func (e *Executor) ResetIgnoredMoves() { e.ignoredMoves.Store(0) }

// Run executes the action list under the provided lifecycle channel pair.
// The semantics mirror ActionScript.py:_execute_actions / _execute_timed.
//
//   - ctx.Done()    — stop immediately and return ctx.Err().
//   - pause         — open channel blocks the executor until it is closed again.
//   - On completion, returns nil unless ctx was cancelled.
//
// Note: the channel-based pause pattern matches Python's
// threading.Event; we use a simple `done chan struct{}` instead because Go
// has no built-in Event type.
func (e *Executor) Run(ctx context.Context, actions []Action, pause <-chan struct{}) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !e.waitResumed(ctx, pause) {
			return ctx.Err()
		}
		more, err := e.runOnce(ctx, actions, pause)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// runOnce executes the actions exactly once. Container actions
// (loop/timed) may loop internally; this returns false when there is no
// more work scheduled (timed with repeat=0 / forever=false after last iter).
func (e *Executor) runOnce(ctx context.Context, actions []Action, pause <-chan struct{}) (bool, error) {
	for _, act := range actions {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !e.waitResumed(ctx, pause) {
			return false, ctx.Err()
		}
		switch a := act.(type) {
		case LoopAction:
			more, err := e.runLoop(ctx, a, pause)
			if err != nil || !more {
				return more, err
			}
		case TimedAction:
			more, err := e.runTimed(ctx, a, pause)
			if err != nil || !more {
				return more, err
			}
		case ClickAction:
			e.runClick(a)
		case MoveAction:
			e.runMove(a)
		case KeyAction:
			e.runKey(ctx, a)
		case ComboAction:
			e.runCombo(ctx, a)
		case WaitAction:
			if !e.sleepInterruptible(ctx, time.Duration(a.DurationMs)*time.Millisecond, pause) {
				return false, ctx.Err()
			}
		default:
			return false, fmt.Errorf("unknown action: %T", act)
		}
	}
	return false, nil
}

// runLoop mirrors ActionScript.py's LoopAction handling: count iterations
// (or forever when Forever/UntilExit/count<=0), with PauseMs between them.
func (e *Executor) runLoop(ctx context.Context, l LoopAction, pause <-chan struct{}) (bool, error) {
	count := l.Count
	if count < 1 {
		count = 1
	}
	for i := 0; l.Forever || i < count; i++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !e.waitResumed(ctx, pause) {
			return false, ctx.Err()
		}
		if _, err := e.runOnce(ctx, l.Actions, pause); err != nil {
			return false, err
		}
		if l.PauseMs > 0 {
			d := time.Duration(e.jitterMs(l.PauseMs, l.PauseJitterMs, 0)) * time.Millisecond
			if !e.sleepInterruptible(ctx, d, pause) {
				return false, ctx.Err()
			}
		}
	}
	return l.Forever, nil
}

// runTimed mirrors ActionScript.py:_execute_timed: run the inner actions
// inside an "on" window, then sleep the "off" window, repeat up to Repeat
// times (or forever).
func (e *Executor) runTimed(ctx context.Context, t TimedAction, pause <-chan struct{}) (bool, error) {
	executeDur := time.Duration(t.ExecuteMs) * time.Millisecond
	sleepDur := time.Duration(t.SleepMs) * time.Millisecond
	count := t.Repeat
	if count < 1 {
		count = 1
	}

	for i := 0; t.Forever || i < count; i++ {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !e.waitResumed(ctx, pause) {
			return false, ctx.Err()
		}

		if executeDur > 0 {
			deadline := time.Now().Add(executeDur)
			for {
				if err := ctx.Err(); err != nil {
					return false, err
				}
				if time.Now().After(deadline) {
					break
				}
				// Pause-aware recheck: when the script is paused mid-window we
				// shift deadline forward by the paused duration so the on-time
				// budget is preserved (matches Python end_time += (resumed-paused)).
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				case <-pause:
					pausedAt := time.Now()
					if !e.waitResumed(ctx, pause) {
						return false, ctx.Err()
					}
					deadline = deadline.Add(time.Since(pausedAt))
					continue
				default:
				}
				// Run one full pass of the inner actions; if it returns
				// false we treat that as "stopped" and bail.
				more, err := e.runOnce(ctx, t.Actions, pause)
				if err != nil {
					return false, err
				}
				if !more {
					// runOnce returned because actions completed and weren't
					// containers — that's fine, just check the deadline again.
				}
			}
		}
		if sleepDur > 0 {
			if !e.sleepInterruptible(ctx, sleepDur, pause) {
				return false, ctx.Err()
			}
		}
	}
	return t.Forever, nil
}

// --- leaf runners ---------------------------------------------------------

// runClick honours the move_mouse gate. When the gate is off we still send
// the down/up events (so the click hits at the current cursor position)
// but increment ignoredMoves if the gate suppressed the move.
func (e *Executor) runClick(a ClickAction) {
	if e.gate != nil && !e.gate.MoveMouse() {
		e.ignoredMoves.Add(1)
		e.logger.Debug("click suppressed move (move_mouse=false)")
	} else {
		x := e.jitterInt(a.X, a.XJitterPx, 0)
		y := e.jitterInt(a.Y, a.YJitterPx, 0)
		if err := e.sender.MoveAbs(x, y); err != nil {
			e.logger.Warnf("moveAbs failed: %v", err)
			return
		}
		// Tiny settle delay (matches Python's 20ms).
		time.Sleep(20 * time.Millisecond)
	}

	hold := time.Duration(e.jitterMs(a.HoldMs, a.HoldJitterMs, 1)) * time.Millisecond
	if err := e.sender.Down(); err != nil {
		e.logger.Warnf("mouse down failed: %v", err)
		return
	}
	time.Sleep(hold)
	if err := e.sender.Up(); err != nil {
		e.logger.Warnf("mouse up failed: %v", err)
	}
}

// runMove also respects the gate; when suppressed we count it and return.
func (e *Executor) runMove(a MoveAction) {
	if e.gate != nil && !e.gate.MoveMouse() {
		e.ignoredMoves.Add(1)
		e.logger.Debug("move suppressed (move_mouse=false)")
		return
	}
	x := e.jitterInt(a.X, a.XJitterPx, 0)
	y := e.jitterInt(a.Y, a.YJitterPx, 0)
	if err := e.sender.MoveAbs(x, y); err != nil {
		e.logger.Warnf("moveAbs failed: %v", err)
		return
	}
	dur := time.Duration(e.jitterMs(a.DurationMs, a.DurationJitterMs, 1)) * time.Millisecond
	time.Sleep(dur)
}

// runKey tries the Interception path first; on any error it falls back to
// user32.keybd_event via the keybd_fallback stub (real implementation lives
// in the manager package — see executor_windows.go for the build-tag split).
func (e *Executor) runKey(ctx context.Context, a KeyAction) {
	sc, ext, ok := LookupScancode(uint16(a.VKCode))
	if !ok {
		e.logger.Warnf("unknown VK 0x%02X", a.VKCode)
		return
	}
	hold := time.Duration(e.jitterMs(a.HoldMs, a.HoldJitterMs, 1)) * time.Millisecond
	if err := e.sender.Send(sc, interception.KeyStateDown, ext); err == nil {
		time.Sleep(hold)
		if err := e.sender.Send(sc, interception.KeyStateUp, ext); err != nil {
			e.logger.Warnf("key up failed: %v", err)
		}
		return
	}
	keybdFallback(uint16(a.VKCode), hold)
}

// runCombo presses all keys, holds, then releases in reverse order.
func (e *Executor) runCombo(ctx context.Context, a ComboAction) {
	type spec struct {
		sc  uint16
		ext bool
		vk  uint16
	}
	specs := make([]spec, 0, len(a.VKCodes))
	for _, vk := range a.VKCodes {
		sc, ext, ok := LookupScancode(uint16(vk))
		if !ok {
			e.logger.Warnf("unknown combo VK 0x%02X", vk)
			continue
		}
		specs = append(specs, spec{sc: sc, ext: ext, vk: uint16(vk)})
	}
	if len(specs) == 0 {
		return
	}
	hold := time.Duration(e.jitterMs(a.HoldMs, a.HoldJitterMs, 1)) * time.Millisecond
	for _, s := range specs {
		if err := e.sender.Send(s.sc, interception.KeyStateDown, s.ext); err != nil {
			e.logger.Warnf("combo down failed vk=0x%X: %v", s.vk, err)
		}
	}
	time.Sleep(hold)
	for i := len(specs) - 1; i >= 0; i-- {
		s := specs[i]
		if err := e.sender.Send(s.sc, interception.KeyStateUp, s.ext); err != nil {
			e.logger.Warnf("combo up failed vk=0x%X: %v", s.vk, err)
		}
	}
}

// --- helpers --------------------------------------------------------------

// sleepInterruptible is the cancellable + pausable sleep used throughout.
// Returns true if the full duration elapsed; false if ctx was cancelled.
//
// We track an absolute deadline rather than resetting a Timer, so that
// pause/resume cycles extend the total elapsed time by exactly the paused
// duration — important for the timed-action window semantics.
func (e *Executor) sleepInterruptible(ctx context.Context, d time.Duration, pause <-chan struct{}) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	deadline := time.Now().Add(d)
	for {
		if ctx.Err() != nil {
			return false
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return true
		}
		// Cap each timer slice to 50ms so stop/pause signals are responsive.
		sleep := remaining
		if sleep > 50*time.Millisecond {
			sleep = 50 * time.Millisecond
		}
		t := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-pause:
			t.Stop()
			// Pause channel closed == "go again". Loop recomputes remaining
			// from the unchanged deadline, so we add the paused duration
			// implicitly.
			continue
		case <-t.C:
			// Continue the loop to recompute remaining.
		}
	}
}

// waitResumed blocks while pause is open (i.e. while the channel still
// has a value buffered in it). The Python reference treats "set" as "go";
// in Go terms that means the channel is NOT receiving — it has nothing
// buffered. We model "set" with a nil channel (immediately ready) and
// "cleared" with an open channel that holds the empty struct forever.
//
// pause == nil        — never pause
// pause is open chan  — paused; wait for it to be closed
func (e *Executor) waitResumed(ctx context.Context, pause <-chan struct{}) bool {
	if pause == nil {
		return ctx.Err() == nil
	}
	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		select {
		case <-ctx.Done():
			return false
		case <-pause:
			return true
		default:
			return true
		}
	}
}

// jitterInt applies ±jitter to value, clamped to at least min.
func (e *Executor) jitterInt(value, jitter, min int) int {
	if jitter <= 0 {
		if value < min {
			return min
		}
		return value
	}
	v := value + e.rng.Intn(2*jitter+1) - jitter
	if v < min {
		return min
	}
	return v
}

// jitterMs is a convenience around jitterInt for millisecond values.
func (e *Executor) jitterMs(value, jitter, min int) int {
	return e.jitterInt(value, jitter, min)
}
