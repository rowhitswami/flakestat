// Package junit parses JUnit XML test reports.
//
// JUnit XML has no official schema. Every framework emits a slightly different
// dialect, so this parser is deliberately tolerant: it accepts either root
// element, recurses into nested suites, tolerates missing or malformed numeric
// attributes, and strips bytes that are illegal in XML 1.0 rather than failing.
//
// See testdata/junit for the dialect corpus this is tested against.
package junit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrNoRoot is returned when no <testsuite>/<testsuites> element is found.
var ErrNoRoot = errors.New("junit: no <testsuite> or <testsuites> root element found")

// Status is the outcome of a single test case.
type Status string

const (
	StatusPass  Status = "pass"
	StatusFail  Status = "fail"
	StatusError Status = "error"
	StatusSkip  Status = "skip"
)

// Case is one test case observation, normalized across dialects.
type Case struct {
	Suite    string
	Class    string
	Name     string
	Status   Status
	Duration time.Duration
	Message  string
	File     string
	Line     int

	// KnownFlaky is set when the framework itself reported flakiness --
	// Surefire's <flakyFailure>/<flakyError>, meaning the test failed and then
	// passed on rerun within a single run. This is a direct flakiness signal
	// and does not need to be inferred from history.
	KnownFlaky bool

	// Reruns counts rerun attempts the framework recorded for this case.
	Reruns int
}

// Report is the result of parsing one JUnit XML document.
type Report struct {
	Name   string
	Cases  []Case
	Suites int

	// Properties are the suite-level <property> entries, merged across suites.
	// The parser makes no judgement about them; callers decide which names
	// they recognize, so that unknown properties are never stored.
	Properties map[string]string
}

// ID is a stable identifier for a test across runs.
func (c Case) ID() string {
	return hashID(c.Suite, c.Class, c.Name)
}

// NormalizedID groups parameterized cases into one family, so that
// test_login[oauth] and test_login[saml] share an ID.
func (c Case) NormalizedID() string {
	return hashID(c.Suite, c.Class, NormalizeName(c.Name))
}

func hashID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])[:16]
}

// DisplayName renders a human-readable identifier for reports.
func (c Case) DisplayName() string {
	if c.Class != "" && c.Class != c.Name {
		return c.Class + "::" + c.Name
	}
	if c.Suite != "" && c.Suite != c.Name {
		return c.Suite + "::" + c.Name
	}
	return c.Name
}

var paramSuffix = regexp.MustCompile(`(\[[^\[\]]*\]|\([^()]*\))\s*$`)

// NormalizeName strips trailing parameterization from a test name:
// "test_login[oauth]" -> "test_login", "testAdd(1, 2)" -> "testAdd".
func NormalizeName(name string) string {
	out := strings.TrimSpace(name)
	for {
		stripped := strings.TrimSpace(paramSuffix.ReplaceAllString(out, ""))
		if stripped == out || stripped == "" {
			return out
		}
		out = stripped
	}
}

// --- wire types -------------------------------------------------------------
//
// Numeric attributes are read as strings and parsed leniently: several
// frameworks emit line="" or locale-formatted times like "0,001", both of
// which make encoding/xml's native int/float binding fail the whole document.

type xmlSuites struct {
	Name   string     `xml:"name,attr"`
	Suites []xmlSuite `xml:"testsuite"`
}

type xmlSuite struct {
	Name       string        `xml:"name,attr"`
	Suites     []xmlSuite    `xml:"testsuite"` // PHPUnit nests suites
	Cases      []xmlCase     `xml:"testcase"`
	Properties []xmlProperty `xml:"properties>property"`
}

type xmlProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type xmlCase struct {
	Name      string `xml:"name,attr"`
	ClassName string `xml:"classname,attr"`
	Class     string `xml:"class,attr"`
	File      string `xml:"file,attr"`
	Line      string `xml:"line,attr"`
	Time      string `xml:"time,attr"`

	Failures []xmlDetail `xml:"failure"`
	Errors   []xmlDetail `xml:"error"`
	Skipped  *xmlDetail  `xml:"skipped"`

	// Surefire rerun markers.
	FlakyFailures []xmlDetail `xml:"flakyFailure"`
	FlakyErrors   []xmlDetail `xml:"flakyError"`
	RerunFailures []xmlDetail `xml:"rerunFailure"`
	RerunErrors   []xmlDetail `xml:"rerunError"`
}

type xmlDetail struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

func (d xmlDetail) text() string {
	if m := strings.TrimSpace(d.Message); m != "" {
		return m
	}
	if t := strings.TrimSpace(d.Type); t != "" {
		return t
	}
	return firstLine(d.Body)
}

// --- parsing ----------------------------------------------------------------

// Parse reads a JUnit XML document.
func Parse(r io.Reader) (*Report, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("junit: read: %w", err)
	}
	return ParseBytes(data)
}

// ParseBytes reads a JUnit XML document from memory.
func ParseBytes(data []byte) (*Report, error) {
	dec := xml.NewDecoder(bytes.NewReader(sanitize(data)))
	// Non-strict mode tolerates unknown entities and mismatched end tags, both
	// of which show up in reports carrying raw application output.
	dec.Strict = false
	dec.CharsetReader = charsetReader

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil, ErrNoRoot
		}
		if err != nil {
			return nil, fmt.Errorf("junit: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		switch strings.ToLower(start.Name.Local) {
		case "testsuites":
			var root xmlSuites
			if err := dec.DecodeElement(&root, &start); err != nil {
				return nil, fmt.Errorf("junit: decode <testsuites>: %w", err)
			}
			rep := &Report{Name: root.Name}
			for _, s := range root.Suites {
				flatten(s, "", rep)
			}
			return rep, nil

		case "testsuite":
			var root xmlSuite
			if err := dec.DecodeElement(&root, &start); err != nil {
				return nil, fmt.Errorf("junit: decode <testsuite>: %w", err)
			}
			rep := &Report{Name: root.Name}
			flatten(root, "", rep)
			return rep, nil

		default:
			// Skip wrappers some CI systems bolt on around the real report.
			continue
		}
	}
}

// ParseFile reads a JUnit XML document from disk.
func ParseFile(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rep, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rep, nil
}

// Expand resolves patterns to matching file paths in a stable order.
//
// Callers that need to know which file a case came from must expand and parse
// themselves rather than using ParseGlob, which deliberately discards that.
func Expand(patterns []string) ([]string, []error) {
	var (
		paths []string
		errs  []error
		seen  = map[string]bool{}
	)
	for _, pattern := range patterns {
		matches, err := glob(pattern)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pattern, err))
			continue
		}
		// A literal path that does not exist is a user error worth naming;
		// a glob matching nothing is not necessarily.
		if len(matches) == 0 && !strings.ContainsAny(pattern, "*?[") {
			errs = append(errs, fmt.Errorf("%s: no such file", pattern))
			continue
		}
		for _, m := range matches {
			m = filepath.Clean(m)
			if !seen[m] {
				seen[m] = true
				paths = append(paths, m)
			}
		}
	}
	sort.Strings(paths)
	return paths, errs
}

