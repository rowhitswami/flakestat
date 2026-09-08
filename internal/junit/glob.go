package junit

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// glob expands a pattern, supporting ** as "any number of directories".
//
// filepath.Glob does not implement **; it treats the two stars as one, so
// "reports/**/*.xml" matches reports/<onedir>/x.xml and silently misses
// reports/x.xml. Every CI recipe in the documentation uses that pattern, and
// `hunt` writes its reports flat, so following the docs end to end found
// nothing. Rather than change fifteen documented commands to mean less, **
// now means what people already read it as: this directory and below.
//
// Patterns without ** are handed to filepath.Glob unchanged.
func glob(pattern string) ([]string, error) {
	if !strings.Contains(pattern, "**") {
		return filepath.Glob(pattern)
	}

	// Walk from the deepest fixed directory, so "reports/**/*.xml" does not
	// scan the whole working tree.
	sep := string(filepath.Separator)
	norm := filepath.FromSlash(pattern)
	segs := strings.Split(norm, sep)

	root := "."
	if fixed := segs[:firstMeta(segs)]; len(fixed) > 0 {
		root = filepath.Join(fixed...)
		if strings.HasPrefix(norm, sep) {
			root = sep + root
		}
	}
	if _, err := os.Stat(root); err != nil {
		return nil, nil // nothing to walk is not an error, same as Glob
	}

	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable directory should not abort the rest
		}
		if d.IsDir() {
			return nil
		}
		if matchSegments(segs, strings.Split(filepath.Clean(p), sep)) {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

// firstMeta returns the index of the first segment containing a metacharacter.
func firstMeta(segs []string) int {
	for i, s := range segs {
		if strings.ContainsAny(s, "*?[") {
			return i
		}
	}
	return len(segs)
}

// matchSegments matches path segments against pattern segments, where "**"
// consumes any number of segments including none.
func matchSegments(pat, name []string) bool {
	// Drop a leading "." that Clean introduces on relative walks.
	if len(name) > 0 && name[0] == "." && (len(pat) == 0 || pat[0] != ".") {
		name = name[1:]
	}

	if len(pat) == 0 {
		return len(name) == 0
	}

	if pat[0] == "**" {
		// Zero segments consumed, then one, then two, and so on.
		for i := 0; i <= len(name); i++ {
			if matchSegments(pat[1:], name[i:]) {
				return true
			}
		}
		return false
	}

	if len(name) == 0 {
		return false
	}
	ok, err := filepath.Match(pat[0], name[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pat[1:], name[1:])
}
