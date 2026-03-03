# Skill: Add a new command to an existing `gha-<family>` action

**Trigger:** User wants to add a new command to an existing action family.

---

## 1. Create `cmd/<name>.go`

File name convention: snake_case matching the command name (e.g. `check_pr_title.go` for `check-pr-title`).

```go
package cmd

import (
    "fmt"

    ac "github.com/bshore/gha/internal/actions-core"
)

func init() { Register("<command-name>", run<Command>) }

func run<Command>() error {
    // Read inputs
    myInput, _ := ac.GetInput("my_input", nil)
    if myInput == "" {
        myInput = "default-value"
    }

    // ... implement logic ...

    // Set output
    if err := ac.SetOutput("my_output", result); err != nil {
        ac.Warning(fmt.Sprintf("could not set my_output: %v", err), nil)
    }

    // Write job summary
    ac.JobSummary.
        AddHeading("<Command>", 2).
        AddTable([][]ac.SummaryTableCell{
            {{Data: "Input", Header: true}, {Data: "Result", Header: true}},
            {{Data: myInput}, {Data: "✅ success"}},
        })
    if err := ac.JobSummary.Write(nil); err != nil {
        ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
    }

    return nil
}
```

---

## 2. Input patterns

### String input with default

```go
myInput, _ := ac.GetInput("my_input", nil)
if myInput == "" {
    myInput = "default"
}
```

### Boolean input

`GetBooleanInput` errors on empty string — always use this helper:

```go
func boolInputOrDefault(name string, defaultVal bool) (bool, error) {
    raw, _ := ac.GetInput(name, nil)
    if raw == "" {
        return defaultVal, nil
    }
    return ac.GetBooleanInput(name, nil)
}
```

### Required input

```go
myInput, _ := ac.GetInput("my_input", nil)
if myInput == "" {
    return fmt.Errorf("input 'my_input' is required")
}
```

### Naming rule

**Input names must use underscores, not hyphens.**
`ac.GetInput` reads `INPUT_<NAME>` after uppercasing and replacing spaces→underscores only.
A hyphen in the name produces an invalid env var like `INPUT_MY-INPUT`.

---

## 3. Output pattern

```go
if err := ac.SetOutput("output_name", value); err != nil {
    ac.Warning(fmt.Sprintf("could not set output_name: %v", err), nil)
}
```

---

## 4. Job summary pattern

```go
ac.JobSummary.
    AddHeading("Title", 2).
    AddTable([][]ac.SummaryTableCell{
        {
            {Data: "Col1", Header: true},
            {Data: "Col2", Header: true},
        },
        {
            {Data: val1},
            {Data: val2},
        },
    }).
    AddSeparator().
    AddList(items, false)  // false = unordered list
if err := ac.JobSummary.Write(nil); err != nil {
    ac.Warning(fmt.Sprintf("could not write job summary: %v", err), nil)
}
```

Available `JobSummary` methods: `AddHeading`, `AddTable`, `AddList`, `AddCodeBlock`,
`AddDetails`, `AddSeparator`, `AddBreak`, `AddQuote`, `AddLink`, `AddImage`, `AddRaw`.

---

## 5. PR comment pattern

The family-level `main.go` already handles upsert-on-failure and delete-on-success for the
family's marker. No per-command PR comment work is needed unless the command wants to post
a command-specific comment (rare). If so:

```go
// In your command function, the marker is defined in main.go:
// const prCommentMarker = "<!-- gha-<family>-error -->"
// Pass it through if needed, or define a sub-marker for the command.
```

---

## 6. Update `action.yml`

### Add the new input

```yaml
inputs:
  # ... existing inputs ...
  my_new_input:
    description: Description of the new input
    required: false
    default: ""
```

### Forward it in the `run` step `env:` block

GitHub Actions composite `run:` steps do NOT auto-populate `INPUT_*`.
You MUST explicitly forward every input:

```yaml
    - id: run
      shell: bash
      env:
        # ... existing env vars ...
        INPUT_MY_NEW_INPUT: ${{ inputs.my_new_input }}
```

### Add a new output (if needed)

```yaml
outputs:
  # ... existing outputs ...
  my_new_output:
    description: Description of the new output
    value: ${{ steps.run.outputs.my_new_output }}
```

---

## 7. Checklist

- [ ] New file `cmd/<name>.go` with `init()` self-registration
- [ ] Input names use underscores only
- [ ] Boolean inputs use `boolInputOrDefault` helper
- [ ] `ac.SetOutput` calls wrapped in warning-on-error
- [ ] `ac.JobSummary.Write` calls wrapped in warning-on-error
- [ ] New inputs added to `action.yml` inputs block
- [ ] New inputs forwarded in `action.yml` run step `env:` block as `INPUT_<NAME>`
- [ ] New outputs added to `action.yml` outputs block if applicable
- [ ] `go build ./actions/gha-<family>/...` — compiles cleanly
- [ ] `go vet ./actions/gha-<family>/...` — no issues
