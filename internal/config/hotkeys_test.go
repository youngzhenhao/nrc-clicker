package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateHotkeysOK(t *testing.T) {
	h := Hotkeys{
		"pause_resume":     "F2",
		"start_recording":  "F3",
		"stop_recording":   "F4",
		"cancel_recording": "F5",
		"mark_anchor":      "F12",
	}
	if err := ValidateHotkeys(h); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateHotkeysRejectsConflict(t *testing.T) {
	h := Hotkeys{
		"pause_resume":     "F2",
		"start_recording":  "F2", // conflict
		"stop_recording":   "F8",
		"cancel_recording": "F9",
	}
	err := ValidateHotkeys(h)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	c := IsConflict(err)
	if c == nil {
		t.Fatalf("expected *ConflictError, got %T", err)
	}
	if len(c.Keys) != 1 || c.Keys[0] != "F2" {
		t.Errorf("conflict keys = %v, want [F2]", c.Keys)
	}
}

func TestValidateHotkeysRejectsInvalidFKey(t *testing.T) {
	h := Hotkeys{"pause_resume": "F99"}
	if err := ValidateHotkeys(h); err == nil {
		t.Fatalf("expected error for invalid F-key")
	}
}

func TestValidateHotkeysRejectsUnknownRole(t *testing.T) {
	h := Hotkeys{"nonexistent": "F2"}
	if err := ValidateHotkeys(h); err == nil {
		t.Fatalf("expected error for unknown role")
	}
}

func TestLoadHotkeysDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	h, err := LoadHotkeys(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for role, def := range DefaultHotkeys {
		if h[role] != def {
			t.Errorf("role %q = %q, want %q", role, h[role], def)
		}
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	want := Hotkeys{
		"pause_resume":     "F3",
		"start_recording":  "F7",
		"stop_recording":   "F8",
		"cancel_recording": "F9",
		"mark_anchor":      "F12",
	}
	if err := SaveHotkeys(dir, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadHotkeys(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("role %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadHotkeysIgnoresUnknownRole(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hotkeys.json")
	if err := os.WriteFile(path, []byte(`{"pause_resume":"F2","invented":"F7"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHotkeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := h["invented"]; present {
		t.Errorf("unknown role should be dropped")
	}
}

func TestErrorsAsConflict(t *testing.T) {
	err := ValidateHotkeys(Hotkeys{"pause_resume": "F2", "start_recording": "F2"})
	var target *ConflictError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As failed: %v", err)
	}
}
