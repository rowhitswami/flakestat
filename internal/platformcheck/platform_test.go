//go:build ciplatform

package platformcheck

import (
	"runtime"
	"testing"
)

// TestPlatformDifferential fails on Windows and passes everywhere else, always.
//
// This proves the pipeline carries execution context correctly:
//
//	matrix job -> JUnit -> ingest with dimensions -> artifact -> merge
//	  -> association -> explain
//
// It is deterministic on purpose. Introducing real randomness into this
// repository's CI would contaminate the signal flakestat is collecting about
// itself, and a tool that deliberately destabilises its own test suite is not
// one anybody should trust.
//
// The expected conclusion is an association with os=windows and, because the
// behaviour never varies within a platform, a stable classification. Those two
// together describe a deterministic platform difference rather than flakiness,
// which is exactly the distinction the two-branch model exists to preserve.
func TestPlatformDifferential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatalf("deliberate platform-differential failure on %s (see package docs)", runtime.GOOS)
	}
}

// TestPlatformControl passes everywhere, so the differential result cannot be
// confused with the whole package failing.
func TestPlatformControl(t *testing.T) {}
