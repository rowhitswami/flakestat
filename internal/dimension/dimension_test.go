package dimension

import (
	"runtime"
	"strings"
	"testing"
)

func TestParseAcceptsRepeatedPairs(t *testing.T) {
	got, err := Parse([]string{"runtime.name=python", "runtime.version=3.13", "database=postgres-17"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got[RuntimeName] != "python" || got[RuntimeVersion] != "3.13" || got["database"] != "postgres-17" {
		t.Errorf("parsed = %v", got)
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	tests := []struct{ in, wants string }{
		{"python", "key=value"},
		{"bad key=v", `invalid dimension "bad key=v"`},
		{"=v", "key is empty"},
		{"key/with/slash=v", "may only contain"},
	}

	for _, tc := range tests {
		_, err := Parse([]string{tc.in})
		if err == nil {
			t.Errorf("Parse(%q) should have failed", tc.in)
			continue
		}
		if !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("Parse(%q) error = %v, want it to mention %q", tc.in, err, tc.wants)
		}
	}
}

// Values are opaque: a value may contain anything, including characters that
// would be rejected in a key.
func TestParseKeepsValuesOpaque(t *testing.T) {
	got, err := Parse([]string{"image=ghcr.io/acme/ci:v2", "note=windows-latest / 3.13"})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got["image"] != "ghcr.io/acme/ci:v2" {
		t.Errorf("value was altered: %q", got["image"])
	}
	if got["note"] != "windows-latest / 3.13" {
		t.Errorf("value was altered: %q", got["note"])
	}
}

// Precedence is what lets a user correct incomplete context: explicit flags
// beat recognized properties, which beat automatic detection.
func TestMergePrecedence(t *testing.T) {
	detected := Set{OS: "linux", Arch: "amd64"}
	props := Set{OS: "windows"}
	explicit := Set{OS: "darwin", "database": "postgres"}

	got := Merge(detected, props, explicit)

	if got[OS] != "darwin" {
		t.Errorf("os = %q, want the explicit value to win", got[OS])
	}
	if got[Arch] != "amd64" {
		t.Errorf("arch = %q, want the detected value to survive", got[Arch])
	}
	if got["database"] != "postgres" {
		t.Error("user dimensions should pass through")
	}
}

func TestMergeIgnoresEmptyValues(t *testing.T) {
	got := Merge(Set{OS: "linux"}, Set{OS: ""})
	if got[OS] != "linux" {
		t.Errorf("an empty later value should not erase an earlier one, got %q", got[OS])
	}
	if Merge(Set{}, nil) != nil {
		t.Error("merging nothing should yield nil, not an empty map")
	}
}

func TestHostReportsThisMachine(t *testing.T) {
	h := Host()
	if h[OS] != runtime.GOOS || h[Arch] != runtime.GOARCH {
		t.Errorf("Host = %v, want %s/%s", h, runtime.GOOS, runtime.GOARCH)
	}
}

// isolateCI clears every variable this package knows how to read, so that a
// test describes exactly one CI environment and nothing else.
//
// Without it these tests only pass on a laptop. Inside GitHub Actions
// GITHUB_ACTIONS=true is already set, so "detect gitlab" silently becomes
// "detect whichever provider happens to be listed first".
func isolateCI(t *testing.T) {
	t.Helper()
	for _, p := range providers {
		for _, name := range []string{p.detect, p.runID, p.jobID, p.worker, p.shard, p.attempt} {
			if name != "" {
				t.Setenv(name, "")
			}
		}
	}
}

// Only whitelisted variables are read. Scraping the process environment would
// eventually capture credentials.
func TestDetectCIReadsOnlyKnownVariables(t *testing.T) {
	isolateCI(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_RUN_ID", "21948210")
	t.Setenv("GITHUB_JOB", "test")
	t.Setenv("GITHUB_RUN_ATTEMPT", "2")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "hunter2")
	t.Setenv("SOME_INTERNAL_TOKEN", "sensitive")

	got := DetectCI()

	if got[CIProvider] != "github" {
		t.Errorf("provider = %q, want github", got[CIProvider])
	}
	if got[CIRunID] != "21948210" || got[CIJobID] != "test" {
		t.Errorf("run/job = %q/%q", got[CIRunID], got[CIJobID])
	}
	// Without the attempt, a re-run is indistinguishable from the same
	// artifact ingested twice.
	if got[CIAttempt] != "2" {
		t.Errorf("attempt = %q, want 2", got[CIAttempt])
	}
	for k, v := range got {
		if strings.Contains(v, "hunter2") || strings.Contains(v, "sensitive") {
			t.Errorf("dimension %q captured a secret: %q", k, v)
		}
	}
}

func TestDetectCIRecognizesShardedProviders(t *testing.T) {
	isolateCI(t)
	t.Setenv("GITLAB_CI", "true")
	t.Setenv("CI_PIPELINE_ID", "999")
	t.Setenv("CI_NODE_INDEX", "3")

	got := DetectCI()
	if got[CIProvider] != "gitlab" {
		t.Fatalf("provider = %q, want gitlab", got[CIProvider])
	}
	if got[CIShard] != "3" {
		t.Errorf("shard = %q, want 3", got[CIShard])
	}
}

// Nested or emulated environments can expose two providers at once. The first
// match wins, which is arbitrary but must at least be deliberate and stable.
func TestDetectCIPrefersTheFirstMatchingProvider(t *testing.T) {
	isolateCI(t)
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITLAB_CI", "true")

	if got := DetectCI(); got[CIProvider] != "github" {
		t.Errorf("provider = %q, want the first listed provider to win", got[CIProvider])
	}
}

func TestDetectCIRequiresTheExpectedValue(t *testing.T) {
	isolateCI(t)
	// GitHub sets GITHUB_ACTIONS=true; a stray empty or false value must not
	// be read as "running in GitHub Actions".
	t.Setenv("GITHUB_ACTIONS", "false")
	if got := DetectCI(); got != nil && got[CIProvider] == "github" {
		t.Errorf("GITHUB_ACTIONS=false should not detect github, got %v", got)
	}
}

func TestInCIIsFalseOutsideCI(t *testing.T) {
	isolateCI(t)
	if InCI() {
		t.Error("InCI should be false when no provider is detected")
	}
}

// Serializing every property would eventually store secrets, absolute paths
// and high-cardinality build ids, none of which group usefully.
func TestFromPropertiesIgnoresUnrecognizedNames(t *testing.T) {
	got := FromProperties(map[string]string{
		"os.name":               "Linux",
		"go.version":            "go1.25.4",
		"AWS_SECRET_ACCESS_KEY": "hunter2",
		"build.timestamp":       "2026-09-07T04:00:00Z",
		"java.class.path":       "/very/long/absolute/path.jar",
		"sun.jnu.encoding":      "UTF-8",
	})

	if got[OS] != "Linux" {
		t.Errorf("os = %q, want Linux", got[OS])
	}
	if got[RuntimeName] != "go" || got[RuntimeVersion] != "go1.25.4" {
		t.Errorf("runtime = %q %q", got[RuntimeName], got[RuntimeVersion])
	}
	if len(got) != 3 {
		t.Errorf("captured %d dimensions, want exactly os, runtime.name, runtime.version: %v", len(got), got)
	}
	for k, v := range got {
		if strings.Contains(v, "hunter2") {
			t.Errorf("dimension %q captured a secret", k)
		}
	}
}

// The runtime under test is never guessed: a version is only attributed when a
// property names the runtime it belongs to.
func TestFromPropertiesAttributesRuntimeExplicitly(t *testing.T) {
	got := FromProperties(map[string]string{"java.version": "21.0.2"})
	if got[RuntimeName] != "java" || got[RuntimeVersion] != "21.0.2" {
		t.Errorf("runtime = %q %q, want java 21.0.2", got[RuntimeName], got[RuntimeVersion])
	}

	// A bare version with no runtime named must not be attributed to anything.
	if got := FromProperties(map[string]string{"version": "3.13"}); got != nil {
		t.Errorf("an unattributed version should be ignored, got %v", got)
	}
}

func TestFromPropertiesEmpty(t *testing.T) {
	if got := FromProperties(nil); got != nil {
		t.Errorf("FromProperties(nil) = %v, want nil", got)
	}
	if got := FromProperties(map[string]string{"os.name": "  "}); got != nil {
		t.Errorf("blank values should be ignored, got %v", got)
	}
}

func TestKeysAreSorted(t *testing.T) {
	got := Set{"z": "1", "a": "2", "m": "3"}.Keys()
	if strings.Join(got, ",") != "a,m,z" {
		t.Errorf("Keys = %v, want sorted", got)
	}
}
