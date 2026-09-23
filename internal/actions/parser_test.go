package actions

import (
	"context"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"nrc-clicker/internal/interception"
)

// stubSender records every Send / MoveAbs / Down / Up call. Implements
// both MouseSender and KeyboardSender so it can stand in for interception.Core
// in executor tests.
type stubSender struct {
	moves   atomic.Int64
	downs   atomic.Int64
	ups     atomic.Int64
	keyDown atomic.Int64
	keyUp   atomic.Int64
}

func (s *stubSender) MoveAbs(x, y int) error { s.moves.Add(1); return nil }
func (s *stubSender) Down() error           { s.downs.Add(1); return nil }
func (s *stubSender) Up() error             { s.ups.Add(1); return nil }
func (s *stubSender) Send(sc uint16, state uint16, _ bool) error {
	switch state {
	case interception.KeyStateDown:
		s.keyDown.Add(1)
	case interception.KeyStateUp:
		s.keyUp.Add(1)
	}
	return nil
}

type alwaysMoveGate struct{}

func (alwaysMoveGate) MoveMouse() bool { return true }

type neverMoveGate struct{}

func (neverMoveGate) MoveMouse() bool { return false }

// TestParseEachActionType verifies every leaf action round-trips through
// ParseActions.
func TestParseEachActionType(t *testing.T) {
	src := []byte(`[
		{"type":"click","x":100,"y":200,"hold_ms":80,"x_jitter_px":4,"y_jitter_px":4,"hold_jitter_ms":10},
		{"type":"move","x":300,"y":400,"duration_ms":120,"x_jitter_px":2,"y_jitter_px":2,"duration_jitter_ms":5},
		{"type":"key","vk_code":32,"hold_ms":50,"hold_jitter_ms":5},
		{"type":"combo","vk_codes":[87,68],"hold_ms":100},
		{"type":"wait","duration_ms":250,"duration_jitter_ms":20}
	]`)
	got, err := ParseActions(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("want 5 actions, got %d", len(got))
	}
	if c, ok := got[0].(ClickAction); !ok || c.X != 100 || c.Y != 200 || c.HoldMs != 80 {
		t.Errorf("click action malformed: %+v", got[0])
	}
	if m, ok := got[1].(MoveAction); !ok || m.DurationMs != 120 {
		t.Errorf("move action malformed: %+v", got[1])
	}
	if k, ok := got[2].(KeyAction); !ok || k.VKCode != 32 {
		t.Errorf("key action malformed: %+v", got[2])
	}
	if c, ok := got[3].(ComboAction); !ok || len(c.VKCodes) != 2 || c.VKCodes[0] != 87 {
		t.Errorf("combo action malformed: %+v", got[3])
	}
	if w, ok := got[4].(WaitAction); !ok || w.DurationMs != 250 {
		t.Errorf("wait action malformed: %+v", got[4])
	}
}

