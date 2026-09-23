// Package manager is the orchestrator: it ties the clicker, the action
// executor, and the hotkey listener together behind a single Status()
// snapshot the GUI polls at 5 Hz.
//
// Lifecycle (Shutdown order is load-bearing — see comments in Shutdown):
//
//  1. running = false                              // stops dispatch goroutine
//  2. scriptCancel() + scriptWG.Wait(2s)           // stop script session
//  3. clicker.Stop()                               // join clicker goroutine
//  4. hotkey.Stop()                                // post WM_QUIT, join producer
//  5. interception.Destroy()                       // only after every send quiesced
//  6. logrus.Info("shutdown complete")
package manager

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"nrc-clicker/internal/actions"
	"nrc-clicker/internal/clicker"
	"nrc-clicker/internal/config"
	"nrc-clicker/internal/hotkey"
	"nrc-clicker/internal/interception"
)

// Sender combines both halves of the Interception driver (mouse + keyboard).
// Implemented by *interception.Core; tests can substitute a stub.
type Sender interface {
	interception.MouseSender
	interception.KeyboardSender
}

// Manager owns every long-lived subsystem. There should be exactly one
// per process.
type Manager struct {
	logger *logrus.Entry

	core     *interception.Core
	sender   Sender // == core normally; exposed via interface for tests
	clicker  *clicker.Clicker
	executor *actions.Executor
	hotkey   *hotkey.Listener

	cfgStore *clicker.Store // shared with the UI

	scriptMgr *actions.Manager

	// Hotkey bindings (also persisted to disk).
	hotkeysMu sync.RWMutex
	hotkeys   config.Hotkeys

	// Script session state.
	sessMu      sync.Mutex
	scriptCtx   context.Context
	scriptCancel context.CancelFunc
	scriptWG    sync.WaitGroup
	currentName string

	// Countdown / state flags read by the UI.
	countdownEnd   atomic.Int64 // unix nano; 0 means no countdown
	countdownLabel atomic.Value // string

	// Lifecycle.
	started atomic.Bool
	closing atomic.Bool
}

// Options configures New. Sender may be nil for unit tests (the manager
// still constructs but clicker/executor work against nil — they'll panic
// only if you try to Start them).
type Options struct {
	Sender     Sender
	Core       *interception.Core // for Destroy()
	Hotkeys    config.Hotkeys
	Config     clicker.Config
	ScriptsDir string
	ConfigsDir string
}

// New wires everything together.
func New(opts Options) (*Manager, error) {
	if opts.Sender == nil {
		return nil, errMissingSender
	}
	if opts.Hotkeys == nil {
		opts.Hotkeys = config.DefaultHotkeys
	}
	if opts.ScriptsDir == "" {
		opts.ScriptsDir = "data/action_scripts"
	}
	if opts.ConfigsDir == "" {
		opts.ConfigsDir = "data/clicker_configs"
	}

	mgr := &Manager{
		logger:   logrus.WithField("subsystem", "manager"),
		core:     opts.Core,
		sender:   opts.Sender,
		hotkeys:  opts.Hotkeys,
		cfgStore: clicker.NewStore(opts.Config),
		hotkey:   hotkey.New(),
		countdownLabel: atomic.Value{},
	}
	mgr.countdownLabel.Store("")

	// Load persisted default config on top of caller-supplied defaults.
	if cfg, err := clicker.LoadConfig(opts.ConfigsDir, "default"); err == nil {
		mgr.cfgStore.Store(cfg)
	}

	// Build the script manager; seed the example scripts on first launch.
	sm, err := actions.NewManager(opts.ScriptsDir)
	if err != nil {
		return nil, err
	}
	if err := sm.SeedIfEmpty(actions.Seeds(), "seeds"); err != nil {
		mgr.logger.Warnf("seed scripts: %v", err)
	}
	mgr.scriptMgr = sm

	// Executor uses the same sender and reads MoveMouse via cfgStore.
	mgr.executor = actions.NewExecutor(opts.Sender, mgr.cfgStore, nil)
	mgr.executor.ResetIgnoredMoves()

	// Clicker reads its own snapshot of cfgStore.
	mgr.clicker = clicker.New(opts.Sender, mgr.cfgStore)

	// Hook up the hotkeys the user has bound.
	mgr.hotkey.Watch(vkList(opts.Hotkeys))
	mgr.hotkey.Start()

	// Background dispatcher: drains events, dispatches to clicker / script.
	mgr.started.Store(true)
	go mgr.dispatchLoop()
	mgr.logger.Info("manager initialised")
	return mgr, nil
}

// errMissingSender is the only fatal constructor error.
type stringError string

func (e stringError) Error() string { return string(e) }

