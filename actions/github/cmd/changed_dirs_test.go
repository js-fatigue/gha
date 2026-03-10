package cmd

import (
	"testing"
)

func TestFilterDirs(t *testing.T) {
	tests := []struct {
		name    string
		dirs    []string
		include []string
		exclude []string
		want    []string
	}{
		{
			name: "no filters",
			dirs: []string{"a", "b", "c"},
			want: []string{"a", "b", "c"},
		},
		{
			name:    "include prefix match",
			dirs:    []string{"actions/github", "actions/go", "internal/core"},
			include: []string{"actions/"},
			want:    []string{"actions/github", "actions/go"},
		},
		{
			name:    "include exact match",
			dirs:    []string{"foo", "foobar", "bar"},
			include: []string{"foo"},
			want:    []string{"foo", "foobar"},
		},
		{
			name:    "include no matches",
			dirs:    []string{"a", "b", "c"},
			include: []string{"z"},
			want:    []string{},
		},
		{
			name:    "exclude exact match",
			dirs:    []string{"a", "b", "c"},
			exclude: []string{"b"},
			want:    []string{"a", "c"},
		},
		{
			name:    "exclude does not apply prefix matching",
			dirs:    []string{"foo", "foobar"},
			exclude: []string{"foo"},
			want:    []string{"foobar"},
		},
		{
			name:    "include and exclude",
			dirs:    []string{"actions/github", "actions/go", "internal/core"},
			include: []string{"actions/"},
			exclude: []string{"actions/go"},
			want:    []string{"actions/github"},
		},
		{
			name:    "include multiple prefixes",
			dirs:    []string{"actions/github", "internal/core", "docker"},
			include: []string{"actions/", "docker"},
			want:    []string{"actions/github", "docker"},
		},
		{
			name: "empty dirs",
			dirs: []string{},
			want: []string{},
		},
		{
			name:    "exclude all",
			dirs:    []string{"a", "b"},
			exclude: []string{"a", "b"},
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterDirs(tt.dirs, tt.include, tt.exclude)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tt.want), got)
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestCapDir(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		maxDepth int
		want     string
	}{
		{
			name:     "maxDepth 0 disables capping",
			dir:      "a/b/c/d",
			maxDepth: 0,
			want:     "a/b/c/d",
		},
		{
			name:     "negative maxDepth disables capping",
			dir:      "a/b/c",
			maxDepth: -1,
			want:     "a/b/c",
		},
		{
			name:     "dot dir returned as-is regardless of maxDepth",
			dir:      ".",
			maxDepth: 1,
			want:     ".",
		},
		{
			name:     "depth within limit unchanged",
			dir:      "a/b",
			maxDepth: 3,
			want:     "a/b",
		},
		{
			name:     "depth exactly at limit unchanged",
			dir:      "a/b/c",
			maxDepth: 3,
			want:     "a/b/c",
		},
		{
			name:     "depth exceeds limit — truncated",
			dir:      "a/b/c/d",
			maxDepth: 2,
			want:     "a/b",
		},
		{
			name:     "maxDepth 1 returns top-level segment",
			dir:      "actions/github/cmd",
			maxDepth: 1,
			want:     "actions",
		},
		{
			name:     "single segment within any positive maxDepth",
			dir:      "foo",
			maxDepth: 5,
			want:     "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capDir(tt.dir, tt.maxDepth)
			if got != tt.want {
				t.Errorf("capDir(%q, %d) = %q, want %q", tt.dir, tt.maxDepth, got, tt.want)
			}
		})
	}
}
