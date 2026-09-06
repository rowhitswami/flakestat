// Package config loads the optional .flakestat.json project file.
//
// The file exists to remove ceremony: without it, every invocation repeats the
// test command and the report path, and the report path has to be written
// twice. With it, "flakestat hunt" is the whole command.
//
// JSON rather than YAML keeps the binary dependency-free, which matters for
// the audience that picks a local tool over a hosted one in the first place.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rowhitswami/flakestat/internal/score"
)

// FileName is the project config, looked for in the working directory and its
// parents so the tool works from anywhere inside a repo.
const FileName = ".flakestat.json"

// Command is a test command, accepted either as a list of arguments or as a
// single string. The list form is canonical and avoids quoting rules; the
// string form is what people reach for when editing the file by hand.
type Command []string

// UnmarshalJSON accepts both ["pytest","-q"] and "pytest -q".
func (c *Command) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*c = list
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("command must be a string or a list of strings")
	}

	parts, err := SplitCommand(s)
	if err != nil {
		return err
	}
	*c = parts
	return nil
}

// File is the on-disk configuration. Every field is optional; command-line
// flags always win.
type File struct {
	Version  int     `json:"version"`
	Command  Command `json:"command,omitempty"`
	JUnit    string  `json:"junit,omitempty"`
	Runs     int     `json:"runs,omitempty"`
	Parallel int     `json:"parallel,omitempty"`
	Dir      string  `json:"dir,omitempty"`
	Timeout  string  `json:"timeout,omitempty"`

	Scoring    score.Config `json:"scoring,omitempty"`
	Quarantine struct {
		Format string `json:"format,omitempty"`
	} `json:"quarantine,omitempty"`

	// path records where the file was found, for error messages.
	path string
}

// Path returns the file this config was loaded from, empty if none.
func (f *File) Path() string {
	if f == nil {
		return ""
	}
	return f.path
}

// Find walks up from dir looking for the config file. An empty result means
// no config exists, which is a supported way to use the tool.
func Find(dir string) string {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	for {
		candidate := filepath.Join(abs, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "" // reached the filesystem root
		}
		abs = parent
	}
}

// Load reads a config file. A missing file yields an empty config and no
// error: running without one is normal.
func Load(path string) (*File, error) {
	if path == "" {
		return &File{}, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &File{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	f := &File{path: path}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	if f.Version != 0 && f.Version != 1 {
		return nil, fmt.Errorf("config: %s: unsupported version %d (this build understands version 1)", path, f.Version)
	}
	return f, nil
}

// Save writes the config, creating parent directories as needed.
func (f *File) Save(path string) error {
	f.Version = 1

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", path, err)
	}
	return nil
}

// SplitCommand tokenizes a command string, honouring single and double quotes
// so paths containing spaces survive. It deliberately does not expand
// variables or globs: the command is executed directly, not through a shell.
func SplitCommand(s string) ([]string, error) {
	var (
		out   []string
		cur   strings.Builder
		quote rune
		open  bool
	)

	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			open = true
		case r == ' ' || r == '\t' || r == '\n':
			if cur.Len() > 0 || open {
				out = append(out, cur.String())
				cur.Reset()
				open = false
			}
		default:
			cur.WriteRune(r)
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote in command", quote)
	}
	if cur.Len() > 0 || open {
		out = append(out, cur.String())
	}
	return out, nil
}