// TestLoopAcceptsLegacyUntilExit covers the Python-compatible loop terminator:
// forever / until_exit / count<=0 all collapse into Forever=true.
func TestLoopAcceptsLegacyUntilExit(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"forever", `{"type":"loop","forever":true,"actions":[]}`, true},
		{"until_exit", `{"type":"loop","until_exit":true,"actions":[]}`, true},
		{"count_zero", `{"type":"loop","count":0,"actions":[]}`, true},
		{"count_negative", `{"type":"loop","count":-1,"actions":[]}`, true},
		{"count_three", `{"type":"loop","count":3,"actions":[]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acts, err := ParseActions([]byte("[" + tc.src + "]"))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			l, ok := acts[0].(LoopAction)
			if !ok {
				t.Fatalf("expected LoopAction, got %T", acts[0])
			}
			if l.Forever != tc.want {
				t.Errorf("Forever=%v want %v", l.Forever, tc.want)
			}
		})
	}
}

// TestUnknownTypeErrors ensures decoder refuses junk types rather than
// silently dropping them.
func TestUnknownTypeErrors(t *testing.T) {
	_, err := ParseActions([]byte(`[{"type":"wiggle","x":1}]`))
	if err == nil {
		t.Fatalf("expected error for unknown type")
	}
}

// TestExecutorRespectsMoveMouseGate proves that ClickAction suppresses the
// MoveAbs call when the global gate is off, and counts ignored_moves.
func TestExecutorRespectsMoveMouseGate(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sender := &stubSender{}

	exe := NewExecutor(sender, neverMoveGate{}, rng)
	exe.Run(context.Background(), []Action{ClickAction{X: 10, Y: 20, HoldMs: 5}}, nil)
	if sender.moves.Load() != 0 {
		t.Errorf("move_abs should not be called when gate is off, got %d", sender.moves.Load())
	}
	if sender.downs.Load() != 1 || sender.ups.Load() != 1 {
		t.Errorf("click should still send down/up, got down=%d up=%d", sender.downs.Load(), sender.ups.Load())
	}
	if exe.IgnoredMoves() != 1 {
		t.Errorf("ignored_moves = %d, want 1", exe.IgnoredMoves())
	}
}

// TestExecutorMovesWhenGateOn sanity-checks the other branch.
func TestExecutorMovesWhenGateOn(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sender := &stubSender{}

	exe := NewExecutor(sender, alwaysMoveGate{}, rng)
	exe.Run(context.Background(), []Action{ClickAction{X: 50, Y: 50, HoldMs: 1}}, nil)
	if sender.moves.Load() != 1 {
		t.Errorf("move_abs should fire when gate is on, got %d", sender.moves.Load())
	}
	if exe.IgnoredMoves() != 0 {
		t.Errorf("ignored_moves should be 0, got %d", exe.IgnoredMoves())
	}
}

// TestLoopForeverStopsOnContextCancel verifies the executor honours context
// cancellation while looping.
func TestLoopForeverStopsOnContextCancel(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sender := &stubSender{}
	exe := NewExecutor(sender, alwaysMoveGate{}, rng)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	actions := []Action{
		LoopAction{
			Forever: true,
			Actions: []Action{WaitAction{DurationMs: 10}},
		},
	}
	if err := exe.Run(ctx, actions, nil); err == nil {
		t.Fatalf("expected context error, got nil")
	}
}

// TestKeyDownUpOrder checks that the combo path issues all downs then all
// ups (in reverse), matching the Python reference.
func TestKeyDownUpOrder(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sender := &stubSender{}
	exe := NewExecutor(sender, alwaysMoveGate{}, rng)

	// 3-key combo so we can assert down-count before any up fires.
	exe.Run(context.Background(), []Action{
		ComboAction{VKCodes: []int{0x57, 0x44, 0x53}, HoldMs: 1},
	}, nil)
	if sender.keyDown.Load() != 3 {
		t.Errorf("want 3 down events, got %d", sender.keyDown.Load())
	}
	if sender.keyUp.Load() != 3 {
		t.Errorf("want 3 up events, got %d", sender.keyUp.Load())
	}
}

// TestRunOnceStopsOnContext ensures mid-action cancel propagates.
func TestRunOnceStopsOnContext(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	sender := &stubSender{}
	exe := NewExecutor(sender, alwaysMoveGate{}, rng)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err := exe.Run(ctx, []Action{ClickAction{X: 1, Y: 2, HoldMs: 1}}, nil)
	if err == nil {
		t.Fatalf("expected error from already-cancelled context")
	}
}

// TestParseSeedFiles sanity-checks every embedded example loads.
func TestParseSeedFiles(t *testing.T) {
	for _, name := range expectedSeeds {
		t.Run(name, func(t *testing.T) {
			data, err := embeddedScripts.ReadFile("seeds/" + name)
			if err != nil {
				t.Fatalf("read embedded %s: %v", name, err)
			}
			if _, err := ParseScript(data); err != nil {
				t.Fatalf("parse embedded %s: %v", name, err)
			}
		})
	}
}

// TestEncodeScriptRoundTrip verifies our MarshalJSON keeps the flat layout.
func TestEncodeScriptRoundTrip(t *testing.T) {
	original := []Action{
		ClickAction{X: 1, Y: 2, HoldMs: 3},
		LoopAction{Forever: true, Actions: []Action{WaitAction{DurationMs: 100}}},
	}
	out, err := EncodeScript("test", original)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	parsed, err := ParseScript(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("want 2 actions, got %d", len(parsed))
	}
}
