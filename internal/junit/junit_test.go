package junit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// corpusPath resolves a file in the shared dialect corpus.
func corpusPath(name string) string {
	return filepath.Join("..", "..", "testdata", "junit", name)
}

// find returns the case with the given name, failing the test if absent.
func find(t *testing.T, rep *Report, name string) Case {
	t.Helper()
	for _, c := range rep.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("case %q not found in report (%d cases)", name, len(rep.Cases))
	return Case{}
}

func TestParseDialects(t *testing.T) {
	tests := []struct {
		file      string
		wantCases int
		wantPass  int
		wantFail  int
		wantError int
		wantSkip  int
	}{
		{"pytest.xml", 4, 1, 1, 1, 1},
		{"jest.xml", 3, 2, 1, 0, 0},
		{"gotestsum.xml", 3, 2, 1, 0, 0},
		{"surefire-flaky.xml", 3, 2, 1, 0, 0},
		{"phpunit-nested.xml", 3, 2, 1, 0, 0},
		{"rspec.xml", 2, 1, 1, 0, 0},
		{"nextest.xml", 2, 2, 0, 0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			rep, err := ParseFile(corpusPath(tc.file))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if len(rep.Cases) != tc.wantCases {
				t.Errorf("cases = %d, want %d", len(rep.Cases), tc.wantCases)
			}

			var pass, fail, errs, skip int
			for _, c := range rep.Cases {
				switch c.Status {
				case StatusPass:
					pass++
				case StatusFail:
					fail++
				case StatusError:
					errs++
				case StatusSkip:
					skip++
				default:
					t.Errorf("case %q has empty status", c.Name)
				}
			}
			if pass != tc.wantPass || fail != tc.wantFail || errs != tc.wantError || skip != tc.wantSkip {
				t.Errorf("pass/fail/error/skip = %d/%d/%d/%d, want %d/%d/%d/%d",
					pass, fail, errs, skip, tc.wantPass, tc.wantFail, tc.wantError, tc.wantSkip)
			}

			// Every case must produce a usable identity.
			for _, c := range rep.Cases {
				if c.Name == "" {
					t.Errorf("case with empty name: %+v", c)
				}
				if len(c.ID()) != 16 {
					t.Errorf("ID() = %q, want 16 hex chars", c.ID())
				}
			}
		})
	}
}

