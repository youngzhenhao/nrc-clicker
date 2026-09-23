package interception

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestToAbsoluteClamping verifies the pixel → 0..65535 scaling and the
// out-of-range clamping that the Python reference relies on.
//
// Python formula (InterceptionCore.py: _screen_to_interception):
//
//	max_x = max(SW - 1, 1)
//	ix = int(max(0, min(x, max_x)) * 65535 / max_x)
//
// Integer truncation in Go matches Python's `int(...)` behaviour.
func TestToAbsoluteClamping(t *testing.T) {
	c := &Core{screenW: 1920, screenH: 1080}
	cases := []struct {
		name          string
		x, y          int
		wantIX, wantIY int
	}{
		{"origin", 0, 0, 0, 0},
		{"middle", 960, 540, 32784, 32797},
		{"far corner", 1919, 1079, 65535, 65535},
		{"above max clamps", 5000, 5000, 65535, 65535},
		{"negative clamps", -10, -10, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ix, iy := c.toAbsolute(tc.x, tc.y)
			if ix != tc.wantIX || iy != tc.wantIY {
				t.Fatalf("toAbsolute(%d,%d) = (%d,%d), want (%d,%d)",
					tc.x, tc.y, ix, iy, tc.wantIX, tc.wantIY)
			}
		})
	}
}

// TestDllCandidatePathsRespectsArch ensures the dllCandidatePaths helper
// includes at least one entry for our detected architecture. We do not
// assert a specific path (it depends on the OS / launch cwd).
func TestDllCandidatePathsRespectsArch(t *testing.T) {
	got := dllCandidatePaths()
	if len(got) < 3 {
		t.Fatalf("expected several candidate paths, got %v", got)
	}
	wantSubstr := filepath.ToSlash(filepath.Join("library", arch(), "interception.dll"))
	found := false
	for _, p := range got {
		if strings.Contains(filepath.ToSlash(p), wantSubstr) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no candidate path contains %s; got %v", wantSubstr, got)
	}
}

// TestMouseStrokeLayout pins the C struct size — touching this test fails
// fast if someone reorders the fields by accident.
//
// On Win64 the C struct layout is:
//
//	ushort state     // 2  (offset 0)
//	ushort flags     // 2  (offset 2)
//	short  rolling   // 2  (offset 4)
//	                   // 2  padding for int alignment
//	int    x         // 4  (offset 8)
//	int    y         // 4  (offset 12)
//	uint   info  // 4  (offset 16)
//
//	                   → total 20 bytes
//
// This matches Python's ctypes layout for the same `_fields_` definition.
func TestMouseStrokeLayout(t *testing.T) {
	if got := sizeofMouseStroke(); got != 20 {
		t.Fatalf("MouseStroke size = %d, want 20 (matches INTERCEPTION_MOUSE_STROKE on Win64)", got)
	}
	if got := sizeofKeyStroke(); got != 8 {
		t.Fatalf("KeyStroke size = %d, want 8", got)
	}
}
