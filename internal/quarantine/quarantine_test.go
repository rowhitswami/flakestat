package quarantine

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/rowhitswami/flakestat/internal/score"
)

func results() []score.Result {
	return []score.Result{
		{TestID: "1", Suite: "pytest", Class: "tests.test_auth", Name: "test_login",
			Verdict: score.ClassFlaky, Score: 0.62, Runs: 20, Passes: 14, Fails: 6},
		{TestID: "2", Suite: "pytest", Class: "tests.test_billing", Name: "test_charge",
			Verdict: score.ClassStable, Score: 0.0, Runs: 20, Passes: 20},
		{TestID: "3", Suite: "pytest", Class: "tests.test_api", Name: "test_broken",
			Verdict: score.ClassConsistentlyFail, Runs: 20, Fails: 20},
		{TestID: "4", Suite: "pytest", Class: "tests.test_api", Name: "test_slow",
			Verdict: score.ClassFlaky, Score: 0.81, Runs: 20, Passes: 10, Fails: 10},
	}
}

func TestSelectTakesOnlyFlakyByDefault(t *testing.T) {
	got := Select(results(), Options{})

	if len(got) != 2 {
		t.Fatalf("selected %d, want 2", len(got))
	}
	// Worst first.
	if got[0].Name != "test_slow" {
		t.Errorf("first = %q, want test_slow (higher score)", got[0].Name)
	}
	for _, r := range got {
		if r.Verdict == score.ClassConsistentlyFail {
			t.Error("a consistently failing test must not be quarantined by default")
		}
	}
}

// Skipping a broken test hides a real defect, so it takes an explicit opt-in.
func TestSelectIncludeBroken(t *testing.T) {
	got := Select(results(), Options{IncludeBroken: true})
	if len(got) != 3 {
		t.Fatalf("selected %d, want 3", len(got))
	}

	var found bool
	for _, r := range got {
		if r.Name == "test_broken" {
			found = true
		}
	}
	if !found {
		t.Error("--include-broken should have included test_broken")
	}
}

