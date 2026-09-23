// Package actions defines the action types the script executor understands
// and the executor itself. The schema mirrors ActionScript.py exactly so
// existing JSON scripts (data/action_scripts/*.json) round-trip without
// manual edits.
//
// Seven action types are supported:
//
//	click, move, key, combo, wait   (leaf actions, executed by Executor)
//	loop, timed                      (container actions, recursed into)
//
// JSON encoding is "flat" (matches the Python reference):
//
//	{ "type": "click", "x": 900, "y": 500, "hold_ms": 80 }
//
// not nested under a discriminator key.
package actions

import "encoding/json"

// Action is the interface every action type satisfies. Executor.run switches
// on the concrete type to dispatch; we keep MarshalJSON per-concrete-type so
// the JSON layout stays flat.
type Action interface {
	// actionType returns the JSON "type" discriminator ("click", "loop", …).
	actionType() string
}

// --- leaf actions ---------------------------------------------------------

// ClickAction moves the cursor (if move_mouse is enabled) then issues a
// left-click with the requested hold duration.
type ClickAction struct {
	X             int `json:"x"`
	Y             int `json:"y"`
	HoldMs        int `json:"hold_ms,omitempty"`
	XJitterPx     int `json:"x_jitter_px,omitempty"`
	YJitterPx     int `json:"y_jitter_px,omitempty"`
	HoldJitterMs  int `json:"hold_jitter_ms,omitempty"`
}

func (ClickAction) actionType() string { return "click" }
func (a ClickAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("click", a)
}

// MoveAction moves the cursor to (x, y) with an optional move duration.
type MoveAction struct {
	X                int `json:"x"`
	Y                int `json:"y"`
	DurationMs       int `json:"duration_ms,omitempty"`
	XJitterPx        int `json:"x_jitter_px,omitempty"`
	YJitterPx        int `json:"y_jitter_px,omitempty"`
	DurationJitterMs int `json:"duration_jitter_ms,omitempty"`
}

func (MoveAction) actionType() string { return "move" }
func (a MoveAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("move", a)
}

// KeyAction presses and releases a single key identified by VK code.
type KeyAction struct {
	VKCode        int `json:"vk_code"`
	HoldMs        int `json:"hold_ms,omitempty"`
	HoldJitterMs  int `json:"hold_jitter_ms,omitempty"`
}

func (KeyAction) actionType() string { return "key" }
func (a KeyAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("key", a)
}

// ComboAction presses several keys simultaneously (down all, hold, then up
// in reverse order). Used for chord inputs like WASD diagonal moves.
type ComboAction struct {
	VKCodes      []int `json:"vk_codes"`
	HoldMs       int   `json:"hold_ms,omitempty"`
	HoldJitterMs int   `json:"hold_jitter_ms,omitempty"`
}

func (ComboAction) actionType() string { return "combo" }
func (a ComboAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("combo", a)
}

// WaitAction sleeps for the given duration with optional jitter.
type WaitAction struct {
	DurationMs       int `json:"duration_ms"`
	DurationJitterMs int `json:"duration_jitter_ms,omitempty"`
}

func (WaitAction) actionType() string { return "wait" }
func (a WaitAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("wait", a)
}

// --- container actions ----------------------------------------------------

// LoopAction repeats its sub-actions count times (or forever when
// Forever / UntilExit is set or count <= 0). PauseMs is an optional sleep
// between iterations that respects pause/stop.
type LoopAction struct {
	Actions       []Action `json:"actions"`
	Count         int      `json:"count,omitempty"`
	Forever       bool     `json:"forever,omitempty"`
	UntilExit     bool     `json:"until_exit,omitempty"`
	PauseMs       int      `json:"pause_ms,omitempty"`
	PauseJitterMs int      `json:"pause_jitter_ms,omitempty"`
}

func (LoopAction) actionType() string { return "loop" }
func (a LoopAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("loop", a)
}

// TimedAction executes its sub-actions for ExecuteMs (the "on" window),
// then sleeps SleepMs (the "off" window), then repeats Repeat times or
// forever. The execute window timer is extended by any time the script
// spent paused.
type TimedAction struct {
	Actions   []Action `json:"actions"`
	ExecuteMs int      `json:"execute_ms"`
	SleepMs   int      `json:"sleep_ms"`
	Repeat    int      `json:"repeat,omitempty"`
	Forever   bool     `json:"forever,omitempty"`
}

