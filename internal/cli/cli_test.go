package cli

import (
	"flag"
	"strings"
	"testing"
)

// newTestFlags mirrors the shape of the ingest command: string flags that take
// a value, and a boolean flag that does not.
func newTestFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("dir", "", "state directory")
	fs.String("commit", "", "commit SHA")
	fs.Bool("verbose", false, "verbose")
	return fs
}

func TestPermuteMovesFlagsBeforePositionals(t *testing.T) {
	tests := []struct {
		name      string
		in        []string
		wantFlags map[string]string
		wantArgs  []string
	}{
		{
			name:      "flags already first",
			in:        []string{"--dir", "build", "reports/a.xml"},
			wantFlags: map[string]string{"dir": "build"},
			wantArgs:  []string{"reports/a.xml"},
		},
		{
			// The reported bug: --dir and its value were read as file patterns.
			name:      "flags after positional",
			in:        []string{"reports/a.xml", "--dir", "build"},
			wantFlags: map[string]string{"dir": "build"},
			wantArgs:  []string{"reports/a.xml"},
		},
		{
			name:      "flags interleaved with positionals",
			in:        []string{"a.xml", "--dir", "build", "b.xml", "--commit", "abc"},
			wantFlags: map[string]string{"dir": "build", "commit": "abc"},
			wantArgs:  []string{"a.xml", "b.xml"},
		},
		{
			name:      "equals form needs no value lookahead",
			in:        []string{"a.xml", "--dir=build"},
			wantFlags: map[string]string{"dir": "build"},
			wantArgs:  []string{"a.xml"},
		},
		{
			name:      "boolean flag does not swallow the next argument",
			in:        []string{"a.xml", "--verbose", "b.xml"},
			wantFlags: map[string]string{},
			wantArgs:  []string{"a.xml", "b.xml"},
		},
		{
			name:      "single dash form",
			in:        []string{"a.xml", "-dir", "build"},
			wantFlags: map[string]string{"dir": "build"},
			wantArgs:  []string{"a.xml"},
		},
		{
			name:      "no flags at all",
			in:        []string{"a.xml", "b.xml"},
			wantFlags: map[string]string{},
			wantArgs:  []string{"a.xml", "b.xml"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fs := newTestFlags()
			if err := fs.Parse(permute(fs, tc.in)); err != nil {
				t.Fatalf("Parse: %v", err)
			}

			for name, want := range tc.wantFlags {
				if got := fs.Lookup(name).Value.String(); got != want {
					t.Errorf("flag %s = %q, want %q", name, got, want)
				}
			}
			if got := fs.Args(); strings.Join(got, ",") != strings.Join(tc.wantArgs, ",") {
				t.Errorf("positional args = %v, want %v", got, tc.wantArgs)
			}
		})
	}
}

// Everything after a bare "--" belongs to the wrapped command and must survive
// untouched, flags included.
func TestPermuteLeavesDoubleDashTailAlone(t *testing.T) {
	fs := newTestFlags()
	in := []string{"--dir", "build", "--", "pytest", "--junitxml=out.xml", "-q"}

	if err := fs.Parse(permute(fs, in)); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := fs.Lookup("dir").Value.String(); got != "build" {
		t.Errorf("dir = %q, want build", got)
	}
	want := "pytest,--junitxml=out.xml,-q"
	if got := strings.Join(fs.Args(), ","); got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

// A value that looks like a path must not be mistaken for a flag.
func TestPermutePreservesFlagValuesThatLookLikePaths(t *testing.T) {
	fs := newTestFlags()
	in := []string{"reports/a.xml", "--dir", "./build/state"}

	if err := fs.Parse(permute(fs, in)); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := fs.Lookup("dir").Value.String(); got != "./build/state" {
		t.Errorf("dir = %q, want ./build/state", got)
	}
	if got := strings.Join(fs.Args(), ","); got != "reports/a.xml" {
		t.Errorf("args = %q, want reports/a.xml", got)
	}
}

func TestRejectStrayFlags(t *testing.T) {
	if err := rejectStrayFlags([]string{"a.xml", "b.xml"}); err != nil {
		t.Errorf("clean positionals should not error: %v", err)
	}

	err := rejectStrayFlags([]string{"a.xml", "--typo"})
	if err == nil {
		t.Fatal("expected an error for a flag-looking positional")
	}
	if !strings.Contains(err.Error(), "--typo") {
		t.Errorf("error should name the offending flag, got: %v", err)
	}
}

func TestTailLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"empty", "", 5, ""},
		{"whitespace only", "\n\n  \n", 5, ""},
		{"fewer lines than limit", "a\nb", 5, "a\nb"},
		{"more lines than limit", "a\nb\nc\nd", 2, "c\nd"},
		{"trailing newlines trimmed", "a\nb\n\n\n", 5, "a\nb"},
		{"single line", "only", 3, "only"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailLines(tc.in, tc.n); got != tc.want {
				t.Errorf("tailLines(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