var errMissingSender = stringError("manager: Sender is required")

// --- status snapshot ------------------------------------------------------

// Snapshot is the read-only view the UI polls. All fields are value types
// or atomic.Pointer copies; safe to access from any goroutine.
type Snapshot struct {
	InterceptionReady bool
	InterceptionError string

	SimpleClickerRunning bool
	SimpleClickerPaused  bool
	ClickerCount         int64

	ScriptRunning  bool
	ScriptPaused   bool
	ScriptName     string
	IgnoredMoves   int64

	CountdownRemaining int // seconds; 0 when no countdown
	CountdownLabel     string

	Hotkeys config.Hotkeys
	Config  clicker.Config
}

// Status returns a copy of the current state.
func (m *Manager) Status() Snapshot {
	s := Snapshot{
		InterceptionReady: m.core == nil || m.core.IsReady(),
		SimpleClickerRunning: m.clicker.Running(),
		SimpleClickerPaused:  m.clicker.Paused(),
		ClickerCount:         m.clicker.ClickCount(),
		ScriptRunning:        m.scriptRunning(),
		ScriptPaused:         m.scriptPaused(),
		ScriptName:           m.currentScript(),
		IgnoredMoves:         m.executor.IgnoredMoves(),
		Hotkeys:              m.snapshotHotkeys(),
		Config:               m.cfgStore.Load(),
	}
	if m.core != nil && !m.core.IsReady() {
		s.InterceptionError = m.core.LastError()
	}
	if end := m.countdownEnd.Load(); end > 0 {
		remaining := time.Until(time.Unix(0, end))
		if remaining > 0 {
			s.CountdownRemaining = int(remaining.Seconds() + 0.5)
			if v := m.countdownLabel.Load(); v != nil {
				s.CountdownLabel, _ = v.(string)
			}
		}
	}
	return s
}

// --- script session -------------------------------------------------------

// RunScript loads and executes a saved script in a worker goroutine.
// Returns immediately. Conflicts with a running session or clicker.
func (m *Manager) RunScript(name string) error {
	actions, err := m.scriptMgr.Load(name)
	if err != nil {
		return err
	}
	if m.clicker.Running() {
		m.clicker.Stop()
	}
	if m.scriptRunning() {
		m.stopScriptImmediate()
	}
	m.sessMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	m.scriptCtx = ctx
	m.scriptCancel = cancel
	m.sessMu.Unlock()
	m.setCurrentScript(name)

	m.scriptWG.Add(1)
	go func() {
		defer m.scriptWG.Done()
		defer m.setCurrentScript("")
		defer cancel()

		// 3-second countdown; honours stop during the wait.
		if err := m.runCountdown(ctx, "脚本即将启动: "+name, 3); err != nil {
			m.logger.Infof("script %q aborted during countdown", name)
			return
		}
		m.logger.Infof("script %q started", name)
		if err := m.executor.Run(ctx, actions, pauseOrNil(ctx)); err != nil && err != context.Canceled {
			m.logger.Warnf("script %q exited with %v", name, err)
		} else {
			m.logger.Infof("script %q finished", name)
		}
	}()
	return nil
}

// pauseOrNil returns the pause channel (open == paused, closed == running)
// when a session is active, else nil.
//
// For phase 1 we use a no-op implementation: scripts run straight through
// without pause support. Future recording-aware phases can replace this
// with a manager-controlled channel wired to the pause_resume hotkey.
func pauseOrNil(ctx context.Context) chan struct{} {
	return nil
}

func (m *Manager) runCountdown(ctx context.Context, label string, seconds int) error {
	if seconds <= 0 {
		return nil
	}
	m.countdownLabel.Store(label)
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	m.countdownEnd.Store(deadline.UnixNano())
	defer func() {
		m.countdownEnd.Store(0)
		m.countdownLabel.Store("")
	}()
	for i := seconds; i > 0; i-- {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil
}

// PauseScript pauses the current script. Currently a no-op (see RunScript).
func (m *Manager) PauseScript() { m.logger.Debug("PauseScript: phase-1 no-op") }

// ResumeScript resumes the current script. Currently a no-op.
func (m *Manager) ResumeScript() { m.logger.Debug("ResumeScript: phase-1 no-op") }

// StopScript cancels the running script session immediately.
func (m *Manager) StopScript() {
	m.stopScriptImmediate()
}

func (m *Manager) stopScriptImmediate() {
	m.sessMu.Lock()
	cancel := m.scriptCancel
	m.scriptCancel = nil
	m.sessMu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.scriptWG.Wait()
}

// --- simple clicker control -----------------------------------------------

// ToggleClicker starts or stops the simple clicker loop, with a 3-second
// countdown.
func (m *Manager) ToggleClicker() {
	if m.clicker.Running() {
		m.clicker.Stop()
		return
	}
	// Stop a running script so we don't double-send.
	if m.scriptRunning() {
		m.stopScriptImmediate()
	}
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := m.runCountdown(ctx, "连点器即将启动", 3); err != nil {
			return
		}
		m.clicker.Start()
	}()
}

