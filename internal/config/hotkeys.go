// Package config holds the on-disk JSON configuration: hotkey bindings
// (and, in a future phase, presets + path-planner parameters).
//
// The file format mirrors ConfigManager.py: a single JSON object mapping
// logical names ("pause_resume", …) to F-key strings ("F2", …).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FKeyVK is the Windows virtual key code for each F-key. Mirrors
// ConfigManager.FKEY_VK exactly.
var FKeyVK = map[string]int{
	"F1":  0x70,
	"F2":  0x71,
	"F3":  0x72,
	"F4":  0x73,
	"F5":  0x74,
	"F6":  0x75,
	"F7":  0x76,
	"F8":  0x77,
	"F9":  0x78,
	"F10": 0x79,
	"F11": 0x7A,
	"F12": 0x7B,
}

// DefaultHotkeys matches the Python reference's DEFAULT_HOTKEYS. Note:
// recording-related entries are kept for forward compatibility (so a future
// recording-mode port reuses the same JSON file), even though recording
// is out of scope for phase 1.
var DefaultHotkeys = map[string]string{
	"pause_resume":    "F2",
	"start_recording": "F7",
	"stop_recording":  "F8",
	"cancel_recording": "F9",
	"mark_anchor":     "F12",
}

// HotkeyLabels is used by the UI to render role names.
var HotkeyLabels = map[string]string{
	"pause_resume":    "暂停/继续",
	"start_recording": "开始录制",
	"stop_recording":  "停止录制并保存",
	"cancel_recording": "取消录制",
	"mark_anchor":     "标记锚点（录制中）",
}

// Hotkeys is the in-memory shape: role → F-key string.
type Hotkeys map[string]string

// LoadHotkeys reads the JSON config at dir/hotkeys.json and merges it on top
// of DefaultHotkeys. Unknown keys are dropped; invalid F-key values fall
// back to the default.
func LoadHotkeys(dir string) (Hotkeys, error) {
	out := Hotkeys{}
	for k, v := range DefaultHotkeys {
		out[k] = v
	}
	p := filepath.Join(dir, "hotkeys.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("read hotkeys: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return out, fmt.Errorf("decode hotkeys: %w", err)
	}
	for k, v := range raw {
		if _, known := DefaultHotkeys[k]; !known {
			continue
		}
		if _, valid := FKeyVK[v]; !valid {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// SaveHotkeys validates and writes the binding to disk. Empty name means
// the default file (hotkeys.json).
func SaveHotkeys(dir string, h Hotkeys) error {
	if err := ValidateHotkeys(h); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "hotkeys.json"), data, 0o644)
}

// ValidateHotkeys rejects duplicate F-key assignments across different
// roles. The Python reference does this too; we surface the conflicting
// key(s) in the error so the UI can toast a specific message.
func ValidateHotkeys(h Hotkeys) error {
	seen := make(map[string]string, len(h))
	conflicts := make(map[string]struct{})
	for role, fkey := range h {
		if _, ok := DefaultHotkeys[role]; !ok {
			return fmt.Errorf("unknown hotkey role %q", role)
		}
		if _, ok := FKeyVK[fkey]; !ok {
			return fmt.Errorf("invalid F-key %q for role %q", fkey, role)
		}
		if other, dup := seen[fkey]; dup {
			conflicts[fkey] = struct{}{}
			_ = other
		}
		seen[fkey] = role
	}
	if len(conflicts) > 0 {
		keys := make([]string, 0, len(conflicts))
		for k := range conflicts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return &ConflictError{Keys: keys}
	}
	return nil
}

// ConflictError is returned by ValidateHotkeys when at least two roles
// share an F-key. Callers can switch on the type to render a specific
// toast (e.g., "❌ 按键冲突：F2 被多个功能使用").
type ConflictError struct {
	Keys []string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("hotkey conflicts on: %v", e.Keys)
}

// IsConflict extracts a ConflictError if present, or nil.
func IsConflict(err error) *ConflictError {
	var c *ConflictError
	if errors.As(err, &c) {
		return c
	}
	return nil
}

// VKFor returns the Windows virtual key code for a role's F-key binding.
// Returns 0 when the role is unknown or the F-key is invalid.
func VKFor(h Hotkeys, role string) int {
	fkey, ok := h[role]
	if !ok {
		return 0
	}
	return FKeyVK[fkey]
}
