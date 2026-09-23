package interception

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// upstreamReleaseURL is the GitHub release zip of the official Interception
// library. We only extract library/x64/interception.dll from it.
//
// The exact version is pinned to keep the bootstrap deterministic. Bump
// when shipping a new release that requires a newer DLL.
const upstreamReleaseURL = "https://github.com/oblitum/Interception/releases/download/v1.0.1/interception.zip"

// dllInstallPath is where bootstrap ensures interception.dll lives before
// the wrapper tries to load it. Relative paths resolve against the
// executable directory at runtime.
func dllInstallPath() string {
	return filepath.Join("third", "Interception", "library", arch(), "interception.dll")
}

// siblingRepoDLLPath is the path the existing Python checkout uses. We
// prefer copying from here so the dev experience matches "clone & run"
// without any network access.
func siblingRepoDLLPath() (string, bool) {
	// Walk upward from CWD looking for tmp/RocoKingdom-Clicker/third/...
	// so this works regardless of where go run / the binary was launched.
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for i := 0; i < 6; i++ {
		p := filepath.Join(dir, "tmp", "RocoKingdom-Clicker", "third", "Interception", "library", arch(), "interception.dll")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// EnsureDLL guarantees interception.dll is present at dllInstallPath().
//
// The strategy is:
//  1. If the install path already has a DLL, do nothing.
//  2. If a sibling checkout has it, copy it (dev-friendly).
//  3. Otherwise, attempt the upstream download.
//
// The function returns nil on success and the resolved DLL path in
// ResolvedPath so the caller can log it. On any failure it returns a
// descriptive error and the caller is expected to surface a modal dialog.
func EnsureDLL() (resolved string, err error) {
	dst := dllInstallPath()
	if _, statErr := os.Stat(dst); statErr == nil {
		return dst, nil
	}

	// Sibling copy first — works offline and keeps LGPL attribution easy.
	if src, ok := siblingRepoDLLPath(); ok {
		if cerr := copyFile(src, dst); cerr == nil {
			return dst, nil
		}
	}

	// Upstream download as last resort.
	if derr := downloadUpstream(dst); derr != nil {
		return "", fmt.Errorf("interception.dll not available: sibling copy unavailable and upstream download failed (%w). Install manually from %s", derr, upstreamReleaseURL)
	}
	return dst, nil
}

// copyFile copies src → dst, creating dst's parent directory as needed.
// Does not overwrite an existing destination.
func copyFile(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// downloadUpstream fetches the GitHub release zip and extracts the DLL
// matching our architecture.
//
// Network failures (timeout, non-2xx, missing archive entry) are returned
// as errors so the caller can present a "download manually" prompt.
func downloadUpstream(dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(upstreamReleaseURL)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Read whole body — the zip is small (kB scale).
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}

	a := arch()
	wantName := filepath.Join("library", a, "interception.dll")
	for _, f := range zr.File {
		// ZIP forward slashes regardless of host OS.
		clean := filepath.ToSlash(f.Name)
		if clean != wantName {
			continue
		}
		if err := extractZipEntry(f, dst); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
		return nil
	}
	return errors.New("archive did not contain " + wantName)
}

func extractZipEntry(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}