func (TimedAction) actionType() string { return "timed" }
func (a TimedAction) MarshalJSON() ([]byte, error) {
	return marshalFlat("timed", a)
}

// --- helpers --------------------------------------------------------------

// marshalFlat re-encodes a typed action into {"type":"...", ...} layout.
//
// We hand-build the JSON for every concrete type rather than calling
// json.Marshal(v) — because v has a MarshalJSON method, doing so would
// recurse forever. Even for container actions (Loop / Timed) we recurse
// through toMap which inlines the nested Action values.
func marshalFlat(kind string, v any) ([]byte, error) {
	m := toMap(v)
	if m == nil {
		return json.Marshal(map[string]string{"type": kind})
	}
	// Defensive: toMap already sets type, but keep this in case future
	// refactors drop the assignment.
	m["type"] = kind
	return json.Marshal(m)
}

// _ discards the unused kind parameter to keep call-sites readable when
// we later decide to derive kind from v directly via a switch.
var _ = func(kind string, v any) ([]byte, error) {
	m := toMap(v)
	if m == nil {
		return json.Marshal(map[string]string{"type": kind})
	}
	return json.Marshal(m)
}

// toMap turns an Action (or any of the concrete action types) into a
// generic map[string]any ready for json.Marshal. Nested Action values in
// LoopAction / TimedAction are recursively resolved so the marshal step
// never sees the MarshalJSON method.
//
// Each map carries its own "type" discriminator so nested children decode
// back into the right concrete struct.
func toMap(v any) map[string]any {
	switch a := v.(type) {
	case ClickAction:
		return typed("click", map[string]any{
			"x":              a.X,
			"y":              a.Y,
			"hold_ms":        omitZero(a.HoldMs),
			"x_jitter_px":    omitZero(a.XJitterPx),
			"y_jitter_px":    omitZero(a.YJitterPx),
			"hold_jitter_ms": omitZero(a.HoldJitterMs),
		})
	case MoveAction:
		return typed("move", map[string]any{
			"x":                  a.X,
			"y":                  a.Y,
			"duration_ms":        omitZero(a.DurationMs),
			"x_jitter_px":        omitZero(a.XJitterPx),
			"y_jitter_px":        omitZero(a.YJitterPx),
			"duration_jitter_ms": omitZero(a.DurationJitterMs),
		})
	case KeyAction:
		return typed("key", map[string]any{
			"vk_code":        a.VKCode,
			"hold_ms":        omitZero(a.HoldMs),
			"hold_jitter_ms": omitZero(a.HoldJitterMs),
		})
	case ComboAction:
		return typed("combo", map[string]any{
			"vk_codes":       a.VKCodes,
			"hold_ms":        omitZero(a.HoldMs),
			"hold_jitter_ms": omitZero(a.HoldJitterMs),
		})
	case WaitAction:
		return typed("wait", map[string]any{
			"duration_ms":        a.DurationMs,
			"duration_jitter_ms": omitZero(a.DurationJitterMs),
		})
	case LoopAction:
		children := make([]any, 0, len(a.Actions))
		for _, c := range a.Actions {
			children = append(children, toMap(c))
		}
		return typed("loop", map[string]any{
			"actions":         children,
			"count":           omitZero(a.Count),
			"forever":         omitFalse(a.Forever),
			"until_exit":      omitFalse(a.UntilExit),
			"pause_ms":        omitZero(a.PauseMs),
			"pause_jitter_ms": omitZero(a.PauseJitterMs),
		})
	case TimedAction:
		children := make([]any, 0, len(a.Actions))
		for _, c := range a.Actions {
			children = append(children, toMap(c))
		}
		return typed("timed", map[string]any{
			"actions":    children,
			"execute_ms": a.ExecuteMs,
			"sleep_ms":   a.SleepMs,
			"repeat":     omitZero(a.Repeat),
			"forever":    omitFalse(a.Forever),
		})
	}
	return nil
}

// typed sets the "type" discriminator on the map. Centralised so adding new
// fields doesn't drift between the seven action types.
func typed(kind string, m map[string]any) map[string]any {
	m["type"] = kind
	return m
}

// omitZero mirrors json:",omitempty" for ints without forcing the caller to
// write a wrapper struct for every field.
func omitZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

// omitFalse mirrors json:",omitempty" for bools (false is omitted).
func omitFalse(v bool) any {
	if !v {
		return nil
	}
	return v
}