// Root element may be <testsuites> or a bare <testsuite>; both must work.
func TestBareTestsuiteRoot(t *testing.T) {
	rep, err := ParseFile(corpusPath("rspec.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(rep.Cases) != 2 {
		t.Fatalf("cases = %d, want 2", len(rep.Cases))
	}
	if rep.Name != "rspec" {
		t.Errorf("Name = %q, want %q", rep.Name, "rspec")
	}
}

// PHPUnit nests <testsuite> inside <testsuite>; cases must be flattened and
// inherit the nearest named suite.
func TestNestedSuites(t *testing.T) {
	rep, err := ParseFile(corpusPath("phpunit-nested.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	got := find(t, rep, "testInvoiceTotal")
	if got.Suite != "BillingTest" {
		t.Errorf("Suite = %q, want %q", got.Suite, "BillingTest")
	}
	if got.Line != 9 {
		t.Errorf("Line = %d, want 9", got.Line)
	}
	if got.File != "/app/tests/BillingTest.php" {
		t.Errorf("File = %q", got.File)
	}
}

// Surefire reports flakiness directly: <flakyFailure> means failed-then-passed
// within one run, while <rerunFailure> means it failed every attempt.
func TestSurefireRerunMarkers(t *testing.T) {
	rep, err := ParseFile(corpusPath("surefire-flaky.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	flaky := find(t, rep, "testConcurrentCheckout")
	if !flaky.KnownFlaky {
		t.Error("testConcurrentCheckout: KnownFlaky = false, want true")
	}
	if flaky.Status != StatusPass {
		t.Errorf("testConcurrentCheckout: Status = %q, want pass (it passed on rerun)", flaky.Status)
	}
	if flaky.Reruns != 1 {
		t.Errorf("testConcurrentCheckout: Reruns = %d, want 1", flaky.Reruns)
	}

	hard := find(t, rep, "testPaymentTimeout")
	if hard.Status != StatusFail {
		t.Errorf("testPaymentTimeout: Status = %q, want fail", hard.Status)
	}
	if hard.KnownFlaky {
		t.Error("testPaymentTimeout: KnownFlaky = true, want false (it never passed)")
	}

	clean := find(t, rep, "testCreateOrder")
	if clean.KnownFlaky {
		t.Error("testCreateOrder: KnownFlaky = true, want false")
	}
}

// Surefire under a comma-decimal locale emits time="4,521" meaning 4.521s.
// encoding/xml's native float binding would reject the whole document.
func TestLocaleFormattedDurations(t *testing.T) {
	rep, err := ParseFile(corpusPath("surefire-flaky.xml"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	got := find(t, rep, "testCreateOrder")
	if want := 412 * time.Millisecond; got.Duration != want {
		t.Errorf("Duration = %v, want %v", got.Duration, want)
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"1", time.Second},
		{"0.5", 500 * time.Millisecond},
		{"0,001", time.Millisecond},             // comma-decimal locale
		{"1,234.5", 1234500 * time.Millisecond}, // comma as group separator
		{"1.5e-03", 1500 * time.Microsecond},
		{"garbage", 0},
		{"-1", 0},
		{"NaN", 0},
		{"  0.25  ", 250 * time.Millisecond},
	}

	for _, tc := range tests {
		if got := parseDuration(tc.in); got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Test output routinely carries ANSI escapes and raw binary. Those bytes are
// illegal in XML 1.0 and would otherwise fail the entire document.
func TestSanitizesIllegalXMLBytes(t *testing.T) {
	doc := "<testsuite name=\"s\" tests=\"1\">" +
		"<testcase classname=\"C\" name=\"t\" time=\"0.1\">" +
		"<failure message=\"boom\">saw \x0c\x00\x1b[31mred\x1b[0m output</failure>" +
		"</testcase></testsuite>"

	rep, err := ParseBytes([]byte(doc))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(rep.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(rep.Cases))
	}
	if rep.Cases[0].Status != StatusFail {
		t.Errorf("Status = %q, want fail", rep.Cases[0].Status)
	}
}

func TestBOMIsStripped(t *testing.T) {
	doc := "\xEF\xBB\xBF<testsuite name=\"s\"><testcase classname=\"C\" name=\"t\"/></testsuite>"
	rep, err := ParseBytes([]byte(doc))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(rep.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(rep.Cases))
	}
}

// line="" appears in real reports and must not fail the document.
func TestEmptyNumericAttributes(t *testing.T) {
	doc := `<testsuite name="s"><testcase classname="C" name="t" line="" time=""/></testsuite>`
	rep, err := ParseBytes([]byte(doc))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if rep.Cases[0].Line != 0 || rep.Cases[0].Duration != 0 {
		t.Errorf("got Line=%d Duration=%v, want zero values", rep.Cases[0].Line, rep.Cases[0].Duration)
	}
}

func TestMissingRootIsAnError(t *testing.T) {
	if _, err := ParseBytes([]byte(`<?xml version="1.0"?><result><ok/></result>`)); err == nil {
		t.Fatal("expected an error for a document with no testsuite root")
	}
}

// A case carrying both <failure> and <skipped> is a failure, not a skip.
func TestFailureOutranksSkipped(t *testing.T) {
	doc := `<testsuite name="s"><testcase classname="C" name="t">
		<skipped message="retried"/><failure message="boom"/>
	</testcase></testsuite>`

	rep, err := ParseBytes([]byte(doc))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if rep.Cases[0].Status != StatusFail {
		t.Errorf("Status = %q, want fail", rep.Cases[0].Status)
	}
}

func TestIDIsStableAndDistinct(t *testing.T) {
	a := Case{Suite: "s", Class: "C", Name: "test_x"}
	b := Case{Suite: "s", Class: "C", Name: "test_x"}
	c := Case{Suite: "s", Class: "C", Name: "test_y"}

	if a.ID() != b.ID() {
		t.Error("identical cases produced different IDs")
	}
	if a.ID() == c.ID() {
		t.Error("different cases produced the same ID")
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"test_login", "test_login"},
		{"test_charge[stripe-usd]", "test_charge"},
		{"test_matrix[a][b]", "test_matrix"},
		{"testAdd(1, 2)", "testAdd"},
		{"test_x [gbp]", "test_x"},
		{"[only-params]", "[only-params]"}, // never normalize a name away entirely
	}

	for _, tc := range tests {
		if got := NormalizeName(tc.in); got != tc.want {
			t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Parameterized siblings share a NormalizedID but keep distinct exact IDs.
func TestNormalizedIDGroupsParameterizedCases(t *testing.T) {
	a := Case{Suite: "s", Class: "C", Name: "test_charge[usd]"}
	b := Case{Suite: "s", Class: "C", Name: "test_charge[eur]"}

	if a.ID() == b.ID() {
		t.Error("parameterized cases should have distinct exact IDs")
	}
	if a.NormalizedID() != b.NormalizedID() {
		t.Error("parameterized cases should share a normalized ID")
	}
}

func TestParseGlobMergesAndReportsErrors(t *testing.T) {
	rep, errs := ParseGlob(corpusPath("*.xml"))
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(rep.Cases) != 20 {
		t.Errorf("merged cases = %d, want 20", len(rep.Cases))
	}
}

func TestDisplayName(t *testing.T) {
	// jest duplicates classname into name. Qualify with the suite (the file,
	// which is useful context) rather than repeating the name twice.
	dup := Case{Suite: "src/auth.test.js", Class: "Auth logs in", Name: "Auth logs in"}
	if got := dup.DisplayName(); got != "src/auth.test.js::Auth logs in" {
		t.Errorf("DisplayName() = %q, want %q", got, "src/auth.test.js::Auth logs in")
	}
	if got := dup.DisplayName(); strings.Count(got, "Auth logs in") != 1 {
		t.Errorf("DisplayName() = %q, repeats the test name", got)
	}

	// When only the name is known, render it alone rather than "t::t".
	bare := Case{Suite: "t", Class: "t", Name: "t"}
	if got := bare.DisplayName(); got != "t" {
		t.Errorf("DisplayName() = %q, want %q", got, "t")
	}

	normal := Case{Suite: "pytest", Class: "tests.test_auth", Name: "test_login"}
	if got := normal.DisplayName(); !strings.Contains(got, "::") {
		t.Errorf("DisplayName() = %q, want a qualified name", got)
	}
}
