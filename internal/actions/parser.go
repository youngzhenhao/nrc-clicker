package actions

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// supportedSchemaVersion is the highest script schema version this binary
// understands. Newer files (with a meta.version > supported) are rejected
// so a future migration isn't silently misread.
const supportedSchemaVersion = 1

// scriptEnvelope is the on-disk wrapper for an action script. We use
// json.RawMessage for Actions so the per-element decode (which IS strict)
// happens in decodeAction.
type scriptEnvelope struct {
	Meta    meta            `json:"meta"`
	Name    string          `json:"name"`
	Actions []json.RawMessage `json:"actions"`
}

type meta struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	Name    string `json:"name"`
}

// ParseScript decodes a script.json payload into a slice of Action values.
// Unknown action types produce an error rather than a silent skip.
func ParseScript(data []byte) ([]Action, error) {
	var env scriptEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode script: %w", err)
	}
	if env.Meta.Version > supportedSchemaVersion {
		return nil, fmt.Errorf("script schema version %d is newer than supported %d",
			env.Meta.Version, supportedSchemaVersion)
	}
	out := make([]Action, 0, len(env.Actions))
	for i, raw := range env.Actions {
		a, err := decodeAny(raw)
		if err != nil {
			return nil, fmt.Errorf("action[%d]: %w", i, err)
		}
		if a != nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// decodeAny inspects the "type" discriminator and decodes the body with
// DisallowUnknownFields enabled — that's how we catch typos like
// {"type":"click","x":1,"hold_ms":80,"holD_ms":100} at load time.
func decodeAny(raw json.RawMessage) (Action, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return nil, fmt.Errorf("missing type: %w", err)
	}
	// Strip the "type" key before the strict body decode so per-type structs
	// (which DON'T have a Type field) don't see it as an unknown field.
	body := stripTypeField(raw)

	switch head.Type {
	case "click":
		var c ClickAction
		if err := strictUnmarshal(body, &c); err != nil {
			return nil, err
		}
		return c, nil
	case "move":
		var m MoveAction
		if err := strictUnmarshal(body, &m); err != nil {
			return nil, err
		}
		return m, nil
	case "key":
		var k KeyAction
		if err := strictUnmarshal(body, &k); err != nil {
			return nil, err
		}
		return k, nil
	case "combo":
		var c ComboAction
		if err := strictUnmarshal(body, &c); err != nil {
			return nil, err
		}
		return c, nil
	case "wait":
		var w WaitAction
		if err := strictUnmarshal(body, &w); err != nil {
			return nil, err
		}
		return w, nil
	case "loop":
		var l loopJSON
		if err := strictUnmarshal(body, &l); err != nil {
			return nil, err
		}
		children := make([]Action, 0, len(l.Actions))
		for i, sub := range l.Actions {
			a, err := decodeAny(sub)
			if err != nil {
				return nil, fmt.Errorf("loop[%d]: %w", i, err)
			}
			if a != nil {
				children = append(children, a)
			}
		}
		count := l.Count
		forever := l.Forever || l.UntilExit || count <= 0
		if count <= 0 {
			count = 1
		}
		return LoopAction{
			Actions:       children,
			Count:         count,
			Forever:       forever,
			PauseMs:       l.PauseMs,
			PauseJitterMs: l.PauseJitterMs,
		}, nil
	case "timed":
		var t timedJSON
		if err := strictUnmarshal(body, &t); err != nil {
			return nil, err
		}
		children := make([]Action, 0, len(t.Actions))
		for i, sub := range t.Actions {
			a, err := decodeAny(sub)
			if err != nil {
				return nil, fmt.Errorf("timed[%d]: %w", i, err)
			}
			if a != nil {
				children = append(children, a)
			}
		}
		return TimedAction{
			Actions:   children,
			ExecuteMs: t.ExecuteMs,
			SleepMs:   t.SleepMs,
			Repeat:    t.Repeat,
			Forever:   t.Forever,
		}, nil
	default:
		return nil, fmt.Errorf("unknown action type %q", head.Type)
	}
}

// strictUnmarshal decodes raw into v with DisallowUnknownFields enabled.
// Caller passes a typed struct so unknown keys fail loud.
func strictUnmarshal(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// stripTypeField decodes raw into a map, drops the "type" key, and re-
// encodes the remainder. Used so per-type decoders (which don't have a
// Type field) can use DisallowUnknownFields without spurious failures.
//
// On any decode failure we fall back to the original raw — better to
// surface the real error from the strict decoder than mask it here.
func stripTypeField(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	delete(m, "type")
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// loopJSON / timedJSON mirror the on-disk schema but keep nested actions as
// json.RawMessage. They get decoded recursively via decodeAny.
type loopJSON struct {
	Actions       []json.RawMessage `json:"actions"`
	Count         int               `json:"count"`
	Forever       bool              `json:"forever"`
	UntilExit     bool              `json:"until_exit"`
	PauseMs       int               `json:"pause_ms"`
	PauseJitterMs int               `json:"pause_jitter_ms"`
}

type timedJSON struct {
	Actions   []json.RawMessage `json:"actions"`
	ExecuteMs int               `json:"execute_ms"`
	SleepMs   int               `json:"sleep_ms"`
	Repeat    int               `json:"repeat"`
	Forever   bool              `json:"forever"`
}

// ParseActions decodes a top-level JSON array of action objects (used by
// tests and by callers that already stripped the meta envelope).
func ParseActions(data []byte) ([]Action, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode actions: %w", err)
	}
	out := make([]Action, 0, len(raws))
	for i, raw := range raws {
		a, err := decodeAny(raw)
		if err != nil {
			return nil, fmt.Errorf("action[%d]: %w", i, err)
		}
		if a != nil {
			out = append(out, a)
		}
	}
	return out, nil
}

// EncodeScript serialises a slice of actions into the on-disk script format
// with the standard meta header. Used by SaveScript.
func EncodeScript(name string, actions []Action) ([]byte, error) {
	env := struct {
		Meta    meta       `json:"meta"`
		Name    string     `json:"name"`
		Actions []Action   `json:"actions"`
	}{
		Meta: meta{Type: "action", Version: supportedSchemaVersion, Name: name},
		Name: name,
		Actions: actions,
	}
	out, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return out, nil
}
