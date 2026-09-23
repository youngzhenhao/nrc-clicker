package actions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// Manager owns the on-disk action scripts directory. Mirrors
// ActionScript.ActionScriptManager.
//
// Concurrency: list / save / delete are called from the GUI goroutine; the
// executor never touches disk. No locking needed in phase 1.
type Manager struct {
	dir    string
	logger *logrus.Entry
}

// NewManager creates the scripts directory if it does not exist and returns
// a manager rooted at dir (e.g. data/action_scripts).
func NewManager(dir string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create scripts dir: %w", err)
	}
	return &Manager{dir: dir, logger: logrus.WithField("subsystem", "actions.manager")}, nil
}

// Dir returns the absolute scripts directory.
func (m *Manager) Dir() string { return m.dir }

// List returns the stem of every *.json file in the scripts dir. Order is
// sorted alphabetically so the GUI sees a stable list across launches.
func (m *Manager) List() ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(m.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(matches))
	for _, p := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// Load reads and parses a single script file.
func (m *Manager) Load(name string) ([]Action, error) {
	p := filepath.Join(m.dir, name+".json")
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	out, err := ParseScript(data)
	if err != nil {
		return nil, err
	}
	m.logger.Infof("loaded %s (%d actions)", name, len(out))
	return out, nil
}

// Save writes a script (creating or overwriting). The on-disk format
// matches the Python reference's save_script helper.
func (m *Manager) Save(name string, actions []Action) error {
	if name == "" {
		return fmt.Errorf("script name is required")
	}
	body, err := EncodeScript(name, actions)
	if err != nil {
		return err
	}
	p := filepath.Join(m.dir, name+".json")
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", p, err)
	}
	m.logger.Infof("saved %s (%d actions)", name, len(actions))
	return nil
}

// Delete removes a script by stem. Missing files are not an error so the
// caller can call Delete idempotently.
func (m *Manager) Delete(name string) error {
	p := filepath.Join(m.dir, name+".json")
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.logger.Infof("deleted %s", name)
	return nil
}

// IsEmpty reports whether the scripts directory has any *.json files.
func (m *Manager) IsEmpty() (bool, error) {
	matches, err := filepath.Glob(filepath.Join(m.dir, "*.json"))
	if err != nil {
		return false, err
	}
	return len(matches) == 0, nil
}

// SeedIfEmpty copies every built-in example script into the scripts dir
// when it is currently empty. The Python reference ships its seeds as
// actual files in data/action_scripts/; the Go version embeds them so the
// binary is self-contained on first launch.
func (m *Manager) SeedIfEmpty(embedded embedFS, embeddedDir string) error {
	empty, err := m.IsEmpty()
	if err != nil {
		return err
	}
	if !empty {
		return nil
	}
	entries, err := embedded.ReadDir(embeddedDir)
	if err != nil {
		return fmt.Errorf("read embedded dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := embedded.ReadFile(filepath.Join(embeddedDir, e.Name()))
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", e.Name(), err)
		}
		// Validate the JSON before copying so a buggy embedded file doesn't
		// silently land in the user's data dir.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(data, &probe); err != nil {
			m.logger.Warnf("skipping invalid embedded seed %s: %v", e.Name(), err)
			continue
		}
		dst := filepath.Join(m.dir, e.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write seed %s: %w", e.Name(), err)
		}
		m.logger.Infof("seeded %s", e.Name())
	}
	return nil
}
