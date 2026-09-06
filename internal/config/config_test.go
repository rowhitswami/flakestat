package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"pytest -q", []string{"pytest", "-q"}},
		{"  pytest   -q  ", []string{"pytest", "-q"}},
		{"", nil},
		{`pytest --junitxml="reports/a b.xml"`, []string{"pytest", "--junitxml=reports/a b.xml"}},
		{`pytest --junitxml='reports/a b.xml'`, []string{"pytest", "--junitxml=reports/a b.xml"}},
		{`echo "" x`, []string{"echo", "", "x"}},
		{"a\tb\nc", []string{"a", "b", "c"}},
	}

	for _, tc := range tests {
		got, err := SplitCommand(tc.in)
		if err != nil {
			t.Errorf("SplitCommand(%q): %v", tc.in, err)
			continue
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("SplitCommand(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	if _, err := SplitCommand(`pytest --x="unterminated`); err == nil {
		t.Error("expected an error for an unterminated quote")
	}
}

func TestCommandAcceptsBothForms(t *testing.T) {
	dir := t.TempDir()

	asList := filepath.Join(dir, "list.json")
	os.WriteFile(asList, []byte(`{"version":1,"command":["pytest","-q"]}`), 0o644)

	asString := filepath.Join(dir, "string.json")
	os.WriteFile(asString, []byte(`{"version":1,"command":"pytest -q"}`), 0o644)

	for _, p := range []string{asList, asString} {
		f, err := Load(p)
		if err != nil {
			t.Fatalf("Load(%s): %v", p, err)
		}
		if strings.Join(f.Command, "|") != "pytest|-q" {
			t.Errorf("%s: command = %v, want [pytest -q]", filepath.Base(p), f.Command)
		}
	}
}

// Running without a config is a supported mode, not an error.
func TestLoadMissingFileIsEmpty(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Command) != 0 || f.Path() != "" {
		t.Errorf("expected an empty config, got %+v", f)
	}
}

func TestLoadRejectsFutureVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(p, []byte(`{"version":99}`), 0o644)

	_, err := Load(p)
	if err == nil {
		t.Fatal("expected an error for an unsupported version")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("error should name the version, got: %v", err)
	}
}

func TestLoadReportsBadJSONWithPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	os.WriteFile(p, []byte(`{not json`), 0o644)

	_, err := Load(p)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if !strings.Contains(err.Error(), FileName) {
		t.Errorf("error should name the file, got: %v", err)
	}
}

// The tool should work from any subdirectory of a repo.
func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(root, FileName)
	os.WriteFile(want, []byte(`{"version":1}`), 0o644)

	got := Find(nested)
	if got != want {
		t.Errorf("Find(%s) = %q, want %q", nested, got, want)
	}
}

func TestFindReturnsEmptyWhenAbsent(t *testing.T) {
	if got := Find(t.TempDir()); got != "" {
		t.Errorf("Find = %q, want empty", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)

	in := &File{Command: []string{"pytest", "-q"}, JUnit: "r/{run}.xml", Runs: 25}
	in.Scoring.DefaultBranch = "main"
	if err := in.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.Version != 1 {
		t.Errorf("Version = %d, want 1", out.Version)
	}
	if out.Runs != 25 || out.JUnit != "r/{run}.xml" {
		t.Errorf("round trip lost data: %+v", out)
	}
	if out.Scoring.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", out.Scoring.DefaultBranch)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"go", map[string]string{"go.mod": "module x"}, "go"},
		{"pytest via conftest", map[string]string{"conftest.py": "", "setup.py": ""}, "pytest"},
		{"pytest via pyproject", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]"}, "pytest"},
		{"cargo", map[string]string{"Cargo.toml": "[package]"}, "cargo-nextest"},
		{"maven", map[string]string{"pom.xml": "<project/>"}, "maven"},
		{"vitest via config", map[string]string{"vitest.config.ts": ""}, "vitest"},
		{"jest via package.json", map[string]string{
			"package.json": `{"devDependencies":{"jest":"^29"}}`}, "jest"},
		{"vitest via package.json", map[string]string{
			"package.json": `{"devDependencies":{"vitest":"^1"}}`}, "vitest"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			found := Detect(dir)
			if len(found) == 0 {
				t.Fatalf("nothing detected for %v", tc.files)
			}
			if found[0].Name != tc.want {
				t.Errorf("detected %q, want %q", found[0].Name, tc.want)
			}
			if len(found[0].Args) == 0 {
				t.Error("detected project has no command")
			}
		})
	}
}

// pyproject.toml alone (a packaging file) must not be read as a pytest setup.
func TestDetectDoesNotGuessPytestFromPackagingAlone(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname='x'\n"), 0o644)

	for _, p := range Detect(dir) {
		if p.Name == "pytest" {
			t.Error("pyproject.toml with no pytest signal should not detect pytest")
		}
	}
}

func TestDetectEmptyDirectory(t *testing.T) {
	if got := Detect(t.TempDir()); len(got) != 0 {
		t.Errorf("Detect = %v, want nothing", got)
	}
}

// A polyglot repo legitimately matches more than one setup.
func TestDetectPolyglot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0o644)
	os.WriteFile(filepath.Join(dir, "conftest.py"), []byte(""), 0o644)

	found := Detect(dir)
	if len(found) < 2 {
		t.Fatalf("detected %d setups, want at least 2", len(found))
	}
}

// Every detector must reference {junit} in its command or point at a fixed
// report path, otherwise flakestat has no report to read.
func TestEveryDetectorProducesAReport(t *testing.T) {
	dirs := map[string]map[string]string{
		"go":      {"go.mod": "module x"},
		"pytest":  {"conftest.py": ""},
		"cargo":   {"Cargo.toml": "[package]"},
		"maven":   {"pom.xml": "<project/>"},
		"gradle":  {"build.gradle": ""},
		"phpunit": {"phpunit.xml": ""},
		"vitest":  {"vitest.config.ts": ""},
		"jest":    {"jest.config.js": ""},
	}

	for name, files := range dirs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for f, body := range files {
				os.WriteFile(filepath.Join(dir, f), []byte(body), 0o644)
			}

			found := Detect(dir)
			if len(found) == 0 {
				t.Fatalf("%s not detected", name)
			}
			p := found[0]
			if p.JUnit == "" {
				t.Errorf("%s has no report path", p.Name)
			}

			usesPlaceholder := strings.Contains(strings.Join(p.Args, " "), "{junit}")
			fixedPath := !strings.Contains(p.JUnit, "{run}")
			// Either the command is told where to write, or the framework
			// writes to a path we already know.
			if !usesPlaceholder && !fixedPath && p.Notes == "" {
				t.Errorf("%s neither uses {junit} nor documents its fixed path", p.Name)
			}
		})
	}
}

// Repos predating Go modules still have Go tests; go.mod alone is too strict.
func TestDetectGoWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "cache.go"), []byte("package cache"), 0o644)
	os.WriteFile(filepath.Join(dir, "cache_test.go"), []byte("package cache"), 0o644)

	found := Detect(dir)
	if len(found) == 0 || found[0].Name != "go" {
		t.Fatalf("pre-modules Go repo not detected, got %v", found)
	}
}

// A directory of plain .go files with no tests is not a test setup.
func TestDetectIgnoresGoSourceWithoutTests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0o644)

	for _, p := range Detect(dir) {
		if p.Name == "go" {
			t.Error("Go source without _test.go files should not be detected")
		}
	}
}
