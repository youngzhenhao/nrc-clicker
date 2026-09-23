// Package logging wires up the process-wide logrus logger.
//
// The format matches the Python reference (Clicker.py: setup_logger):
//
//	"2026-09-22 15:04:05 | INFO     | message body"
//
// Two sinks: stdout (when stdout is a terminal) and UTF-8 file at
// data/clicker.log. We keep the Python-flavored format so any existing log
// analysis scripts continue to work.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// Init configures the global logrus logger.
//
// dataDir is the directory under which clicker.log is written; it is created
// if missing. Calling Init more than once is safe — it replaces existing
// handlers.
func Init(dataDir string) error {
	if dataDir == "" {
		dataDir = "data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	logFile := filepath.Join(dataDir, "clicker.log")
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	logger := logrus.StandardLogger()
	logger.SetOutput(io.Discard) // disable default stderr writer; we add our own
	logger.SetFormatter(&formatter{})
	logger.SetLevel(logrus.InfoLevel)

	logger.AddHook(&fileHook{file: f})
	if isTerminal(os.Stdout) {
		logger.AddHook(&stdoutHook{})
	}

	logrus.Infof("logger initialised, file=%s", logFile)
	return nil
}

// fileHook writes log records to the clicker.log file.
type fileHook struct {
	file *os.File
}

func (h *fileHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *fileHook) Fire(e *logrus.Entry) error {
	line, err := formatEntry(e)
	if err != nil {
		return err
	}
	_, err = h.file.WriteString(line + "\n")
	return err
}

// stdoutHook mirrors the file hook to stdout when it's a real terminal.
type stdoutHook struct{}

func (h *stdoutHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *stdoutHook) Fire(e *logrus.Entry) error {
	line, err := formatEntry(e)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(line + "\n")
	return err
}

// formatter renders entries in the Python-shaped layout.
//
//	2026-09-22 15:04:05 | INFO     | message
//
// We use a tiny custom formatter (rather than logrus.TextFormatter) so the
// column widths line up with the Python output byte-for-byte.
type formatter struct{}

func formatEntry(e *logrus.Entry) (string, error) {
	ts := e.Time.Format("2006-01-02 15:04:05")
	level := fmt.Sprintf("%-8s", e.Level.String())
	msg := e.Message
	if len(e.Data) > 0 {
		msg += " " + fmt.Sprint(e.Data)
	}
	return ts + " | " + level + " | " + msg, nil
}

func (f *formatter) Format(e *logrus.Entry) ([]byte, error) {
	line, err := formatEntry(e)
	if err != nil {
		return nil, err
	}
	return []byte(line), nil
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// padClock returns a fixed-format clock suffix used in startup banners.
func padClock() string { return time.Now().Format("15:04:05") }

// Banner emits the "=====" startup banner used by Clicker.py:run().
func Banner(text string) {
	logrus.Info("==================================================")
	logrus.Info(text)
	logrus.Info("==================================================")
	_ = padClock() // kept so callers can extend banners without re-importing time
}
