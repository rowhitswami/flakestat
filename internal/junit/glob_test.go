package junit

import (
	"os"
	"path/filepath"
	"testing"
)

// The documented CI command is `flakestat ingest 'reports/**/*.xml'`, and
// `hunt` writes its reports flat. Before ** was implemented, that pair found
// nothing: filepath.Glob reads ** as a single *, which requires exactly one
// intervening directory.
func TestGlobDoubleStar(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"reports/junit-1.xml",
		"reports/junit-2.xml",
		"reports/shard-a/junit.xml",
		"reports/shard-a/nested/deep.xml",
		"reports/notes.txt",
		"other/junit.xml",
	}
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("<testsuites/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		pattern string
		want    []string
	}{
		{"reports/**/*.xml", []string{
			"reports/junit-1.xml",
			"reports/junit-2.xml",
			"reports/shard-a/junit.xml",
			"reports/shard-a/nested/deep.xml",
		}},
		{"**/*.xml", []string{
			"other/junit.xml",
			"reports/junit-1.xml",
			"reports/junit-2.xml",
			"reports/shard-a/junit.xml",
			"reports/shard-a/nested/deep.xml",
		}},
		// No **: unchanged filepath.Glob behaviour, one level only.
		{"reports/*.xml", []string{"reports/junit-1.xml", "reports/junit-2.xml"}},
		{"reports/*/*.xml", []string{"reports/shard-a/junit.xml"}},
		// The extension still filters.
		{"reports/**/*.txt", []string{"reports/notes.txt"}},
		{"reports/**/nope.xml", nil},
		{"missing/**/*.xml", nil},
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got, err := glob(filepath.FromSlash(tt.pattern))
			if err != nil {
				t.Fatalf("glob(%q): %v", tt.pattern, err)
			}
			var norm []string
			for _, g := range got {
				norm = append(norm, filepath.ToSlash(g))
			}
			if len(norm) != len(tt.want) {
				t.Fatalf("glob(%q) = %v, want %v", tt.pattern, norm, tt.want)
			}
			for i := range norm {
				if norm[i] != tt.want[i] {
					t.Fatalf("glob(%q) = %v, want %v", tt.pattern, norm, tt.want)
				}
			}
		})
	}
}
