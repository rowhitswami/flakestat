package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Project is a recognized test setup and the command that makes it emit the
// JUnit XML flakestat reads.
type Project struct {
	Name  string   // human label, e.g. "pytest"
	Args  []string // test command; {junit} is replaced with the report path
	JUnit string   // default report path, with {run} for parallel safety
	Setup string   // extra install step the user must run first, if any
	Notes string   // anything worth knowing before the first run

	// Filter is how this runner selects a single test. Hunting one suspect
	// test is far cheaper than hunting a whole suite, and it is the question
	// people usually actually have.
	Filter string
}

const defaultReport = ".flakestat/reports/junit-{run}.xml"

// Detect inspects dir and returns the matching project setups, most likely
// first. A polyglot repo legitimately matches more than one.
func Detect(dir string) []Project {
	if dir == "" {
		dir = "."
	}

	var found []Project
	for _, d := range detectors {
		if p, ok := d(dir); ok {
			found = append(found, p)
		}
	}
	return found
}

type detector func(dir string) (Project, bool)

// Ordered by how unambiguous the signal is.
var detectors = []detector{
	detectGo,
	detectPytest,
	detectVitest,
	detectJest,
	detectCargo,
	detectMaven,
	detectGradle,
	detectPHPUnit,
	detectRSpec,
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func anyExists(dir string, names ...string) bool {
	for _, n := range names {
		if exists(dir, n) {
			return true
		}
	}
	return false
}

// hasGoTests reports whether dir contains Go test files at its top level.
func hasGoTests(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

// readFile returns file contents, empty on any error.
func readFile(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return string(b)
}

// packageJSONHasDep reports whether package.json depends on name.
func packageJSONHasDep(dir, name string) bool {
	raw := readFile(dir, "package.json")
	if raw == "" {
		return false
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		return false
	}
	_, a := pkg.Dependencies[name]
	_, b := pkg.DevDependencies[name]
	return a || b
}

func detectGo(dir string) (Project, bool) {
	// go.mod is the usual signal, but repos predating modules still have Go
	// tests; patrickmn/go-cache is one. Fall back to looking for _test.go.
	if !exists(dir, "go.mod") && !hasGoTests(dir) {
		return Project{}, false
	}
	return Project{
		Name:   "go",
		Filter: "-run '^TestName$'",
		// go test has no JUnit output of its own; gotestsum is the standard wrapper.
		Args:  []string{"gotestsum", "--junitfile", "{junit}", "--format", "none", "--", "-count=1", "./..."},
		JUnit: defaultReport,
		Setup: "go install gotest.tools/gotestsum@latest",
		Notes: "-count=1 disables Go's test cache, which would otherwise replay a cached result instead of running the test again.",
	}, true
}

func detectPytest(dir string) (Project, bool) {
	if !anyExists(dir, "pytest.ini", "tox.ini", "setup.cfg", "pyproject.toml", "setup.py", "conftest.py") {
		return Project{}, false
	}
	// pyproject.toml alone is not proof of pytest; look for a real signal.
	if !anyExists(dir, "pytest.ini", "conftest.py", "tests", "test") &&
		!strings.Contains(readFile(dir, "pyproject.toml"), "pytest") &&
		!strings.Contains(readFile(dir, "tox.ini"), "pytest") &&
		!strings.Contains(readFile(dir, "setup.cfg"), "pytest") {
		return Project{}, false
	}
	return Project{
		Name:   "pytest",
		Filter: "-k test_name",
		Args:   []string{"pytest", "-q", "--junitxml={junit}"},
		JUnit:  defaultReport,
	}, true
}

func detectVitest(dir string) (Project, bool) {
	if !packageJSONHasDep(dir, "vitest") &&
		!anyExists(dir, "vitest.config.ts", "vitest.config.js", "vitest.config.mjs") {
		return Project{}, false
	}
	return Project{
		Name:   "vitest",
		Filter: "-t 'test name'",
		Args:   []string{"npx", "vitest", "run", "--reporter=junit", "--outputFile={junit}"},
		JUnit:  defaultReport,
	}, true
}

func detectJest(dir string) (Project, bool) {
	if !packageJSONHasDep(dir, "jest") && !anyExists(dir, "jest.config.js", "jest.config.ts", "jest.config.mjs") {
		return Project{}, false
	}
	return Project{
		Name:   "jest",
		Filter: "-t 'test name'",
		Args:   []string{"npx", "jest", "--reporters=default", "--reporters=jest-junit"},
		JUnit:  defaultReport,
		Setup:  "npm install --save-dev jest-junit",
		// jest-junit is configured by environment, not by a CLI flag.
		Notes: "jest-junit takes its output path from JEST_JUNIT_OUTPUT_NAME; flakestat sets it for each run.",
	}, true
}

func detectCargo(dir string) (Project, bool) {
	if !exists(dir, "Cargo.toml") {
		return Project{}, false
	}
	return Project{
		Name:   "cargo-nextest",
		Filter: "-E 'test(test_name)'",
		Args:   []string{"cargo", "nextest", "run", "--profile", "ci"},
		JUnit:  "target/nextest/ci/junit.xml",
		Setup:  "cargo install cargo-nextest",
		Notes:  "nextest writes JUnit only when the profile enables it; see .config/nextest.toml. The path is fixed, so use --parallel 1.",
	}, true
}

func detectMaven(dir string) (Project, bool) {
	if !exists(dir, "pom.xml") {
		return Project{}, false
	}
	return Project{
		Name:   "maven",
		Filter: "-Dtest=ClassName#methodName",
		Args:   []string{"mvn", "-q", "test"},
		JUnit:  "target/surefire-reports/*.xml",
		Notes:  "Surefire writes its own report paths, so --junit is a glob and --parallel 1 is required.",
	}, true
}

func detectGradle(dir string) (Project, bool) {
	if !anyExists(dir, "build.gradle", "build.gradle.kts") {
		return Project{}, false
	}
	return Project{
		Name:   "gradle",
		Filter: "--tests '*.ClassName.methodName'",
		Args:   []string{"./gradlew", "test"},
		JUnit:  "build/test-results/test/*.xml",
		Notes:  "Gradle writes fixed report paths, so --parallel 1 is required.",
	}, true
}

func detectPHPUnit(dir string) (Project, bool) {
	if !anyExists(dir, "phpunit.xml", "phpunit.xml.dist") && !strings.Contains(readFile(dir, "composer.json"), "phpunit") {
		return Project{}, false
	}
	return Project{
		Name:   "phpunit",
		Filter: "--filter testName",
		Args:   []string{"./vendor/bin/phpunit", "--log-junit", "{junit}"},
		JUnit:  defaultReport,
	}, true
}

func detectRSpec(dir string) (Project, bool) {
	if !exists(dir, "spec") || !anyExists(dir, "Gemfile", ".rspec") {
		return Project{}, false
	}
	return Project{
		Name:   "rspec",
		Filter: "-e 'test name'",
		Args:   []string{"bundle", "exec", "rspec", "--format", "RspecJunitFormatter", "--out", "{junit}"},
		JUnit:  defaultReport,
		Setup:  "add rspec_junit_formatter to your Gemfile",
	}, true
}