// ParseGlob parses every file matching pattern and merges the results. Files
// that fail to parse are reported but do not abort the others: a single
// corrupt shard should not discard an entire run.
func ParseGlob(pattern string) (*Report, []error) {
	paths, err := glob(pattern)
	if err != nil {
		return nil, []error{err}
	}

	merged := &Report{}
	var errs []error
	for _, p := range paths {
		rep, err := ParseFile(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		merged.Cases = append(merged.Cases, rep.Cases...)
		merged.Suites += rep.Suites
		for k, v := range rep.Properties {
			if merged.Properties == nil {
				merged.Properties = map[string]string{}
			}
			if _, seen := merged.Properties[k]; !seen {
				merged.Properties[k] = v
			}
		}
	}
	return merged, errs
}

// flatten walks nested suites, inheriting the nearest non-empty suite name.
func flatten(s xmlSuite, parent string, rep *Report) {
	name := s.Name
	if name == "" {
		name = parent
	}
	rep.Suites++

	for _, p := range s.Properties {
		if p.Name == "" {
			continue
		}
		if rep.Properties == nil {
			rep.Properties = map[string]string{}
		}
		// First value wins: a nested suite should not silently override the
		// outer one it inherits from.
		if _, seen := rep.Properties[p.Name]; !seen {
			rep.Properties[p.Name] = p.Value
		}
	}

	for _, c := range s.Cases {
		rep.Cases = append(rep.Cases, convert(c, name))
	}
	for _, child := range s.Suites {
		flatten(child, name, rep)
	}
}

func convert(c xmlCase, suite string) Case {
	class := c.ClassName
	if class == "" {
		class = c.Class
	}
	if class == "" {
		class = suite
	}

	out := Case{
		Suite:    suite,
		Class:    class,
		Name:     strings.TrimSpace(c.Name),
		Duration: parseDuration(c.Time),
		File:     c.File,
		Line:     parseInt(c.Line),
		Reruns:   len(c.FlakyFailures) + len(c.FlakyErrors) + len(c.RerunFailures) + len(c.RerunErrors),
	}

	// Precedence matters: a case carrying both <failure> and <skipped> (some
	// runners emit both when a failing test is skipped on retry) is a failure.
	switch {
	case len(c.Errors) > 0:
		out.Status = StatusError
		out.Message = c.Errors[0].text()
	case len(c.Failures) > 0:
		out.Status = StatusFail
		out.Message = c.Failures[0].text()
	case len(c.RerunFailures) > 0:
		// Failed, was rerun, failed again: a genuine failure.
		out.Status = StatusFail
		out.Message = c.RerunFailures[0].text()
	case len(c.RerunErrors) > 0:
		out.Status = StatusError
		out.Message = c.RerunErrors[0].text()
	case c.Skipped != nil:
		out.Status = StatusSkip
		out.Message = c.Skipped.text()
	default:
		out.Status = StatusPass
	}

	// Failed then passed on rerun, within a single run: the framework has
	// already proven this test is flaky.
	if len(c.FlakyFailures) > 0 || len(c.FlakyErrors) > 0 {
		out.KnownFlaky = true
		if out.Message == "" {
			if len(c.FlakyFailures) > 0 {
				out.Message = c.FlakyFailures[0].text()
			} else {
				out.Message = c.FlakyErrors[0].text()
			}
		}
	}

	return out
}

// --- lenient scalar parsing -------------------------------------------------

func parseDuration(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Locale-formatted numbers: "1,234.5" uses comma as a group separator,
	// while "0,001" from a comma-decimal locale means 0.001.
	if strings.Contains(s, ",") {
		if strings.Contains(s, ".") {
			s = strings.ReplaceAll(s, ",", "")
		} else {
			s = strings.ReplaceAll(s, ",", ".")
		}
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

func parseInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// charsetReader passes through the encodings we can already read as bytes.
// Anything else is an explicit error rather than silent mojibake.
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii", "iso-8859-1", "latin1":
		return input, nil
	default:
		return nil, fmt.Errorf("junit: unsupported charset %q", charset)
	}
}

// sanitize strips a UTF-8 BOM, invalid UTF-8 bytes, and control characters
// that are illegal in XML 1.0. Test output routinely carries ANSI escapes and
// raw binary, which would otherwise fail the whole document.
func sanitize(b []byte) []byte {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})

	if !bytes.ContainsFunc(b, func(r rune) bool { return !validXMLChar(r) }) && utf8.Valid(b) {
		return b
	}

	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			i++ // drop the invalid byte
			continue
		}
		if validXMLChar(r) {
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return out
}

func validXMLChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}