// ToggleClickerPause flips the clicker pause state.
func (m *Manager) ToggleClickerPause() bool { return m.clicker.TogglePause() }

// UpdateConfig atomically swaps the clicker config.
func (m *Manager) UpdateConfig(c clicker.Config) { m.cfgStore.Store(c) }

// SetMoveMouse updates just the MoveMouse flag.
func (m *Manager) SetMoveMouse(v bool) {
	c := m.cfgStore.Load()
	c.MoveMouse = v
	m.cfgStore.Store(c)
}

// --- hotkey management ----------------------------------------------------

// SetHotkeys validates the new bindings and persists them. Returns a
// ConflictError on duplicates so the UI can surface a specific toast.
func (m *Manager) SetHotkeys(h config.Hotkeys) error {
	if err := config.ValidateHotkeys(h); err != nil {
		return err
	}
	m.hotkeysMu.Lock()
	m.hotkeys = h
	m.hotkeysMu.Unlock()
	m.hotkey.Watch(vkList(h))
	if err := config.SaveHotkeys("data/clicker_configs", h); err != nil {
		return err
	}
	return nil
}

func (m *Manager) snapshotHotkeys() config.Hotkeys {
	m.hotkeysMu.RLock()
	defer m.hotkeysMu.RUnlock()
	out := make(config.Hotkeys, len(m.hotkeys))
	for k, v := range m.hotkeys {
		out[k] = v
	}
	return out
}

func vkList(h config.Hotkeys) []int {
	out := make([]int, 0, len(h))
	for _, fkey := range h {
		if vk, ok := config.FKeyVK[fkey]; ok {
			out = append(out, vk)
		}
	}
	return out
}

// --- script CRUD ----------------------------------------------------------

// ListScripts returns the saved script names.
func (m *Manager) ListScripts() ([]string, error) { return m.scriptMgr.List() }

// DeleteScript removes a saved script.
func (m *Manager) DeleteScript(name string) error { return m.scriptMgr.Delete(name) }

// --- hotkey dispatch loop -------------------------------------------------

func (m *Manager) dispatchLoop() {
	for m.started.Load() {
		select {
		case ev, ok := <-m.hotkey.Events():
			if !ok {
				return
			}
			m.handleHotkey(ev)
		case <-time.After(500 * time.Millisecond):
			// Heartbeat so we exit promptly when Stop() flips started.
		}
	}
}

func (m *Manager) handleHotkey(ev hotkey.KeyEvent) {
	// Only react to key-down edges; ignore key-up to match Python.
	if ev.Up {
		return
	}
	m.hotkeysMu.RLock()
	pauseVK := config.VKFor(m.hotkeys, "pause_resume")
	m.hotkeysMu.RUnlock()
	if ev.VK != pauseVK {
		return
	}
	// Priority: script running → toggle script pause (no-op in phase 1 but
	// still surfaced so the user gets feedback); else if clicker running
	// → toggle clicker pause; else nothing.
	if m.scriptRunning() {
		if m.scriptPaused() {
			m.ResumeScript()
		} else {
			m.PauseScript()
		}
		return
	}
	if m.clicker.Running() {
		m.ToggleClickerPause()
	}
}

// --- shutdown -------------------------------------------------------------

// Shutdown stops everything in the documented order. Safe to call
// multiple times.
func (m *Manager) Shutdown() {
	if !m.closing.CompareAndSwap(false, true) {
		return
	}
	// 1. signalling
	m.started.Store(false)

	// 2. script session
	m.stopScriptImmediate()

	// 3. clicker
	m.clicker.Stop()

	// 4. hotkey
	m.hotkey.Stop()

	// 5. interception context
	if m.core != nil {
		m.core.Destroy()
	}

	m.logger.Info("shutdown complete")
}

// --- helpers --------------------------------------------------------------

func (m *Manager) currentScript() string {
	m.sessMu.Lock()
	defer m.sessMu.Unlock()
	return m.currentName
}

func (m *Manager) setCurrentScript(name string) {
	m.sessMu.Lock()
	defer m.sessMu.Unlock()
	m.currentName = name
}

func (m *Manager) scriptRunning() bool {
	m.sessMu.Lock()
	defer m.sessMu.Unlock()
	return m.scriptCancel != nil
}

func (m *Manager) scriptPaused() bool {
	// Phase-1: no pause channel implemented; always report not paused.
	return false
}
