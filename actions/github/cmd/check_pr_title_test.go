package cmd

import "testing"

func TestCheckPRTitle(t *testing.T) {
	valid := []string{
		"feat: add login flow",
		"fix: correct off-by-one error",
		"docs: update README",
		"style: reformat imports",
		"refactor: extract helper",
		"perf: cache database calls",
		"test: add unit tests",
		"chore: bump dependencies",
		"ci: add lint step",
		"build: update Makefile",
		"revert: undo previous change",
		// with scope
		"feat(auth): support OAuth2",
		"fix(api): handle 404 gracefully",
		// breaking change
		"feat!: drop support for Go 1.20",
		"fix(core)!: change error return type",
		// description with extra punctuation / spaces
		"chore: update go.sum and go.mod",
		"feat(ui): add button — redesigned",
	}

	invalid := []string{
		"",
		"just a plain message",
		// missing colon
		"feat add something",
		// unknown type
		"unknown: something",
		// missing description after colon+space
		"feat: ",
		// no space after colon
		"feat:description",
		// uppercase type
		"Feat: something",
		// scope with no description
		"fix(scope): ",
	}

	for _, title := range valid {
		t.Run("valid/"+title, func(t *testing.T) {
			if err := checkPRTitle(title); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}

	for _, title := range invalid {
		t.Run("invalid/"+title, func(t *testing.T) {
			if err := checkPRTitle(title); err == nil {
				t.Errorf("expected error for %q, got nil", title)
			}
		})
	}
}
