package cli

import "testing"

// go install and a release must both report a real version. A working tree
// must not, because its module version is a pseudo-version naming a release
// that was never cut.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name       string
		stamp      string
		modVersion string
		settings   map[string]string
		want       string
	}{
		{"release stamped by ldflags", "0.2.0", "", nil, "0.2.0"},
		{"stamp wins over build info", "0.2.0", "v9.9.9", nil, "0.2.0"},
		{"go install at a tag", "", "v0.2.0", nil, "0.2.0"},
		{"go install at a tag, prefix kept off", "", "v1.10.3", nil, "1.10.3"},
		{"no build info at all", "", "", nil, "dev"},
		{"module says devel", "", "(devel)", nil, "dev"},
		{
			"working tree, clean",
			"", "v0.2.1-0.20260908070508-07d5a03287cb",
			map[string]string{"vcs.revision": "07d5a03287cbaf0e01ac7c08b15b88130bd4cd5b"},
			"dev (07d5a03287cb)",
		},
		{
			"working tree, dirty",
			"", "v0.2.1-0.20260908070508-07d5a03287cb+dirty",
			map[string]string{"vcs.revision": "07d5a03287cbaf0e01ac7c08b15b88130bd4cd5b", "vcs.modified": "true"},
			"dev (07d5a03287cb, dirty)",
		},
		{
			"short revision is not truncated past its length",
			"", "", map[string]string{"vcs.revision": "abc123"}, "dev (abc123)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.stamp, tt.modVersion, tt.settings); got != tt.want {
				t.Errorf("resolveVersion(%q, %q, %v) = %q, want %q",
					tt.stamp, tt.modVersion, tt.settings, got, tt.want)
			}
		})
	}
}
