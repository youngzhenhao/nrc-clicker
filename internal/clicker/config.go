// Package clicker is the simple auto-clicker loop: send a left click at a
// fixed centre coordinate with configurable interval, hold, radius, and
// jitter. The loop runs in a single goroutine and respects pause / stop
// signals over channels.
package clicker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

// Config holds every tweakable parameter of the simple clicker loop.
//
// We use atomic.Pointer[Config] (see manager) so the running loop can
// snapshot the latest values each iteration without tearing the click.
type Config struct {
	// Centre is the centre of the click region in screen pixels.
	CentreX int `json:"center_x"`
	CentreY int `json:"center_y"`

	// Radius is the maximum random offset applied around Centre (pixels).
	Radius int `json:"radius"`

	// MoveMouse, when false, suppresses the pre-click MoveAbs so the
	// cursor stays where it was. The down/up events still fire at the
	// current cursor position.
	MoveMouse bool `json:"move_mouse"`

	// IntervalMs is the wait between successive click cycles.
	IntervalMs int `json:"click_interval"`

	// HoldMs is how long the left button stays down per click.
	HoldMs int `json:"hold_duration"`

	// JitterRangeMs is ±jitter applied to IntervalMs each cycle.
	JitterRangeMs int `json:"jitter_range"`

	// MicroPauseEveryMin/Max and MicroPauseMinMs/MaxMs are reserved for
	// future "human-like micro pause every N clicks" behaviour. Zero in
	// default.json; turning the feature on later doesn't require a schema
	// migration.
	MicroPauseEveryMin int `json:"micro_pause_every_min,omitempty"`
	MicroPauseEveryMax int `json:"micro_pause_every_max,omitempty"`
	MicroPauseMinMs    int `json:"micro_pause_min_ms,omitempty"`
	MicroPauseMaxMs    int `json:"micro_pause_max_ms,omitempty"`
}

// DefaultConfig matches the Python ClickerConfig defaults (centre of
// the primary monitor, 30px radius, 100ms interval, 50ms hold, 20ms jitter,
// move_mouse=true).
func DefaultConfig() Config {
	return Config{
		CentreX:        960,
		CentreY:        540,
		Radius:         30,
		MoveMouse:      true,
		IntervalMs:     100,
		HoldMs:         50,
		JitterRangeMs:  20,
		MicroPauseEveryMin: 0,
		MicroPauseEveryMax: 0,
		MicroPauseMinMs:    0,
		MicroPauseMaxMs:    0,
	}
}

// LoadConfig reads a config JSON from data/clicker_configs/<name>.json.
// Missing files return DefaultConfig without error.
func LoadConfig(dir, name string) (Config, error) {
	cfg := DefaultConfig()
	p := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config %s: %w", p, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config %s: %w", p, err)
	}
	return cfg, nil
}

// SaveConfig writes the config to disk. Empty name means "default".
func SaveConfig(dir, name string, cfg Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, name+".json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// Store is an atomic holder of the latest Config. The clicker loop and the
// UI both go through it; UI updates never tear a click in flight.
type Store struct {
	ptr atomic.Pointer[Config]
}

// NewStore seeds the store with cfg.
func NewStore(cfg Config) *Store {
	s := &Store{}
	s.ptr.Store(&cfg)
	return s
}

// Load returns the current config (snapshot — caller may mutate freely).
func (s *Store) Load() Config {
	return *s.ptr.Load()
}

// Store atomically replaces the held config.
func (s *Store) Store(cfg Config) {
	s.ptr.Store(&cfg)
}

// MoveMouse implements actions.MouseMoveGate — the action executor reads
// this at every click to decide whether to issue a MoveAbs. Atomic read
// means we never see a torn value.
func (s *Store) MoveMouse() bool { return s.Load().MoveMouse }
