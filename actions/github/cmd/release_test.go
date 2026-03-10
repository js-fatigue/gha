package cmd

import "testing"

func TestDetermineBumpType(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		// major — breaking change marker "!"
		{"feat!: drop Go 1.20 support", "major"},
		{"fix!: change error return type", "major"},
		{"chore(deps)!: remove legacy API", "major"},
		// minor — feat without "!"
		{"feat: add login flow", "minor"},
		{"feat(auth): support OAuth2", "minor"},
		// patch — all other types
		{"fix: correct off-by-one error", "patch"},
		{"docs: update README", "patch"},
		{"style: reformat imports", "patch"},
		{"refactor: extract helper", "patch"},
		{"perf: cache DB calls", "patch"},
		{"test: add unit tests", "patch"},
		{"chore: bump dependencies", "patch"},
		{"ci: add lint step", "patch"},
		{"build: update Makefile", "patch"},
		{"revert: undo last change", "patch"},
		// non-conventional → patch
		{"just some commit", "patch"},
		{"", "patch"},
		{"Feat: not conventional (uppercase)", "patch"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := determineBumpType(tt.title)
			if got != tt.want {
				t.Errorf("determineBumpType(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestApplyBump(t *testing.T) {
	tests := []struct {
		version  string
		bumpType string
		want     string
	}{
		// major resets minor and patch
		{"1.2.3", "major", "2.0.0"},
		{"0.0.0", "major", "1.0.0"},
		{"3.9.9", "major", "4.0.0"},
		// minor resets patch only
		{"1.2.3", "minor", "1.3.0"},
		{"0.0.0", "minor", "0.1.0"},
		{"1.9.9", "minor", "1.10.0"},
		// patch increments only patch
		{"1.2.3", "patch", "1.2.4"},
		{"0.0.0", "patch", "0.0.1"},
		{"1.2.9", "patch", "1.2.10"},
		// unknown bumpType falls through to patch
		{"1.2.3", "unknown", "1.2.4"},
		{"1.2.3", "", "1.2.4"},
	}

	for _, tt := range tests {
		t.Run(tt.version+"/"+tt.bumpType, func(t *testing.T) {
			got := applyBump(tt.version, tt.bumpType)
			if got != tt.want {
				t.Errorf("applyBump(%q, %q) = %q, want %q", tt.version, tt.bumpType, got, tt.want)
			}
		})
	}
}