func TestPytestNodeID(t *testing.T) {
	tests := []struct {
		name string
		in   score.Result
		want string
	}{
		{
			name: "module only",
			in:   score.Result{Class: "tests.test_auth", Name: "test_login"},
			want: "tests/test_auth.py::test_login",
		},
		{
			name: "module with class",
			in:   score.Result{Class: "tests.test_auth.TestAuth", Name: "test_login"},
			want: "tests/test_auth.py::TestAuth::test_login",
		},
		{
			name: "parameterized",
			in:   score.Result{Class: "tests.test_billing", Name: "test_charge[stripe-usd]"},
			want: "tests/test_billing.py::test_charge[stripe-usd]",
		},
		{
			name: "reported file wins over derived path",
			in:   score.Result{Class: "tests.test_auth", File: "src/tests/test_auth.py", Name: "test_login"},
			want: "src/tests/test_auth.py::test_login",
		},
		{
			name: "reported file with class",
			in:   score.Result{Class: "AuthTest", File: "tests/AuthTest.php", Name: "testLogin"},
			want: "tests/AuthTest.php::AuthTest::testLogin",
		},
		{
			name: "no class at all",
			in:   score.Result{Name: "test_bare"},
			want: "test_bare",
		},
		{
			name: "nested classes",
			in:   score.Result{Class: "tests.mod.Outer.Inner", Name: "test_x"},
			want: "tests/mod.py::Outer::Inner::test_x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PytestNodeID(tc.in); got != tc.want {
				t.Errorf("PytestNodeID() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderPytest(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, results(), Options{Format: FormatPytest}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "tests/test_api.py::test_slow") {
		t.Errorf("missing node ID for test_slow:\n%s", out)
	}
	if strings.Contains(out, "test_broken") {
		t.Error("broken test should not appear by default")
	}
	// The usage hint is the difference between a list and something actionable.
	if !strings.Contains(out, "--deselect") {
		t.Error("output should show how to use it with pytest")
	}
}

func TestRenderGoProducesValidSkipRegex(t *testing.T) {
	in := []score.Result{
		{Name: "TestConcurrentWrite", Verdict: score.ClassFlaky, Score: 0.5},
		{Name: "TestOuter/sub_case", Verdict: score.ClassFlaky, Score: 0.4},
	}

	var buf bytes.Buffer
	if err := Render(&buf, in, Options{Format: FormatGo}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var pattern string
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			pattern = line
			break
		}
	}
	if pattern == "" {
		t.Fatalf("no regex emitted:\n%s", buf.String())
	}

	re, err := regexpCompile(pattern)
	if err != nil {
		t.Fatalf("emitted an invalid regex %q: %v", pattern, err)
	}
	if !re.MatchString("TestConcurrentWrite") {
		t.Errorf("regex %q should match TestConcurrentWrite", pattern)
	}
	if !re.MatchString("TestOuter/sub_case") {
		t.Errorf("regex %q should match the subtest", pattern)
	}
	if re.MatchString("TestSomethingElse") {
		t.Errorf("regex %q should not match unrelated tests", pattern)
	}
}

// Regex metacharacters in test names must not corrupt the pattern.
func TestRenderGoEscapesMetacharacters(t *testing.T) {
	in := []score.Result{
		{Name: "TestRegex(a|b).*", Verdict: score.ClassFlaky, Score: 0.5},
	}

	var buf bytes.Buffer
	if err := Render(&buf, in, Options{Format: FormatGo}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var pattern string
	for _, line := range strings.Split(buf.String(), "\n") {
		if !strings.HasPrefix(line, "#") && strings.TrimSpace(line) != "" {
			pattern = line
			break
		}
	}

	re, err := regexpCompile(pattern)
	if err != nil {
		t.Fatalf("invalid regex %q: %v", pattern, err)
	}
	if !re.MatchString("TestRegex(a|b).*") {
		t.Errorf("regex %q should match the literal name", pattern)
	}
	if re.MatchString("TestRegexa") {
		t.Errorf("regex %q leaked an unescaped alternation", pattern)
	}
}

func TestRenderJestIsHonestAboutFileGranularity(t *testing.T) {
	in := []score.Result{
		{Suite: "src/auth.test.js", Name: "signs a user in", Verdict: score.ClassFlaky, Score: 0.5},
	}

	var buf bytes.Buffer
	if err := Render(&buf, in, Options{Format: FormatJest}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	// jest cannot deselect individual tests; the output must say so.
	if !strings.Contains(out, "cannot skip individual tests") {
		t.Errorf("jest output should state the file-level limitation:\n%s", out)
	}
	if !strings.Contains(out, "testPathIgnorePatterns") {
		t.Error("missing testPathIgnorePatterns")
	}

	// The JSON block follows the comment header. Parse it and check the
	// pattern jest would actually receive, rather than the escaped literal.
	blob := out[strings.Index(out, "{"):]
	var cfg struct {
		Patterns []string `json:"testPathIgnorePatterns"`
	}
	if err := json.Unmarshal([]byte(blob), &cfg); err != nil {
		t.Fatalf("emitted invalid JSON: %v\n%s", err, blob)
	}
	if len(cfg.Patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(cfg.Patterns))
	}

	re, err := regexpCompile(cfg.Patterns[0])
	if err != nil {
		t.Fatalf("pattern %q is not a valid regex: %v", cfg.Patterns[0], err)
	}
	if !re.MatchString("src/auth.test.js") {
		t.Errorf("pattern %q should match the file", cfg.Patterns[0])
	}
	// An unescaped dot would match this too.
	if re.MatchString("src/authXtest.js") {
		t.Errorf("pattern %q left the dots unescaped", cfg.Patterns[0])
	}
}

func TestRenderJSONIsValid(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, results(), Options{Format: FormatJSON}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	var got struct {
		Version     int `json:"version"`
		Quarantined []struct {
			Name    string  `json:"name"`
			Verdict string  `json:"verdict"`
			Score   float64 `json:"score"`
		} `json:"quarantined"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Version != 1 {
		t.Errorf("version = %d, want 1", got.Version)
	}
	if len(got.Quarantined) != 2 {
		t.Errorf("quarantined = %d, want 2", len(got.Quarantined))
	}
}

func TestRenderYAMLQuotesStrings(t *testing.T) {
	in := []score.Result{
		{TestID: "x", Class: "C", Name: `test: with "quotes" and: colons`,
			Verdict: score.ClassFlaky, Score: 0.5, Runs: 10},
	}

	var buf bytes.Buffer
	if err := Render(&buf, in, Options{Format: FormatYAML}); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, `\"quotes\"`) {
		t.Errorf("quotes were not escaped:\n%s", out)
	}
	if !strings.Contains(out, "version: 1") {
		t.Error("missing version")
	}
}

func TestEmptySelectionStillRenders(t *testing.T) {
	stable := []score.Result{
		{Name: "test_ok", Verdict: score.ClassStable, Runs: 20, Passes: 20},
	}

	for _, f := range Formats() {
		t.Run(f, func(t *testing.T) {
			var buf bytes.Buffer
			if err := Render(&buf, stable, Options{Format: Format(f)}); err != nil {
				t.Fatalf("Render: %v", err)
			}
			if buf.Len() == 0 {
				t.Error("empty selection should still produce output")
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	if _, err := ParseFormat("pytest"); err != nil {
		t.Errorf("pytest should be valid: %v", err)
	}
	if _, err := ParseFormat("  PyTest "); err != nil {
		t.Errorf("format parsing should be case and space insensitive: %v", err)
	}
	if _, err := ParseFormat("nonsense"); err == nil {
		t.Error("expected an error for an unknown format")
	}
}

// regexpCompile is a tiny indirection so the test reads as "is this a valid
// regex" rather than importing regexp for a single call.
func regexpCompile(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}
