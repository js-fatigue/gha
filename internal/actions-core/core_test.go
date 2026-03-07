package ac

import (
	"testing"
)

func TestGetInput(t *testing.T) {
	t.Run("basic read", func(t *testing.T) {
		t.Setenv("INPUT_MY_INPUT", "hello")
		got, err := GetInput("my_input", nil)
		if err != nil || got != "hello" {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("name uppercased and spaces to underscores", func(t *testing.T) {
		t.Setenv("INPUT_WORKING_DIRECTORY", "src")
		got, err := GetInput("working directory", nil)
		if err != nil || got != "src" {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("trims whitespace by default", func(t *testing.T) {
		t.Setenv("INPUT_FOO", "  bar  ")
		got, _ := GetInput("foo", nil)
		if got != "bar" {
			t.Errorf("got %q, want %q", got, "bar")
		}
	})

	t.Run("SkipTrimWhitespace preserves spaces", func(t *testing.T) {
		t.Setenv("INPUT_FOO", "  bar  ")
		got, _ := GetInput("foo", &InputOptions{SkipTrimWhitespace: true})
		if got != "  bar  " {
			t.Errorf("got %q, want %q", got, "  bar  ")
		}
	})

	t.Run("required returns error when empty", func(t *testing.T) {
		t.Setenv("INPUT_MISSING", "")
		_, err := GetInput("missing", &InputOptions{Required: true})
		if err == nil {
			t.Fatal("expected error for required empty input")
		}
	})

	t.Run("optional empty returns no error", func(t *testing.T) {
		t.Setenv("INPUT_MISSING", "")
		got, err := GetInput("missing", nil)
		if err != nil || got != "" {
			t.Errorf("got %q, %v", got, err)
		}
	})
}

func TestGetBooleanInput(t *testing.T) {
	trueVals := []string{"true", "True", "TRUE"}
	for _, v := range trueVals {
		t.Run("true: "+v, func(t *testing.T) {
			t.Setenv("INPUT_FLAG", v)
			got, err := GetBooleanInput("flag", nil)
			if err != nil || !got {
				t.Errorf("got %v, %v", got, err)
			}
		})
	}

	falseVals := []string{"false", "False", "FALSE"}
	for _, v := range falseVals {
		t.Run("false: "+v, func(t *testing.T) {
			t.Setenv("INPUT_FLAG", v)
			got, err := GetBooleanInput("flag", nil)
			if err != nil || got {
				t.Errorf("got %v, %v", got, err)
			}
		})
	}

	t.Run("invalid value returns error", func(t *testing.T) {
		t.Setenv("INPUT_FLAG", "yes")
		_, err := GetBooleanInput("flag", nil)
		if err == nil {
			t.Fatal("expected error for invalid boolean")
		}
	})

	t.Run("empty value returns error", func(t *testing.T) {
		t.Setenv("INPUT_FLAG", "")
		_, err := GetBooleanInput("flag", nil)
		if err == nil {
			t.Fatal("expected error for empty boolean")
		}
	})
}

func TestGetBooleanInputOrDefault(t *testing.T) {
	t.Run("empty returns true default", func(t *testing.T) {
		t.Setenv("INPUT_FLAG", "")
		got, err := GetBooleanInputOrDefault("flag", true, nil)
		if err != nil || !got {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("empty returns false default", func(t *testing.T) {
		t.Setenv("INPUT_FLAG", "")
		got, err := GetBooleanInputOrDefault("flag", false, nil)
		if err != nil || got {
			t.Errorf("got %v, %v", got, err)
		}
	})

	t.Run("non-empty overrides default", func(t *testing.T) {
		t.Setenv("INPUT_FLAG", "false")
		got, err := GetBooleanInputOrDefault("flag", true, nil)
		if err != nil || got {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestGetMultilineInput(t *testing.T) {
	t.Run("splits on newlines and filters empty lines", func(t *testing.T) {
		t.Setenv("INPUT_PATHS", "foo\n\nbar\n  \nbaz")
		got, err := GetMultilineInput("paths", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"foo", "bar", "baz"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("[%d] got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		t.Setenv("INPUT_PATHS", "")
		got, err := GetMultilineInput("paths", nil)
		if err != nil || len(got) != 0 {
			t.Errorf("got %v, %v", got, err)
		}
	})
}

func TestGetState(t *testing.T) {
	t.Setenv("STATE_MY_KEY", "my-value")
	if got := GetState("MY_KEY"); got != "my-value" {
		t.Errorf("got %q, want %q", got, "my-value")
	}
}

// TestGetStructuredInput covers JSON auto-detection, HCL parsing, empty input, and errors.
func TestGetStructuredInput(t *testing.T) {
	type cacheInput struct {
		Action      string   `json:"action"`
		Path        []string `json:"path"`
		Key         string   `json:"key"`
		RestoreKeys []string `json:"restore_keys"`
		FailOnMiss  bool     `json:"fail_on_miss"`
	}

	type changedDirsInput struct {
		Base     string `json:"base"`
		MaxDepth int    `json:"max_depth"`
	}

	t.Run("JSON path", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", `{"max_depth": 2}`)
		inp := changedDirsInput{}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.MaxDepth != 2 {
			t.Errorf("MaxDepth: got %d, want 2", inp.MaxDepth)
		}
	})

	t.Run("HCL path simple", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", "max_depth = 2")
		inp := changedDirsInput{}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.MaxDepth != 2 {
			t.Errorf("MaxDepth: got %d, want 2", inp.MaxDepth)
		}
	})

	t.Run("HCL path multiline", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", "base = \"main\"\nmax_depth = 3")
		inp := changedDirsInput{}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.Base != "main" {
			t.Errorf("Base: got %q, want %q", inp.Base, "main")
		}
		if inp.MaxDepth != 3 {
			t.Errorf("MaxDepth: got %d, want 3", inp.MaxDepth)
		}
	})

	t.Run("HCL with array", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", `action = "restore"
key    = "my-key"
path   = ["~/go/pkg/mod", "~/go/bin"]
restore_keys = ["go-modules-"]`)
		inp := cacheInput{}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.Action != "restore" {
			t.Errorf("Action: got %q, want %q", inp.Action, "restore")
		}
		if inp.Key != "my-key" {
			t.Errorf("Key: got %q, want %q", inp.Key, "my-key")
		}
		if len(inp.Path) != 2 || inp.Path[0] != "~/go/pkg/mod" || inp.Path[1] != "~/go/bin" {
			t.Errorf("Path: got %v, want [~/go/pkg/mod ~/go/bin]", inp.Path)
		}
		if len(inp.RestoreKeys) != 1 || inp.RestoreKeys[0] != "go-modules-" {
			t.Errorf("RestoreKeys: got %v, want [go-modules-]", inp.RestoreKeys)
		}
	})

	t.Run("empty input leaves struct unchanged", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", "")
		inp := changedDirsInput{Base: "develop", MaxDepth: 5}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.Base != "develop" || inp.MaxDepth != 5 {
			t.Errorf("defaults overwritten: got %+v", inp)
		}
	})

	t.Run("pre-init defaults preserved when field absent from HCL", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", `max_depth = 1`)
		inp := changedDirsInput{Base: "develop"}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inp.Base != "develop" {
			t.Errorf("Base default overwritten: got %q", inp.Base)
		}
		if inp.MaxDepth != 1 {
			t.Errorf("MaxDepth: got %d, want 1", inp.MaxDepth)
		}
	})

	t.Run("invalid HCL returns error", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", "this is not valid = = hcl !!!")
		inp := changedDirsInput{}
		if err := GetStructuredInput("input", &inp); err == nil {
			t.Fatal("expected error for invalid HCL, got nil")
		}
	})

	t.Run("HCL bool field", func(t *testing.T) {
		t.Setenv("INPUT_INPUT", "fail_on_miss = true")
		inp := cacheInput{}
		if err := GetStructuredInput("input", &inp); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !inp.FailOnMiss {
			t.Error("FailOnMiss: got false, want true")
		}
	})
}
